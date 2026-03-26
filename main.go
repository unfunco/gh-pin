package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

// PinsFile represents the top-level structure of the pins JSON file.
type PinsFile struct {
	Actions []Pin `json:"actions"`
}

// Pin represents a pinned GitHub Action with its commit SHA and tag.
type Pin struct {
	Action      string    `json:"action"`
	Tag         string    `json:"tag"`
	SHA         string    `json:"sha"`
	PublishedAt time.Time `json:"published_at"`
}

const pinsURL = "https://unfunco.github.io/toolbox/pins.json"

var usesPattern = regexp.MustCompile(`^(\s*-?\s*uses:\s*)([^@\s]+)@(\S+)(.*)$`)

func main() {
	workflowDir := filepath.Join(".github", "workflows")

	entries, err := os.ReadDir(workflowDir)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error reading workflows directory: %v\n", err)
		os.Exit(1)
	}

	pins, err := fetchPins()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error fetching pins: %v\n", err)
		os.Exit(1)
	}

	pinMap := make(map[string]Pin)
	for _, pin := range pins.Actions {
		pinMap[pin.Action] = pin
	}

	missingSet := make(map[string]struct{})

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}

		path := filepath.Join(workflowDir, name)
		missing, procErr := processWorkflow(path, pinMap)
		if procErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", name, procErr)
		}
		for _, action := range missing {
			missingSet[action] = struct{}{}
		}
	}

	if len(missingSet) == 0 {
		return
	}

	missing := make([]string, 0, len(missingSet))
	for action := range missingSet {
		missing = append(missing, action)
	}
	sort.Strings(missing)

	fmt.Println()
	fmt.Println("The following actions are not in the pin list:")
	for _, action := range missing {
		fmt.Printf("  • %s\n", action)
	}
	fmt.Print("\nWould you like to open an issue to request they be added? [y/N] ")

	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer == "y" || answer == "yes" {
		if err := createIssue(missing); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error creating issue: %v\n", err)
			os.Exit(1)
		}
	}
}

func fetchPins() (*PinsFile, error) {
	resp, err := http.Get(pinsURL)
	if err != nil {
		return nil, fmt.Errorf("fetching pins: %w", err)
	}

	//goland:noinspection GoUnhandledErrorResult
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching pins: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var pins PinsFile
	if err = json.Unmarshal(body, &pins); err != nil {
		return nil, fmt.Errorf("parsing pins: %w", err)
	}

	return &pins, nil
}

func processWorkflow(path string, pins map[string]Pin) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	modified := false
	var missing []string

	for i, line := range lines {
		matches := usesPattern.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		prefix := matches[1] // leading whitespace + "uses: "
		action := matches[2] // e.g. "actions/checkout"
		oldRef := matches[3] // e.g. "v6" or a SHA

		pin, ok := pins[action]
		if !ok {
			_, _ = fmt.Fprintf(os.Stderr, "  ⚠ %s: %s not found in pins\n", filepath.Base(path), action)
			missing = append(missing, action)
			continue
		}

		newLine := fmt.Sprintf("%s%s@%s # %s", prefix, action, pin.SHA, pin.Tag)
		if lines[i] != newLine {
			fmt.Printf("  ✓ %s: %s@%s → %s\n", filepath.Base(path), action, oldRef, pin.Tag)
			lines[i] = newLine
			modified = true
		}
	}

	if modified {
		return missing, os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
	}

	return missing, nil
}

func createIssue(actions []string) error {
	client, err := api.DefaultRESTClient()
	if err != nil {
		return fmt.Errorf("creating API client: %w", err)
	}

	body := "### Actions\n\n" + strings.Join(actions, "\n")

	params := map[string]any{
		"title":  "Add actions to pin list",
		"body":   body,
		"labels": []string{"pins"},
	}

	payload, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshalling issue body: %w", err)
	}

	var result struct {
		HTMLURL string `json:"html_url"`
	}

	if err = client.Post("repos/unfunco/toolbox/issues", bytes.NewReader(payload), &result); err != nil {
		return fmt.Errorf("creating issue: %w", err)
	}

	fmt.Printf("\nIssue created: %s\n", result.HTMLURL)
	return nil
}
