package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- Build infrastructure ---

var (
	integrationOnce sync.Once
	integrationBin  string // path to compiled scrip binary
	mockClaudeBin   string // path to compiled mock-claude binary (named "claude")
	integrationErr  error
)

func ensureIntegrationBinaries(t *testing.T) (scripPath, mockDir string) {
	t.Helper()
	integrationOnce.Do(func() {
		tmpDir, err := os.MkdirTemp("", "scrip-integration-*")
		if err != nil {
			integrationErr = fmt.Errorf("create temp dir: %w", err)
			return
		}

		// Build scrip binary
		integrationBin = filepath.Join(tmpDir, "scrip")
		cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", integrationBin, ".")
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			integrationErr = fmt.Errorf("build scrip: %w", err)
			return
		}

		// Build mock-claude as "claude" so it's found by PATH lookup
		mockClaudeBin = filepath.Join(tmpDir, "claude")
		cmd = exec.Command("go", "build", "-o", mockClaudeBin, "./testdata/mock-claude")
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			integrationErr = fmt.Errorf("build mock-claude: %w", err)
			return
		}
	})

	if integrationErr != nil {
		t.Fatalf("Integration setup failed: %v", integrationErr)
	}
	return integrationBin, filepath.Dir(mockClaudeBin)
}

// --- Mock config ---

type mockResponse struct {
	Match    string `json:"match"`
	Response string `json:"response"`
	Commit   bool   `json:"commit"`
}

func writeMockConfig(t *testing.T, responses []mockResponse) string {
	t.Helper()
	data, err := json.Marshal(responses)
	if err != nil {
		t.Fatalf("marshal mock config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "mock-config.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write mock config: %v", err)
	}
	return path
}

// --- Test project setup ---

func setupIntegrationProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Initialize git repo
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "test@scrip.dev"},
		{"config", "user.name", "Scrip Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// go.mod
	writeFile(t, dir, "go.mod", "module example.com/integration-test\n\ngo 1.21\n")

	// main.go — minimal valid Go program
	writeFile(t, dir, "main.go", `package main

func main() {}

func Add(a, b int) int { return a + b }
`)

	// main_test.go — minimal passing test
	writeFile(t, dir, "main_test.go", `package main

import "testing"

func TestAdd(t *testing.T) {
	if got := Add(1, 2); got != 3 {
		t.Errorf("Add(1,2) = %d, want 3", got)
	}
}
`)

	// Initial commit
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "initial commit")

	return dir
}

func writeFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = integrationEnv(dir, "")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// --- Environment ---

func integrationEnv(projectDir, mockConfigPath string) []string {
	env := os.Environ()
	env = append(env,
		"GIT_AUTHOR_NAME=Scrip Test",
		"GIT_AUTHOR_EMAIL=test@scrip.dev",
		"GIT_COMMITTER_NAME=Scrip Test",
		"GIT_COMMITTER_EMAIL=test@scrip.dev",
	)
	if mockConfigPath != "" {
		env = append(env, "MOCK_CLAUDE_CONFIG="+mockConfigPath)
	}
	return env
}

func envWithMockPath(base []string, mockDir string) []string {
	// Prepend mockDir to PATH so "claude" resolves to mock binary
	for i, e := range base {
		if strings.HasPrefix(e, "PATH=") {
			base[i] = "PATH=" + mockDir + ":" + e[5:]
			return base
		}
	}
	return append(base, "PATH="+mockDir)
}

// --- Command runners ---

func runCmd(t *testing.T, bin, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("scrip %v failed (exit %d):\nSTDOUT: %s\nSTDERR: %s",
			args, cmd.ProcessState.ExitCode(), stdout.String(), stderr.String())
	}
	return stdout.String()
}

