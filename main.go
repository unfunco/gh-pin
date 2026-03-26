package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
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

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}

		path := filepath.Join(workflowDir, name)
		if err := processWorkflow(path, pinMap); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", name, err)
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

func processWorkflow(path string, pins map[string]Pin) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	modified := false

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
		return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
	}

	return nil
}
