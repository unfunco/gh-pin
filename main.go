package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

const (
	pinsURL           = "https://unfun.co/pins.json"
	issueEndpoint     = "repos/unfunco/github-actions-pins/issues"
	workflowDirectory = ".github/workflows"
	maxPinsSize       = 1 << 20
	httpTimeout       = 10 * time.Second
)

var shaPattern = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

type pinsFile struct {
	Actions []pin `json:"actions"`
}

type pin struct {
	Action      string    `json:"action"`
	Tag         string    `json:"tag"`
	SHA         string    `json:"sha"`
	PublishedAt time.Time `json:"published_at"`
}

type githubClient interface {
	DoWithContext(ctx context.Context, method, path string, body io.Reader, response any) error
}

type app struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	httpClient *http.Client
	pinsURL    string
	userAgent  string
	github     githubClient
}

type resolvedPin struct {
	sha    string
	label  string
	source string
}

type missingAction struct {
	Action string
	Ref    string
}

func (m missingAction) key() string {
	return m.Action + "@" + m.Ref
}

func main() {
	if err := run(context.Background(), os.Stdin, os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "gh pin: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error {
	client, err := api.DefaultRESTClient()
	if err != nil {
		return fmt.Errorf("creating GitHub client: %w", err)
	}

	a := app{
		stdin:      stdin,
		stdout:     stdout,
		stderr:     stderr,
		httpClient: &http.Client{Timeout: httpTimeout},
		pinsURL:    pinsURL,
		userAgent:  formatUserAgent(version),
		github:     client,
	}

	return a.run(ctx)
}

func (a app) run(ctx context.Context) error {
	entries, err := os.ReadDir(workflowDirectory)
	if err != nil {
		return fmt.Errorf("reading workflows directory: %w", err)
	}

	pins, err := a.fetchPins(ctx)
	if err != nil {
		return err
	}

	pinMap := indexPins(pins.Actions)

	missing := make(map[string]missingAction)
	var errs []error

	for _, entry := range entries {
		if entry.IsDir() || !isWorkflowFile(entry.Name()) {
			continue
		}

		path := filepath.Join(workflowDirectory, entry.Name())
		actions, err := a.processWorkflow(ctx, path, pinMap)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", entry.Name(), err))
		}

		for _, action := range actions {
			missing[action.key()] = action
		}
	}

	if len(missing) > 0 {
		if err := a.offerIssue(ctx, sortedMissingActions(missing)); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (a app) fetchPins(ctx context.Context) (*pinsFile, error) {
	endpoint := a.pinsURL
	if endpoint == "" {
		endpoint = pinsURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating pins request: %w", err)
	}
	req.Header.Set("User-Agent", a.userAgentOrDefault())

	resp, err := a.httpClientOrDefault().Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching pins: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("fetching pins: HTTP %d", resp.StatusCode)
	}

	var pins pinsFile
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxPinsSize)).Decode(&pins); err != nil {
		return nil, fmt.Errorf("parsing pins: %w", err)
	}

	return &pins, nil
}

func (a app) processWorkflow(ctx context.Context, path string, pins map[string]pin) ([]missingAction, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading workflow: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	modified := false
	missing := make(map[string]missingAction)
	name := filepath.Base(path)

	for i, line := range lines {
		prefix, action, ref, ok := parseUsesLine(line)
		if !ok {
			continue
		}

		repo, ok := actionRepository(action)
		if !ok {
			continue
		}

		resolved, needsIssue, resolveErr := a.resolveAction(ctx, pins, repo, action, ref)
		if resolveErr != nil {
			_, _ = fmt.Fprintf(a.stderr, "  ⚠ %s: %s@%s: %v\n", name, action, ref, resolveErr)
			missingRef := missingAction{Action: action, Ref: ref}
			missing[missingRef.key()] = missingRef
			continue
		}

		if needsIssue {
			missingRef := missingAction{Action: action, Ref: ref}
			missing[missingRef.key()] = missingRef
		}

		newLine := formatUsesLine(prefix, action, resolved.sha, resolved.label)
		if line == newLine {
			continue
		}

		lines[i] = newLine
		modified = true

		_, _ = fmt.Fprintf(
			a.stdout,
			"  ✓ %s: %s@%s pinned to %s (%s)\n",
			name,
			action,
			ref,
			shortSHA(resolved.sha),
			resolved.source,
		)
	}

	if !modified {
		return sortedMissingActions(missing), nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return sortedMissingActions(missing), fmt.Errorf("stat workflow: %w", err)
	}

	if err := writeFileAtomically(path, []byte(strings.Join(lines, "\n")), info.Mode().Perm()); err != nil {
		return sortedMissingActions(missing), fmt.Errorf("writing workflow: %w", err)
	}

	return sortedMissingActions(missing), nil
}

func (a app) resolveAction(ctx context.Context, pins map[string]pin, repo, action, ref string) (resolvedPin, bool, error) {
	if pinEntry, ok := pins[action]; ok {
		label := pinEntry.Tag
		if label == "" {
			label = ref
		}
		displayLabel := sanitizeLabel(label)
		if displayLabel == "" {
			displayLabel = ref
		}
		return resolvedPin{
			sha:    pinEntry.SHA,
			label:  label,
			source: displayLabel + " via pin list",
		}, false, nil
	}

	if looksLikeSHA(ref) {
		return resolvedPin{
			sha:    ref,
			label:  "",
			source: "already pinned",
		}, false, nil
	}

	sha, err := a.lookupCommitSHA(ctx, repo, action, ref)
	if err != nil {
		return resolvedPin{}, true, err
	}

	return resolvedPin{
		sha:    sha,
		label:  ref,
		source: sanitizeLabel(ref) + " via live GitHub lookup",
	}, true, nil
}