// runCmdExpect runs a scrip command with expect-style stdin responses.
// When a pattern is found in stdout/stderr output, the corresponding
// response is written to stdin. Each pattern fires at most once.
func runCmdExpect(t *testing.T, bin, dir string, env []string, responses map[string]string, timeout time.Duration, args ...string) string {
	t.Helper()

	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = env

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start scrip %v: %v", args, err)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	var mu sync.Mutex
	respondedPatterns := make(map[string]bool)

	readPipe := func(pipe io.Reader, buf *bytes.Buffer) {
		chunk := make([]byte, 256)
		var line strings.Builder
		for {
			n, readErr := pipe.Read(chunk)
			if n > 0 {
				buf.Write(chunk[:n])
				for _, b := range chunk[:n] {
					if b == '\n' {
						line.Reset()
					} else {
						line.WriteByte(b)
						partial := line.String()
						mu.Lock()
						for pattern, resp := range responses {
							if !respondedPatterns[pattern] && strings.Contains(partial, pattern) {
								respondedPatterns[pattern] = true
								time.Sleep(50 * time.Millisecond)
								io.WriteString(stdinPipe, resp+"\n")
								line.Reset()
								mu.Unlock()
								goto nextByte
							}
						}
						mu.Unlock()
					}
				nextByte:
				}
			}
			if readErr != nil {
				break
			}
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); readPipe(stdoutPipe, &stdoutBuf) }()
	go func() { defer wg.Done(); readPipe(stderrPipe, &stderrBuf) }()

	done := make(chan error, 1)
	go func() {
		wg.Wait()
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("scrip %v failed:\nSTDOUT: %s\nSTDERR: %s\nError: %v",
				args, stdoutBuf.String(), stderrBuf.String(), err)
		}
	case <-time.After(timeout):
		cmd.Process.Kill()
		t.Fatalf("scrip %v timed out after %v:\nSTDOUT: %s\nSTDERR: %s",
			args, timeout, stdoutBuf.String(), stderrBuf.String())
	}

	return stdoutBuf.String()
}

// --- Mock responses ---

const mockPlanDraft = `---
feature: test-feat
created: 2026-01-01T00:00:00Z
item_count: 1
---

# Test Feature

## Items

1. **Add test endpoint**
   - Acceptance: GET /test returns 200 with body "ok"
`

const mockVerifyPass = "<scrip>VERIFY_PASS</scrip>\n"

const mockExecDone = "<scrip>DONE</scrip>\n"

const mockSummary = `<scrip>SUMMARY_START</scrip>
# test-feat

Test feature implemented successfully.

## Changes
- Added test endpoint
<scrip>SUMMARY_END</scrip>
`

// --- Integration Tests ---

