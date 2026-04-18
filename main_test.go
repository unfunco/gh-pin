package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type stubGitHubClient struct {
	doCalls   []string
	responses map[string]any
}

func (s *stubGitHubClient) DoWithContext(_ context.Context, _, path string, _ io.Reader, response any) error {
	s.doCalls = append(s.doCalls, path)

	payload, ok := s.responses[path]
	if !ok {
		return errors.New("not found")
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, response)
}

func TestActionRepository(t *testing.T) {
	t.Parallel()

	tests := []struct {
		action string
		want   string
		ok     bool
	}{
		{action: "actions/checkout", want: "actions/checkout", ok: true},
		{action: "docker/build-push-action/subpath", want: "docker/build-push-action", ok: true},
		{action: "./local-action", ok: false},
		{action: "docker://alpine:3.20", ok: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.action, func(t *testing.T) {
			t.Parallel()

			got, ok := actionRepository(tt.action)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("actionRepository(%q) = (%q, %t), want (%q, %t)", tt.action, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestParseUsesLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		line       string
		wantPrefix string
		wantAction string
		wantRef    string
		wantOK     bool
	}{
		{
			name:       "plain uses line",
			line:       "      - uses: actions/checkout@v4",
			wantPrefix: "      - uses: ",
			wantAction: "actions/checkout",
			wantRef:    "v4",
			wantOK:     true,
		},
		{
			name:       "quoted uses line",
			line:       `      - uses: "actions/setup-go@v5"`,
			wantPrefix: "      - uses: ",
			wantAction: "actions/setup-go",
			wantRef:    "v5",
			wantOK:     true,
		},
		{
			name:   "not a uses key",
			line:   "      env: uses: actions/checkout@v4",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotPrefix, gotAction, gotRef, gotOK := parseUsesLine(tt.line)
			if gotPrefix != tt.wantPrefix || gotAction != tt.wantAction || gotRef != tt.wantRef || gotOK != tt.wantOK {
				t.Fatalf(
					"parseUsesLine(%q) = (%q, %q, %q, %t), want (%q, %q, %q, %t)",
					tt.line,
					gotPrefix,
					gotAction,
					gotRef,
					gotOK,
					tt.wantPrefix,
					tt.wantAction,
					tt.wantRef,
					tt.wantOK,
				)
			}
		})
	}
}

func TestProcessWorkflowResolvesMissingActionLive(t *testing.T) {
	t.Parallel()

	sha := strings.Repeat("a", 40)
	dir := t.TempDir()
	path := filepath.Join(dir, "ci.yml")
	contents := "jobs:\n  test:\n    steps:\n      - uses: owner/action@v1\n"

	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &stubGitHubClient{
		responses: map[string]any{
			"repos/owner/action/commits/v1": map[string]string{"sha": sha},
		},
	}

	a := app{
		stdout: io.Discard,
		stderr: io.Discard,
		github: client,
	}

	missing, err := a.processWorkflow(context.Background(), path, map[string]pin{})
	if err != nil {
		t.Fatalf("processWorkflow() error = %v", err)
	}

	if !reflect.DeepEqual(missing, []missingAction{{Action: "owner/action", Ref: "v1"}}) {
		t.Fatalf("processWorkflow() missing = %#v", missing)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	wantLine := "- uses: owner/action@" + sha + " # v1"
	if !strings.Contains(string(got), wantLine) {
		t.Fatalf("workflow = %q, want line %q", string(got), wantLine)
	}

	if !reflect.DeepEqual(client.doCalls, []string{"repos/owner/action/commits/v1"}) {
		t.Fatalf("DoWithContext() paths = %#v", client.doCalls)
	}
}

func TestResolveActionKeepsExistingSHA(t *testing.T) {
	t.Parallel()

	sha := strings.Repeat("b", 40)
	client := &stubGitHubClient{}
	a := app{github: client}

	resolved, needsIssue, err := a.resolveAction(context.Background(), map[string]pin{}, "owner/action", "owner/action", sha)
	if err != nil {
		t.Fatalf("resolveAction() error = %v", err)
	}
	if needsIssue {
		t.Fatalf("resolveAction() needsIssue = true, want false")
	}
	if resolved.sha != sha || resolved.source != "already pinned" {
		t.Fatalf("resolveAction() = %#v", resolved)
	}
	if len(client.doCalls) != 0 {
		t.Fatalf("resolveAction() unexpectedly called GitHub: %#v", client.doCalls)
	}
}

func TestLookupCommitSHARejectsInvalidSHA(t *testing.T) {
	t.Parallel()

	client := &stubGitHubClient{
		responses: map[string]any{
			"repos/owner/action/commits/v1": map[string]string{"sha": "not-a-sha"},
		},
	}

	a := app{github: client}
	_, err := a.lookupCommitSHA(context.Background(), "owner/action", "owner/action", "v1")
	if err == nil || !strings.Contains(err.Error(), "invalid SHA") {
		t.Fatalf("lookupCommitSHA() error = %v, want invalid SHA error", err)
	}
}

func TestFormatUsesLineSanitizesLabel(t *testing.T) {
	t.Parallel()

	sha := strings.Repeat("c", 40)
	got := formatUsesLine("- uses: ", "actions/checkout", sha, "  release #1 \n candidate  ")
	want := "- uses: actions/checkout@" + sha + " # release 1 candidate"
	if got != want {
		t.Fatalf("formatUsesLine() = %q, want %q", got, want)
	}
}