func (a app) lookupCommitSHA(ctx context.Context, repo, action, ref string) (string, error) {
	path := fmt.Sprintf("repos/%s/commits/%s", repo, url.PathEscape(ref))

	var response struct {
		SHA string `json:"sha"`
	}

	if err := a.github.DoWithContext(ctx, http.MethodGet, path, nil, &response); err != nil {
		return "", fmt.Errorf("resolving %s: %w", action, err)
	}
	if !looksLikeSHA(response.SHA) {
		return "", fmt.Errorf("resolving %s: invalid SHA from GitHub", action)
	}

	return response.SHA, nil
}

func (a app) offerIssue(ctx context.Context, actions []missingAction) error {
	if len(actions) == 0 {
		return nil
	}

	_, _ = fmt.Fprintln(a.stdout)
	_, _ = fmt.Fprintln(a.stdout, "These actions are not yet in the pin list:")
	for _, action := range actions {
		_, _ = fmt.Fprintf(a.stdout, "  • %s@%s\n", action.Action, action.Ref)
	}
	_, _ = fmt.Fprint(a.stdout, "\nOpen an issue so they can be cached for faster future runs? [y/N] ")

	input := a.stdin
	if input == nil {
		input = strings.NewReader("")
	}

	reader := bufio.NewReader(input)
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("reading answer: %w", err)
	}

	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		return nil
	}

	return createIssue(ctx, a.github, a.stdout, actions)
}

func parseUsesLine(line string) (prefix, action, ref string, ok bool) {
	before, after, found := strings.Cut(line, "uses:")
	if !found {
		return "", "", "", false
	}

	trimmedBefore := strings.TrimSpace(before)
	if trimmedBefore != "" && trimmedBefore != "-" {
		return "", "", "", false
	}

	ws := len(after) - len(strings.TrimLeft(after, " \t"))
	prefix = before + "uses:" + after[:ws]

	value, _, _ := strings.Cut(strings.TrimSpace(after), "#")
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
	}

	action, ref, ok = strings.Cut(value, "@")
	if !ok || action == "" || ref == "" {
		return "", "", "", false
	}

	return prefix, action, ref, true
}

func actionRepository(action string) (string, bool) {
	if strings.HasPrefix(action, "./") || strings.HasPrefix(action, "../") || strings.HasPrefix(action, "docker://") {
		return "", false
	}

	parts := strings.Split(action, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}

	return parts[0] + "/" + parts[1], true
}

func looksLikeSHA(ref string) bool {
	return shaPattern.MatchString(ref)
}

func formatUsesLine(prefix, action, sha, label string) string {
	line := prefix + action + "@" + sha
	label = sanitizeLabel(label)
	if label == "" {
		return line
	}
	return line + " # " + label
}

func shortSHA(sha string) string {
	if len(sha) <= 12 {
		return sha
	}
	return sha[:12]
}

func isWorkflowFile(name string) bool {
	return strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml")
}

func sortedMissingActions(set map[string]missingAction) []missingAction {
	actions := make([]missingAction, 0, len(set))
	for _, action := range set {
		actions = append(actions, action)
	}

	sort.Slice(actions, func(i, j int) bool {
		if actions[i].Action == actions[j].Action {
			return actions[i].Ref < actions[j].Ref
		}
		return actions[i].Action < actions[j].Action
	})

	return actions
}

func indexPins(actions []pin) map[string]pin {
	index := make(map[string]pin, len(actions))
	for _, pinEntry := range actions {
		if pinEntry.Action == "" || !looksLikeSHA(pinEntry.SHA) {
			continue
		}
		index[pinEntry.Action] = pinEntry
	}
	return index
}

func sanitizeLabel(label string) string {
	label = strings.ReplaceAll(label, "#", "")
	label = strings.Join(strings.Fields(label), " ")

	var b strings.Builder
	b.Grow(len(label))
	for _, r := range label {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (a app) httpClientOrDefault() *http.Client {
	if a.httpClient != nil {
		return a.httpClient
	}
	return &http.Client{Timeout: httpTimeout}
}

func (a app) userAgentOrDefault() string {
	if a.userAgent != "" {
		return a.userAgent
	}
	return formatUserAgent(version)
}

func writeFileAtomically(path string, data []byte, perm os.FileMode) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}

	tmpName := tmp.Name()
	closed := false
	defer func() {
		if err != nil {
			if !closed {
				_ = tmp.Close()
			}
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(perm); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	closed = true

	return os.Rename(tmpName, path)
}

func createIssue(ctx context.Context, client githubClient, out io.Writer, actions []missingAction) error {
	var body strings.Builder
	body.WriteString("### Actions\n\n")
	body.WriteString("These actions were resolved live by `gh pin`.\n\n")

	for _, action := range actions {
		_, _ = fmt.Fprintf(&body, "- `%s@%s`\n", action.Action, action.Ref)
	}

	params := map[string]any{
		"title":  "Add actions to pin list",
		"body":   body.String(),
		"labels": []string{"pins"},
	}

	payload, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshalling issue body: %w", err)
	}

	var result struct {
		HTMLURL string `json:"html_url"`
	}

	if err := client.DoWithContext(ctx, http.MethodPost, issueEndpoint, bytes.NewReader(payload), &result); err != nil {
		return fmt.Errorf("creating issue: %w", err)
	}

	if result.HTMLURL == "" {
		return errors.New("creating issue: empty HTML URL from GitHub")
	}

	_, _ = fmt.Fprintf(out, "\nIssue created: %s\n", result.HTMLURL)
	return nil
}
