// mock-claude is a fake claude binary for integration testing.
// It reads a prompt from stdin, matches against patterns configured
// via MOCK_CLAUDE_CONFIG (path to JSON file), and returns the matching
// canned response. Optionally creates a git commit for exec-mode testing.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// MockResponse defines a pattern→response mapping.
type MockResponse struct {
	Match    string `json:"match"`    // Substring to match in the prompt
	Response string `json:"response"` // What to write to stdout
	Commit   bool   `json:"commit"`   // If true, create a git commit before responding
}

func main() {
	configPath := os.Getenv("MOCK_CLAUDE_CONFIG")
	if configPath == "" {
		fmt.Fprintln(os.Stderr, "MOCK_CLAUDE_CONFIG not set")
		os.Exit(1)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read config %s: %v\n", configPath, err)
		os.Exit(1)
	}

	var responses []MockResponse
	if err := json.Unmarshal(data, &responses); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse config: %v\n", err)
		os.Exit(1)
	}

	// Read prompt from stdin
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read stdin: %v\n", err)
		os.Exit(1)
	}
	prompt := string(input)

	// Match first pattern found in prompt
	for _, r := range responses {
		if strings.Contains(prompt, r.Match) {
			if r.Commit {
				if err := createMockCommit(); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to create mock commit: %v\n", err)
					os.Exit(1)
				}
			}
			fmt.Print(r.Response)
			return
		}
	}

	// No match — print nothing (empty response)
	fmt.Fprintln(os.Stderr, "mock-claude: no matching pattern found")
}

// createMockCommit creates a unique file and commits it.
// Uses an incrementing counter to ensure unique filenames across invocations.
func createMockCommit() error {
	// Read/increment counter
	counterPath := filepath.Join(os.TempDir(), "mock-claude-counter")
	counterData, _ := os.ReadFile(counterPath)
	counter, _ := strconv.Atoi(strings.TrimSpace(string(counterData)))
	counter++
	os.WriteFile(counterPath, []byte(strconv.Itoa(counter)), 0644)

	// Create a text file (not .go to avoid breaking go vet/test)
	filename := fmt.Sprintf("impl-%d.txt", counter)
	content := fmt.Sprintf("Mock implementation %d\n", counter)
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		return fmt.Errorf("write %s: %w", filename, err)
	}

	// Stage and commit
	if out, err := exec.Command("git", "add", filename).CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %w\n%s", err, out)
	}
	commitMsg := fmt.Sprintf("feat: implement item %d", counter)
	if out, err := exec.Command("git", "commit", "-m", commitMsg).CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %w\n%s", err, out)
	}

	return nil
}