// TestPrepPlanExecLand exercises the full scrip lifecycle with a mock claude provider:
// prep → plan → exec → land. Verifies that artifacts are created and cleaned up correctly.
func TestPrepPlanExecLand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	scripBin, mockDir := ensureIntegrationBinaries(t)
	projectDir := setupIntegrationProject(t)

	// Clean up mock counter between tests
	counterPath := filepath.Join(os.TempDir(), "mock-claude-counter")
	os.Remove(counterPath)
	t.Cleanup(func() { os.Remove(counterPath) })

	configPath := writeMockConfig(t, []mockResponse{
		// Plan create: return valid plan draft
		{Match: "planning agent", Response: mockPlanDraft},
		// Plan verify: pass
		{Match: "adversarial verification", Response: mockVerifyPass},
		// Exec build: commit + DONE
		{Match: "autonomous coding agent", Response: mockExecDone, Commit: true},
		// Land analyze: pass
		{Match: "Landing Analysis", Response: mockVerifyPass},
		// Land summary: return summary with markers
		{Match: "Feature Summary Generation", Response: mockSummary},
	})

	env := envWithMockPath(integrationEnv(projectDir, configPath), mockDir)

	// --- Phase 1: Prep ---
	t.Log("Phase 1: scrip prep")
	runCmd(t, scripBin, projectDir, env, "prep")

	// Verify config created
	configJSON := filepath.Join(projectDir, ".scrip", "config.json")
	if _, err := os.Stat(configJSON); os.IsNotExist(err) {
		t.Fatal("scrip prep did not create .scrip/config.json")
	}

	// --- Phase 2: Plan ---
	t.Log("Phase 2: scrip plan")
	runCmdExpect(t, scripBin, projectDir, env,
		map[string]string{
			"Finalize?": "yes",
		},
		2*time.Minute,
		"plan", "test-feat", "Add test endpoint",
	)

	// Find feature dir
	featureDir := findFeatureDir(t, projectDir, "test-feat")

	// Verify plan.md was created
	planMd := filepath.Join(featureDir, "plan.md")
	if _, err := os.Stat(planMd); os.IsNotExist(err) {
		t.Fatal("scrip plan did not create plan.md")
	}

	// Verify plan.jsonl was created
	planJsonl := filepath.Join(featureDir, "plan.jsonl")
	if _, err := os.Stat(planJsonl); os.IsNotExist(err) {
		t.Fatal("scrip plan did not create plan.jsonl")
	}

	// Verify we're on the feature branch
	branch := gitBranchName(t, projectDir)
	if branch != "plan/test-feat" {
		t.Errorf("expected branch plan/test-feat, got %s", branch)
	}

	// --- Phase 3: Exec ---
	t.Log("Phase 3: scrip exec")
	runCmd(t, scripBin, projectDir, env, "exec", "test-feat")

	// Verify progress.jsonl has events
	progressJsonl := filepath.Join(featureDir, "progress.jsonl")
	events, err := LoadProgressEvents(progressJsonl)
	if err != nil {
		t.Fatalf("load progress events: %v", err)
	}

	hasExecStart := false
	hasExecEnd := false
	hasItemDone := false
	for _, e := range events {
		switch e.Event {
		case ProgressExecStart:
			hasExecStart = true
		case ProgressExecEnd:
			hasExecEnd = true
		case ProgressItemDone:
			if e.Status == "passed" {
				hasItemDone = true
			}
		}
	}
	if !hasExecStart {
		t.Error("progress.jsonl missing exec_start event")
	}
	if !hasExecEnd {
		t.Error("progress.jsonl missing exec_end event")
	}
	if !hasItemDone {
		t.Error("progress.jsonl missing item_done(passed) event")
	}

	// Verify progress.md was written
	progressMd := filepath.Join(featureDir, "progress.md")
	if _, err := os.Stat(progressMd); os.IsNotExist(err) {
		t.Error("scrip exec did not create progress.md")
	}

	// --- Phase 4: Land ---
	t.Log("Phase 4: scrip land")

	// Land will try to push; ignore push failure (no remote)
	cmd := exec.Command(scripBin, "land", "test-feat")
	cmd.Dir = projectDir
	cmd.Env = env
	var landOut, landErr bytes.Buffer
	cmd.Stdout = &landOut
	cmd.Stderr = &landErr
	err = cmd.Run()

	// Land should succeed even though push fails (push is non-fatal)
	if err != nil {
		t.Fatalf("scrip land failed:\nSTDOUT: %s\nSTDERR: %s\nError: %v",
			landOut.String(), landErr.String(), err)
	}

	// Verify summary.md exists
	summaryMd := filepath.Join(featureDir, "summary.md")
	if _, err := os.Stat(summaryMd); os.IsNotExist(err) {
		t.Error("scrip land did not create summary.md")
	}

	// Verify summary content
	summaryContent, _ := os.ReadFile(summaryMd)
	if !strings.Contains(string(summaryContent), "test-feat") {
		t.Error("summary.md does not contain feature name")
	}

	// Verify plan.md was purged
	if _, err := os.Stat(planMd); err == nil {
		t.Error("scrip land did not purge plan.md")
	}

	// Verify land_passed event in progress.jsonl
	events, _ = LoadProgressEvents(progressJsonl)
	hasLandPassed := false
	for _, e := range events {
		if e.Event == ProgressLandPassed {
			hasLandPassed = true
		}
	}
	if !hasLandPassed {
		t.Error("progress.jsonl missing land_passed event")
	}

	t.Log("Full lifecycle completed successfully")
}

// --- Helpers ---

func findFeatureDir(t *testing.T, projectDir, feature string) string {
	t.Helper()
	scripDir := filepath.Join(projectDir, ".scrip")
	entries, err := os.ReadDir(scripDir)
	if err != nil {
		t.Fatalf("read .scrip: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Feature dirs are YYYY-MM-DD-<feature>
		name := e.Name()
		parts := strings.SplitN(name, "-", 4)
		if len(parts) >= 4 && strings.EqualFold(parts[3], feature) {
			return filepath.Join(scripDir, name)
		}
	}
	t.Fatalf("no feature directory found for %q in %s", feature, scripDir)
	return ""
}

func gitBranchName(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git branch: %v", err)
	}
	return strings.TrimSpace(string(out))
}
