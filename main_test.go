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

func (s *stubGitHubClient) Post(_ string, _ io.Reader, _ any) error {
	return nil
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
