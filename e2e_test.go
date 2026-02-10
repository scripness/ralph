//go:build e2e

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// ralphBin is the path to the compiled ralph binary, set in TestMain.
var ralphBin string

func TestMain(m *testing.M) {
	// Build ralph binary into a temp directory
	tmpDir, err := os.MkdirTemp("", "ralph-e2e-*")
	if err != nil {
		log.Fatalf("Failed to create temp dir: %v", err)
	}

	ralphBin = filepath.Join(tmpDir, "ralph")
	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", ralphBin, ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tmpDir)
		log.Fatalf("Failed to build ralph: %v", err)
	}

	// Verify prerequisites
	for _, bin := range []string{"claude", "bun", "git", "gh"} {
		if _, err := exec.LookPath(bin); err != nil {
			os.RemoveAll(tmpDir)
			log.Fatalf("E2E tests require %s in PATH", bin)
		}
	}

	code := m.Run()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

// processTracker keeps track of running ralph subprocesses for cleanup on test abort.
type processTracker struct {
	mu    sync.Mutex
	procs map[int]*os.Process
}

var activeProcs = &processTracker{procs: make(map[int]*os.Process)}

func (pt *processTracker) track(p *os.Process) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.procs[p.Pid] = p
}

func (pt *processTracker) untrack(pid int) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	delete(pt.procs, pid)
}

func (pt *processTracker) killAll() {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	for pid := range pt.procs {
		syscall.Kill(-pid, syscall.SIGTERM)
	}
	time.Sleep(1 * time.Second)
	for pid := range pt.procs {
		syscall.Kill(-pid, syscall.SIGKILL)
	}
	pt.procs = make(map[int]*os.Process)
}

// testEnv holds shared state across test phases.
type testEnv struct {
	ralphBin    string
	projectDir  string
	featureName string
	artifactDir string // persistent output directory for this run
}

// ralphResult captures the output and exit code of a ralph invocation.
type ralphResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

func (r ralphResult) Success() bool {
	return r.ExitCode == 0
}

func (r ralphResult) Combined() string {
	return r.Stdout + r.Stderr
}

// buildEnv returns environment variables for subprocess isolation.
func buildEnv(projectDir string) []string {
	env := os.Environ()
	// Ensure git identity is set for commits
	env = append(env,
		"GIT_AUTHOR_NAME=Ralph E2E Test",
		"GIT_AUTHOR_EMAIL=ralph-e2e@test.local",
		"GIT_COMMITTER_NAME=Ralph E2E Test",
		"GIT_COMMITTER_EMAIL=ralph-e2e@test.local",
	)
	return env
}

// initArtifactDir creates the artifact directory for this run.
// Directory: e2e-runs/<timestamp>/ under the ralph project root.
// Override with RALPH_E2E_ARTIFACT_DIR env var.
func initArtifactDir(t *testing.T) string {
	t.Helper()

	base := os.Getenv("RALPH_E2E_ARTIFACT_DIR")
	if base == "" {
		base = "e2e-runs"
	}

	timestamp := time.Now().Format("2006-01-02T15-04-05")
	dir := filepath.Join(base, timestamp)

	for _, sub := range []string{"phases", "stories", "logs"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			t.Fatalf("Failed to create artifact dir %s: %v", filepath.Join(dir, sub), err)
		}
	}

	t.Logf("Artifact directory: %s", dir)
	return dir
}

// savePhaseOutput writes a ralph result to the phases/ artifact directory.
func savePhaseOutput(env *testEnv, phase string, result ralphResult) {
	if env.artifactDir == "" {
		return
	}
	path := filepath.Join(env.artifactDir, "phases", phase+".txt")

	var buf strings.Builder
	fmt.Fprintf(&buf, "=== %s ===\n", phase)
	fmt.Fprintf(&buf, "Exit code: %d\n", result.ExitCode)
	if result.Err != nil {
		fmt.Fprintf(&buf, "Error: %v\n", result.Err)
	}
	fmt.Fprintf(&buf, "\n--- STDOUT ---\n%s\n", result.Stdout)
	fmt.Fprintf(&buf, "\n--- STDERR ---\n%s\n", result.Stderr)

	os.WriteFile(path, []byte(buf.String()), 0644)
}

// saveArtifact writes arbitrary content to the artifact directory.
func saveArtifact(env *testEnv, relPath, content string) {
	if env.artifactDir == "" {
		return
	}
	path := filepath.Join(env.artifactDir, relPath)
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte(content), 0644)
}

// copyArtifact copies a file into the artifact directory.
func copyArtifact(env *testEnv, relPath, srcPath string) {
	if env.artifactDir == "" {
		return
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return
	}
	dst := filepath.Join(env.artifactDir, relPath)
	os.MkdirAll(filepath.Dir(dst), 0755)
	os.WriteFile(dst, data, 0644)
}

// saveSkippedPhase records that a phase was skipped and why.
func saveSkippedPhase(env *testEnv, phase, reason string) {
	saveArtifact(env, filepath.Join("phases", phase+".txt"),
		fmt.Sprintf("=== %s ===\nSKIPPED: %s\n", phase, reason))
}

// runRalph executes ralph with no stdin and returns the result.
func runRalph(t *testing.T, dir string, args ...string) ralphResult {
	t.Helper()
	return runRalphWithTimeout(t, dir, 2*time.Minute, args...)
}

// runRalphWithTimeout executes ralph with no stdin and a custom timeout.
func runRalphWithTimeout(t *testing.T, dir string, timeout time.Duration, args ...string) ralphResult {
	t.Helper()

	cmd := exec.Command(ralphBin, args...)
	cmd.Dir = dir
	cmd.Env = buildEnv(dir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return ralphResult{Err: err, ExitCode: -1}
	}
	activeProcs.track(cmd.Process)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		activeProcs.untrack(cmd.Process.Pid)
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}
		return ralphResult{
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			ExitCode: exitCode,
			Err:      err,
		}
	case <-time.After(timeout):
		syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		time.Sleep(2 * time.Second)
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		activeProcs.untrack(cmd.Process.Pid)
		return ralphResult{
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			ExitCode: -1,
			Err:      fmt.Errorf("command timed out after %v", timeout),
		}
	}
}

// runRalphWithStdin pipes fixed stdin content to ralph.
func runRalphWithStdin(t *testing.T, dir string, stdin string, timeout time.Duration, args ...string) ralphResult {
	t.Helper()

	cmd := exec.Command(ralphBin, args...)
	cmd.Dir = dir
	cmd.Env = buildEnv(dir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdin = strings.NewReader(stdin)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return ralphResult{Err: err, ExitCode: -1}
	}
	activeProcs.track(cmd.Process)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		activeProcs.untrack(cmd.Process.Pid)
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}
		return ralphResult{
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			ExitCode: exitCode,
			Err:      err,
		}
	case <-time.After(timeout):
		syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		time.Sleep(2 * time.Second)
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		activeProcs.untrack(cmd.Process.Pid)
		return ralphResult{
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			ExitCode: -1,
			Err:      fmt.Errorf("command timed out after %v", timeout),
		}
	}
}

// promptResponse defines an expect-style stdin interaction.
type promptResponse struct {
	pattern  string // substring to match in combined stdout+stderr
	response string // what to write to stdin when matched
	used bool // track if already triggered (each response fires at most once)
}

// runRalphInteractive runs ralph with expect-style stdin interaction.
// It reads stdout/stderr byte-by-byte (via raw Read), checking partial lines
// for prompt patterns. This is critical because prompts like "Enter choice (1-6): "
// and "  > " don't end with newlines — bufio.Scanner would deadlock waiting for \n
// while ralph blocks waiting for stdin.
func runRalphInteractive(t *testing.T, dir string, timeout time.Duration,
	responses []promptResponse, args ...string) ralphResult {
	t.Helper()

	cmd := exec.Command(ralphBin, args...)
	cmd.Dir = dir
	cmd.Env = buildEnv(dir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return ralphResult{Err: fmt.Errorf("stdin pipe: %w", err), ExitCode: -1}
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return ralphResult{Err: fmt.Errorf("stdout pipe: %w", err), ExitCode: -1}
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return ralphResult{Err: fmt.Errorf("stderr pipe: %w", err), ExitCode: -1}
	}

	if err := cmd.Start(); err != nil {
		return ralphResult{Err: fmt.Errorf("start: %w", err), ExitCode: -1}
	}
	activeProcs.track(cmd.Process)

	var stdoutBuf, stderrBuf bytes.Buffer
	var mu sync.Mutex

	// readStream reads from a pipe in small chunks, processing bytes incrementally.
	// It checks partial lines (before \n) against prompt patterns so that prompts
	// without trailing newlines (e.g. "Enter choice (1-6): ") are matched immediately.
	// After a match on a partial line, further pattern checks are suppressed until
	// the next newline to prevent substring false positives (e.g. "Test" in "Typecheck").
	readStream := func(pipe io.Reader, buf *bytes.Buffer, label string) {
		chunk := make([]byte, 256)
		var currentLine strings.Builder
		matchedOnCurrentLine := false
		for {
			n, readErr := pipe.Read(chunk)
			if n > 0 {
				buf.Write(chunk[:n])
				for _, b := range chunk[:n] {
					if b == '\n' {
						line := currentLine.String()
						t.Logf("%s: %s", label, line)
						currentLine.Reset()
						matchedOnCurrentLine = false
					} else {
						currentLine.WriteByte(b)
						if !matchedOnCurrentLine {
							partial := currentLine.String()
							mu.Lock()
							matchIdx := -1
							for i := range responses {
								if responses[i].used {
									continue
								}
								if strings.Contains(partial, responses[i].pattern) {
									matchIdx = i
									responses[i].used = true
									break
								}
							}
							mu.Unlock()
							if matchIdx >= 0 {
								matchedOnCurrentLine = true
								time.Sleep(100 * time.Millisecond)
								io.WriteString(stdinPipe, responses[matchIdx].response+"\n")
								t.Logf("RESPOND: matched %q -> sent %q",
									responses[matchIdx].pattern, responses[matchIdx].response)
							}
						}
					}
				}
			}
			if readErr != nil {
				if currentLine.Len() > 0 {
					t.Logf("%s: %s", label, currentLine.String())
				}
				break
			}
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		readStream(stdoutPipe, &stdoutBuf, "STDOUT")
	}()
	go func() {
		defer wg.Done()
		readStream(stderrPipe, &stderrBuf, "STDERR")
	}()

	done := make(chan error, 1)
	go func() {
		wg.Wait()
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		activeProcs.untrack(cmd.Process.Pid)
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}
		return ralphResult{
			Stdout:   stdoutBuf.String(),
			Stderr:   stderrBuf.String(),
			ExitCode: exitCode,
			Err:      err,
		}
	case <-time.After(timeout):
		stdinPipe.Close()
		syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		time.Sleep(2 * time.Second)
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		activeProcs.untrack(cmd.Process.Pid)
		wg.Wait()
		return ralphResult{
			Stdout:   stdoutBuf.String(),
			Stderr:   stderrBuf.String(),
			ExitCode: -1,
			Err:      fmt.Errorf("command timed out after %v", timeout),
		}
	}
}

// loadPRD loads and parses a prd.json file.
func loadPRD(t *testing.T, path string) *PRD {
	t.Helper()
	prd, err := LoadPRD(path)
	if err != nil {
		t.Fatalf("Failed to load PRD from %s: %v", path, err)
	}
	return prd
}

// loadConfig loads and parses a ralph.config.json file.
func loadConfig(t *testing.T, path string) *RalphConfig {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read config from %s: %v", path, err)
	}
	var cfg RalphConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Failed to parse config from %s: %v", path, err)
	}
	return &cfg
}

// findFeatureDir scans .ralph/ for a matching feature directory.
// Supports both YYYY-MM-DD-<feature> and YYYYMMDD-<feature> formats.
func findFeatureDir(t *testing.T, projectDir, feature string) string {
	t.Helper()
	ralphDir := filepath.Join(projectDir, ".ralph")
	entries, err := os.ReadDir(ralphDir)
	if err != nil {
		t.Fatalf("Failed to read .ralph dir: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Feature dirs are YYYY-MM-DD-<feature>
		parts := strings.SplitN(name, "-", 4)
		if len(parts) >= 4 && strings.EqualFold(parts[3], feature) {
			return filepath.Join(ralphDir, name)
		}
		// Also try YYYYMMDD-<feature> format
		if len(name) > 9 && name[8] == '-' {
			suffix := name[9:]
			if strings.EqualFold(suffix, feature) {
				return filepath.Join(ralphDir, name)
			}
		}
	}
	t.Fatalf("No feature directory found for %q in %s", feature, ralphDir)
	return ""
}

// gitBranch returns the current git branch name.
func gitBranch(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git branch: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// gitLog returns the last n commits as one-line summaries.
func gitLog(t *testing.T, dir string, n int) string {
	t.Helper()
	cmd := exec.Command("git", "log", fmt.Sprintf("-%d", n), "--oneline")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// gitCommitCount returns the number of commits on the current branch since diverging from main.
func gitCommitCount(t *testing.T, dir string) int {
	t.Helper()
	cmd := exec.Command("git", "rev-list", "--count", "HEAD", "--not", "main")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	var count int
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &count)
	return count
}

// gitDiff returns the full diff from main to HEAD.
func gitDiff(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "diff", "main...HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return fmt.Sprintf("(git diff failed: %v)", err)
	}
	return string(out)
}

// gitDiffStat returns the diff stat from main to HEAD.
func gitDiffStat(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "diff", "--stat", "main...HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return fmt.Sprintf("(git diff --stat failed: %v)", err)
	}
	return string(out)
}

// gitWorkingTreeStatus returns the output of git status --short.
func gitWorkingTreeStatus(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "status", "--short")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return fmt.Sprintf("(git status failed: %v)", err)
	}
	return string(out)
}

// runCmd is a shorthand for running a shell command in a directory.
func runCmd(t *testing.T, dir string, timeout time.Duration, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatalf("%s %v: start failed: %v", name, args, err)
	}
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("%s %v: %v", name, args, err)
		}
	case <-time.After(timeout):
		cmd.Process.Kill()
		t.Fatalf("%s %v: timed out after %v", name, args, timeout)
	}
}

// countStories returns (total, passed, blocked, pending, withRetries) from a PRD.
func countStories(prd *PRD) (total, passed, blocked, pending, withRetries int) {
	total = len(prd.UserStories)
	for _, s := range prd.UserStories {
		switch {
		case s.Passes:
			passed++
		case s.Blocked:
			blocked++
		default:
			pending++
		}
		if s.Retries > 0 {
			withRetries++
		}
	}
	return
}

// hasUIStory returns true if any story has a "ui" tag.
func hasUIStory(prd *PRD) bool {
	for _, s := range prd.UserStories {
		for _, tag := range s.Tags {
			if strings.EqualFold(tag, "ui") {
				return true
			}
		}
	}
	return false
}

// storyStatus returns a human-readable status string.
func storyStatus(s UserStory) string {
	if s.Passes {
		return "PASSED"
	}
	if s.Blocked {
		return "BLOCKED"
	}
	return "PENDING"
}

// firstStoryID returns the ID of the first story in the PRD, or "" if none.
func firstStoryID(prd *PRD) string {
	if len(prd.UserStories) > 0 {
		return prd.UserStories[0].ID
	}
	return ""
}

// assertLogEventExists checks that at least one event of the given type exists in the log events.
func assertLogEventExists(t *testing.T, events []Event, eventType EventType, label string) {
	t.Helper()
	for _, ev := range events {
		if ev.Type == eventType {
			return
		}
	}
	t.Errorf("Expected log event %q (%s) not found in JSONL logs", eventType, label)
}

// countLogEventType counts events of a specific type.
func countLogEventType(events []Event, eventType EventType) int {
	count := 0
	for _, ev := range events {
		if ev.Type == eventType {
			count++
		}
	}
	return count
}

// TestE2E runs the full end-to-end test using real Claude, real project, no mocking.
func TestE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	env := &testEnv{
		ralphBin:    ralphBin,
		featureName: "certificate-search",
	}

	// Create persistent artifact directory for this run
	env.artifactDir = initArtifactDir(t)

	// Kill any orphaned processes on test abort
	t.Cleanup(func() {
		activeProcs.killAll()
	})

	// ==================== Phase 0: Smoke Tests ====================
	t.Run("Phase0_SmokeTests", func(t *testing.T) {
		phase0SmokeTests(t, env)
	})

	// ==================== Phase 1: Project Setup ====================
	t.Run("Phase1_ProjectSetup", func(t *testing.T) {
		phase1Setup(t, env)
	})
	if t.Failed() {
		writeReport(t, env)
		t.Fatal("Phase 1 failed — cannot proceed")
	}

	// ==================== Phase 2: ralph init ====================
	t.Run("Phase2_RalphInit", func(t *testing.T) {
		phase2Init(t, env)
	})
	if t.Failed() {
		writeReport(t, env)
		t.Fatal("Phase 2 failed — cannot proceed")
	}

	// ==================== Phase 3: Config Enhancement ====================
	t.Run("Phase3_ConfigEnhancement", func(t *testing.T) {
		phase3ConfigEnhancement(t, env)
	})
	if t.Failed() {
		writeReport(t, env)
		t.Fatal("Phase 3 failed — cannot proceed")
	}

	// ==================== Phase 4: ralph doctor ====================
	t.Run("Phase4_Doctor", func(t *testing.T) {
		phase4Doctor(t, env)
	})

	// ==================== Phase 5: ralph prd (Create + Finalize) ====================
	t.Run("Phase5_PrdCreate", func(t *testing.T) {
		phase5PrdCreate(t, env)
	})
	if t.Failed() {
		writeReport(t, env)
		t.Fatal("Phase 5 failed — cannot proceed")
	}

	// ==================== Phase 6: Pre-Run Checks ====================
	t.Run("Phase6_PreRunChecks", func(t *testing.T) {
		phase6PreRunChecks(t, env)
	})

	// ==================== Phase 7: ralph run (First Run) ====================
	t.Run("Phase7_FirstRun", func(t *testing.T) {
		phase7FirstRun(t, env)
	})

	// ==================== Phase 8: Post-Run Analysis ====================
	t.Run("Phase8_PostRunAnalysis", func(t *testing.T) {
		phase8PostRunAnalysis(t, env)
	})

	// ==================== Phase 9: PRD Refinement (conditional) ====================
	t.Run("Phase9_PrdRefine", func(t *testing.T) {
		phase9PrdRefine(t, env)
	})

	// ==================== Phase 10: Second Run (conditional) ====================
	t.Run("Phase10_SecondRun", func(t *testing.T) {
		phase10SecondRun(t, env)
	})

	// ==================== Phase 11: ralph status + logs + resources ====================
	t.Run("Phase11_StatusLogsResources", func(t *testing.T) {
		phase11StatusLogsResources(t, env)
	})

	// ==================== Phase 12: ralph verify (conditional) ====================
	t.Run("Phase12_Verify", func(t *testing.T) {
		phase12Verify(t, env)
	})

	// ==================== Phase 13: Post-Run Doctor ====================
	t.Run("Phase13_PostRunDoctor", func(t *testing.T) {
		phase13PostRunDoctor(t, env)
	})

	// ==================== Phase 14: Comprehensive Report ====================
	t.Run("Phase14_Report", func(t *testing.T) {
		writeReport(t, env)
	})
}

// phase0SmokeTests validates basic CLI operations before touching any project.
func phase0SmokeTests(t *testing.T, env *testEnv) {
	// ralph --help
	result := runRalphWithTimeout(t, ".", 10*time.Second, "--help")
	savePhaseOutput(env, "00-help", result)
	if !result.Success() {
		t.Errorf("ralph --help failed (exit %d)", result.ExitCode)
	} else if !strings.Contains(result.Combined(), "Usage: ralph") {
		t.Errorf("ralph --help missing usage text")
	}

	// ralph --version
	result = runRalphWithTimeout(t, ".", 10*time.Second, "--version")
	savePhaseOutput(env, "00-version", result)
	if !result.Success() {
		t.Errorf("ralph --version failed (exit %d)", result.ExitCode)
	} else if !strings.Contains(result.Combined(), "ralph v") {
		t.Errorf("ralph --version missing version string")
	}

	// ralph nonexistent — unknown command
	result = runRalphWithTimeout(t, ".", 10*time.Second, "nonexistent-command")
	savePhaseOutput(env, "00-unknown-cmd", result)
	if result.ExitCode != 1 {
		t.Errorf("ralph unknown-command should exit 1, got %d", result.ExitCode)
	} else if !strings.Contains(result.Combined(), "Unknown command") {
		t.Errorf("ralph unknown-command missing 'Unknown command' message")
	}

	t.Log("Smoke tests passed: --help, --version, unknown command")
}

// phase1Setup clones warrantycert, installs deps, sets up database, installs Playwright.
func phase1Setup(t *testing.T, env *testEnv) {
	tmpDir, err := os.MkdirTemp("", "ralph-e2e-project-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	env.projectDir = tmpDir
	t.Logf("Project directory: %s", env.projectDir)
	saveArtifact(env, "project-dir.txt", env.projectDir+"\n")

	// Cleanup on success only — preserve on failure for post-mortem
	t.Cleanup(func() {
		if !t.Failed() {
			// Check top-level test status by looking at PRD
			prdJsonPath := ""
			featureDir := filepath.Join(env.projectDir, ".ralph")
			if entries, err := os.ReadDir(featureDir); err == nil {
				for _, e := range entries {
					if e.IsDir() && strings.HasSuffix(e.Name(), "-"+env.featureName) {
						prdJsonPath = filepath.Join(featureDir, e.Name(), "prd.json")
						break
					}
				}
			}
			if prdJsonPath != "" {
				if prd, err := LoadPRD(prdJsonPath); err == nil {
					_, passed, _, _, _ := countStories(prd)
					if passed == len(prd.UserStories) {
						os.RemoveAll(env.projectDir)
						return
					}
				}
			}
		}
		t.Logf("Artifacts preserved at: %s", env.projectDir)
	})

	// Clone warrantycert (using gh for authenticated access)
	t.Log("Cloning warrantycert...")
	runCmd(t, "", 2*time.Minute, "gh", "repo", "clone",
		"scripness/warrantycert", env.projectDir, "--", "--depth", "1")

	// Install dependencies
	t.Log("Installing dependencies (bun install)...")
	runCmd(t, env.projectDir, 2*time.Minute, "bun", "install")

	// Set up database
	t.Log("Setting up database (bun run db:fresh)...")
	runCmd(t, env.projectDir, 30*time.Second, "bun", "run", "db:fresh")

	// Install Playwright chromium
	t.Log("Installing Playwright chromium...")
	runCmd(t, env.projectDir, 2*time.Minute, "bunx", "playwright", "install", "chromium")

	// Verify baseline: typecheck must pass
	t.Log("Verifying baseline (typecheck)...")
	runCmd(t, env.projectDir, 60*time.Second, "bun", "run", "typecheck")
}

// phase2Init runs ralph init interactively, selecting claude and accepting detected defaults.
func phase2Init(t *testing.T, env *testEnv) {
	result := runRalphInteractive(t, env.projectDir, 30*time.Second,
		[]promptResponse{
			{pattern: "Enter choice", response: "3"},
			{pattern: "Typecheck", response: ""},
			{pattern: "Lint", response: ""},
			{pattern: "Test", response: ""},
		},
		"init",
	)
	savePhaseOutput(env, "02-init", result)

	if !result.Success() {
		t.Fatalf("ralph init failed (exit %d):\nstdout: %s\nstderr: %s",
			result.ExitCode, result.Stdout, result.Stderr)
	}

	// Verify config exists and has correct values
	configPath := filepath.Join(env.projectDir, "ralph.config.json")
	cfg := loadConfig(t, configPath)

	if cfg.Provider.Command != "claude" {
		t.Errorf("Expected provider.command=claude, got %q", cfg.Provider.Command)
	}

	verifyDefaults := strings.Join(cfg.Verify.Default, ", ")
	for _, expected := range []string{"bun run typecheck", "bun run lint"} {
		if !strings.Contains(verifyDefaults, expected) {
			t.Errorf("verify.default missing %q, got: %v", expected, cfg.Verify.Default)
		}
	}
	hasTest := false
	for _, cmd := range cfg.Verify.Default {
		if strings.Contains(cmd, "test") {
			hasTest = true
			break
		}
	}
	if !hasTest {
		t.Errorf("verify.default missing a test command, got: %v", cfg.Verify.Default)
	}

	ralphDir := filepath.Join(env.projectDir, ".ralph")
	if _, err := os.Stat(ralphDir); os.IsNotExist(err) {
		t.Error(".ralph/ directory was not created")
	}
	gitignorePath := filepath.Join(ralphDir, ".gitignore")
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		t.Error(".ralph/.gitignore was not created")
	}

	t.Logf("ralph init succeeded. Provider: %s, Verify: %v",
		cfg.Provider.Command, cfg.Verify.Default)
}

// phase3ConfigEnhancement adds services, verify.ui, and browser config.
func phase3ConfigEnhancement(t *testing.T, env *testEnv) {
	configPath := filepath.Join(env.projectDir, "ralph.config.json")
	cfg := loadConfig(t, configPath)

	cfg.Services = []ServiceConfig{
		{
			Name:                "dev",
			Start:               "bun run dev",
			Ready:               "http://localhost:3000",
			ReadyTimeout:        30,
			RestartBeforeVerify: true,
		},
	}
	cfg.Verify.UI = []string{"bun run test:e2e"}
	if cfg.Verify.Timeout == 0 {
		cfg.Verify.Timeout = 300
	}
	cfg.Browser = &BrowserConfig{
		Enabled:  true,
		Headless: true,
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Save config to artifacts
	saveArtifact(env, "config.json", string(data))

	reloaded := loadConfig(t, configPath)
	if len(reloaded.Services) != 1 {
		t.Errorf("Expected 1 service, got %d", len(reloaded.Services))
	}
	if reloaded.Services[0].Ready != "http://localhost:3000" {
		t.Errorf("Service ready URL: got %q", reloaded.Services[0].Ready)
	}
	if len(reloaded.Verify.UI) != 1 || reloaded.Verify.UI[0] != "bun run test:e2e" {
		t.Errorf("verify.ui: got %v", reloaded.Verify.UI)
	}
	if reloaded.Browser == nil || !reloaded.Browser.Enabled {
		t.Error("Browser should be enabled")
	}

	t.Log("Config enhanced with services, verify.ui, and browser")
}

// phase4Doctor runs ralph doctor and checks output.
func phase4Doctor(t *testing.T, env *testEnv) {
	result := runRalph(t, env.projectDir, "doctor")
	savePhaseOutput(env, "04-doctor", result)

	combined := result.Combined()

	checks := map[string]string{
		"config found":   "ralph.config.json found",
		"provider":       "Provider command: claude",
		"ralph dir":      ".ralph directory exists",
		"ralph writable": ".ralph directory writable",
		"sh available":   "sh available",
		"git available":  "git available",
		"git repo":       "git repository found",
		"all passed":     "All checks passed.",
	}
	for name, pattern := range checks {
		if !strings.Contains(combined, pattern) {
			t.Errorf("doctor missing %q check (expected %q in output)", name, pattern)
		}
	}

	if !result.Success() {
		t.Errorf("ralph doctor failed with exit code %d:\n%s", result.ExitCode, combined)
	}

	t.Logf("ralph doctor completed (exit %d)", result.ExitCode)
}

// phase5PrdCreate runs ralph prd to create and finalize a PRD.
func phase5PrdCreate(t *testing.T, env *testEnv) {
	result := runRalphInteractive(t, env.projectDir, 10*time.Minute,
		[]promptResponse{
			{pattern: "Ready to finalize for execution?", response: "y"},
		},
		"prd", env.featureName,
	)
	savePhaseOutput(env, "05-prd-create", result)

	if !result.Success() {
		t.Fatalf("ralph prd failed (exit %d):\nstdout: %s\nstderr: %s",
			result.ExitCode, result.Stdout, result.Stderr)
	}

	featureDir := findFeatureDir(t, env.projectDir, env.featureName)

	// Copy PRD files to artifacts
	copyArtifact(env, "prd.md", filepath.Join(featureDir, "prd.md"))
	copyArtifact(env, "prd.json", filepath.Join(featureDir, "prd.json"))

	prdMdPath := filepath.Join(featureDir, "prd.md")
	if _, err := os.Stat(prdMdPath); os.IsNotExist(err) {
		t.Fatal("prd.md was not created")
	}

	prdJsonPath := filepath.Join(featureDir, "prd.json")
	if _, err := os.Stat(prdJsonPath); os.IsNotExist(err) {
		t.Fatal("prd.json was not created")
	}

	prd := loadPRD(t, prdJsonPath)

	if prd.SchemaVersion != 2 {
		t.Errorf("Expected schemaVersion=2, got %d", prd.SchemaVersion)
	}
	if len(prd.UserStories) < 2 {
		t.Errorf("Expected at least 2 user stories, got %d", len(prd.UserStories))
	}
	if !hasUIStory(prd) {
		t.Log("Warning: No UI-tagged stories found (certificate-search is a UI feature)")
	}
	for _, s := range prd.UserStories {
		if len(s.AcceptanceCriteria) == 0 {
			t.Errorf("Story %s has no acceptance criteria", s.ID)
		}
	}
	expectedBranch := "ralph/" + env.featureName
	if prd.BranchName != expectedBranch {
		t.Errorf("Expected branchName=%q, got %q", expectedBranch, prd.BranchName)
	}
	if prd.Project == "" {
		t.Error("PRD project name is empty")
	}
	if prd.Description == "" {
		t.Error("PRD description is empty")
	}

	t.Logf("PRD created: %d stories, branch=%s", len(prd.UserStories), prd.BranchName)
	for _, s := range prd.UserStories {
		t.Logf("  %s [P%d] %s (tags: %v)", s.ID, s.Priority, s.Title, s.Tags)
	}
}

// phase6PreRunChecks runs validate, status, and next commands.
func phase6PreRunChecks(t *testing.T, env *testEnv) {
	// ralph validate
	result := runRalph(t, env.projectDir, "validate", env.featureName)
	savePhaseOutput(env, "06-validate", result)
	if !result.Success() {
		t.Errorf("ralph validate failed: %s", result.Combined())
	} else {
		combined := result.Combined()
		if !strings.Contains(combined, "prd.json is valid") {
			t.Errorf("validate output missing 'prd.json is valid': %s", combined)
		}
		if !strings.Contains(combined, "stories") {
			t.Errorf("validate output missing story count: %s", combined)
		}
		if !strings.Contains(combined, "Schema version: 2") {
			t.Errorf("validate output missing schema version: %s", combined)
		}
	}

	// ralph status <feature>
	result = runRalph(t, env.projectDir, "status", env.featureName)
	savePhaseOutput(env, "06-status", result)
	if !result.Success() {
		t.Errorf("ralph status failed: %s", result.Combined())
	} else if !strings.Contains(result.Combined(), "stories complete") {
		t.Errorf("status output missing 'stories complete': %s", result.Combined())
	}

	// ralph status (no arg — list all features)
	result = runRalph(t, env.projectDir, "status")
	savePhaseOutput(env, "06-status-all", result)
	if !result.Success() {
		t.Errorf("ralph status (all features) failed: %s", result.Combined())
	} else if !strings.Contains(result.Combined(), env.featureName) {
		t.Errorf("status (all features) missing feature name %q: %s",
			env.featureName, result.Combined())
	}

	// ralph next
	result = runRalph(t, env.projectDir, "next", env.featureName)
	savePhaseOutput(env, "06-next", result)
	if !result.Success() {
		t.Errorf("ralph next failed: %s", result.Combined())
	} else {
		combined := result.Combined()
		if !strings.Contains(combined, "US-") {
			t.Errorf("next output missing story ID (US-*): %s", combined)
		}
		if !strings.Contains(combined, "Priority:") {
			t.Errorf("next output missing 'Priority:': %s", combined)
		}
	}
}

// phase7FirstRun executes the main ralph run loop.
func phase7FirstRun(t *testing.T, env *testEnv) {
	t.Log("Starting ralph run (first run, 25 min timeout)...")

	result := runRalphWithTimeout(t, env.projectDir, 25*time.Minute, "run", env.featureName)
	savePhaseOutput(env, "07-first-run", result)

	combined := result.Combined()
	t.Logf("ralph run exited with code %d", result.ExitCode)

	checks := map[string]string{
		"banner":     "Ralph - Autonomous Agent Loop",
		"branch":     "ralph/" + env.featureName,
		"pre-verify": "Pre-verifying stories",
		"iteration":  "Iteration 1:",
	}
	for name, pattern := range checks {
		if !strings.Contains(combined, pattern) {
			t.Errorf("run output missing %q (expected %q)", name, pattern)
		}
	}

	lockPath := filepath.Join(env.projectDir, ".ralph", "ralph.lock")
	if _, err := os.Stat(lockPath); err == nil {
		t.Error("ralph.lock still exists after run completed — cleanup failed")
	}

	branch := gitBranch(t, env.projectDir)
	expectedBranch := "ralph/" + env.featureName
	if branch != expectedBranch {
		t.Errorf("Expected branch %q after run, got %q", expectedBranch, branch)
	}

	featureDir := findFeatureDir(t, env.projectDir, env.featureName)
	logsDir := filepath.Join(featureDir, "logs")
	if entries, err := os.ReadDir(logsDir); err != nil {
		t.Errorf("No logs directory found: %v", err)
	} else if len(entries) == 0 {
		t.Error("Logs directory is empty — no run log created")
	} else {
		t.Logf("Found %d log file(s)", len(entries))
		// Copy all log files to artifacts
		for _, entry := range entries {
			copyArtifact(env, filepath.Join("logs", entry.Name()),
				filepath.Join(logsDir, entry.Name()))
		}
	}
}

// phase8PostRunAnalysis parses and reports on what happened during the run.
func phase8PostRunAnalysis(t *testing.T, env *testEnv) {
	featureDir := findFeatureDir(t, env.projectDir, env.featureName)
	prdJsonPath := filepath.Join(featureDir, "prd.json")

	if _, err := os.Stat(prdJsonPath); os.IsNotExist(err) {
		t.Log("No prd.json found — skipping analysis")
		return
	}

	// Save current prd.json snapshot
	copyArtifact(env, "prd-after-run1.json", prdJsonPath)

	prd := loadPRD(t, prdJsonPath)
	total, passed, blocked, pending, withRetries := countStories(prd)

	t.Logf("Post-run story status: %d total, %d passed, %d blocked, %d pending, %d with retries",
		total, passed, blocked, pending, withRetries)

	stateChanged := false
	for _, s := range prd.UserStories {
		if s.Passes || s.Blocked || s.Retries > 0 {
			stateChanged = true
			break
		}
	}
	if !stateChanged {
		t.Error("No story changed state during the run — nothing was attempted")
	}

	if prd.Run.StartedAt != nil {
		t.Logf("Run started at: %s", *prd.Run.StartedAt)
	} else {
		t.Error("run.startedAt not set — run may not have started properly")
	}

	if len(prd.Run.Learnings) > 0 {
		t.Logf("Learnings captured: %d", len(prd.Run.Learnings))
		for i, l := range prd.Run.Learnings {
			t.Logf("  Learning %d: %s", i+1, truncate(l, 120))
		}
	}

	// Validate passed stories have commits
	for _, s := range prd.UserStories {
		if s.Passes && (s.LastResult == nil || s.LastResult.Commit == "") {
			t.Errorf("Story %s is marked passed but has no commit in LastResult", s.ID)
		}
	}

	branch := gitBranch(t, env.projectDir)
	if strings.HasPrefix(branch, "ralph/") {
		commitLog := gitLog(t, env.projectDir, 20)
		t.Logf("Recent commits:\n%s", commitLog)
		saveArtifact(env, "git-log.txt", commitLog)

		commits := gitCommitCount(t, env.projectDir)
		if commits == 0 && passed > 0 {
			t.Error("Stories passed but no commits found on branch")
		}
	}

	// Parse JSONL log events and assert on expected event types
	logsDir := filepath.Join(featureDir, "logs")
	var logEvents []Event
	if entries, err := os.ReadDir(logsDir); err == nil && len(entries) > 0 {
		logEvents = parseLogEvents(env, featureDir)

		// Summarize event types
		eventTypes := make(map[string]int)
		for _, ev := range logEvents {
			eventTypes[string(ev.Type)]++
		}
		t.Logf("Log event types: %v", eventTypes)

		// Assert critical event types exist
		assertLogEventExists(t, logEvents, EventRunStart, "run should have started")
		assertLogEventExists(t, logEvents, EventProviderStart, "provider should have been spawned")
		assertLogEventExists(t, logEvents, EventVerifyStart, "verification should have run")
		assertLogEventExists(t, logEvents, EventStoryStart, "at least one story should have started")

		// Check for service lifecycle events (configured in phase3)
		if countLogEventType(logEvents, EventServiceStart) == 0 {
			t.Error("No service_start events — dev server may not have started")
		}

		// Check for marker detection events
		markerCount := countLogEventType(logEvents, EventMarkerDetected)
		if markerCount == 0 {
			t.Log("Warning: No marker_detected events — provider may not have emitted markers")
		} else {
			t.Logf("Marker events detected: %d", markerCount)
		}

		// Check verification command results
		verifyPasses := 0
		verifyFails := 0
		for _, ev := range logEvents {
			if ev.Type == EventVerifyCmdEnd && ev.Success != nil {
				if *ev.Success {
					verifyPasses++
				} else {
					verifyFails++
				}
			}
		}
		if verifyPasses > 0 || verifyFails > 0 {
			t.Logf("Verification commands: %d passed, %d failed", verifyPasses, verifyFails)
		}
	}

	for _, s := range prd.UserStories {
		t.Logf("  %s: %s [%s] retries=%d", s.ID, s.Title, storyStatus(s), s.Retries)
	}
}

// phase9PrdRefine refines the PRD if not all stories passed.
func phase9PrdRefine(t *testing.T, env *testEnv) {
	featureDir := findFeatureDir(t, env.projectDir, env.featureName)
	prdJsonPath := filepath.Join(featureDir, "prd.json")

	prd := loadPRD(t, prdJsonPath)
	_, passed, _, _, _ := countStories(prd)

	if passed == len(prd.UserStories) {
		saveSkippedPhase(env, "09-refine", fmt.Sprintf("All %d stories passed — no refinement needed", passed))
		t.Skip("All stories passed — skipping refinement")
	}

	t.Logf("Not all stories passed (%d/%d) — refining PRD", passed, len(prd.UserStories))

	result := runRalphInteractive(t, env.projectDir, 10*time.Minute,
		[]promptResponse{
			{pattern: "Choose", response: "a"},
			{pattern: "Finalize for execution?", response: "y"},
		},
		"prd", env.featureName,
	)
	savePhaseOutput(env, "09-refine", result)

	if !result.Success() {
		t.Logf("ralph prd refine failed (exit %d): %s", result.ExitCode, result.Combined())
	} else {
		t.Log("PRD refined and re-finalized")
		// Save refined PRD
		copyArtifact(env, "prd-refined.md", filepath.Join(featureDir, "prd.md"))
		copyArtifact(env, "prd-refined.json", filepath.Join(featureDir, "prd.json"))

		// Validate the refined PRD
		valResult := runRalph(t, env.projectDir, "validate", env.featureName)
		savePhaseOutput(env, "09-validate-after-refine", valResult)
		if !valResult.Success() {
			t.Errorf("ralph validate failed after PRD refinement: %s", valResult.Combined())
		} else if !strings.Contains(valResult.Combined(), "prd.json is valid") {
			t.Errorf("Refined PRD did not pass validation: %s", valResult.Combined())
		}
	}
}

// phase10SecondRun runs ralph again if stories are still pending.
func phase10SecondRun(t *testing.T, env *testEnv) {
	featureDir := findFeatureDir(t, env.projectDir, env.featureName)
	prdJsonPath := filepath.Join(featureDir, "prd.json")

	prd := loadPRD(t, prdJsonPath)
	_, passed, _, pending, _ := countStories(prd)

	if passed == len(prd.UserStories) {
		saveSkippedPhase(env, "10-second-run", fmt.Sprintf("All %d stories already passed", passed))
		t.Skip("All stories already passed — skipping second run")
	}
	if pending == 0 {
		saveSkippedPhase(env, "10-second-run", "No pending stories — all are passed or blocked")
		t.Skip("No pending stories — all are passed or blocked")
	}

	t.Logf("Starting second run (%d pending stories, 20 min timeout)...", pending)

	result := runRalphWithTimeout(t, env.projectDir, 20*time.Minute, "run", env.featureName)
	savePhaseOutput(env, "10-second-run", result)

	t.Logf("Second run exited with code %d", result.ExitCode)

	if strings.Contains(result.Combined(), "Ralph - Autonomous Agent Loop") {
		t.Log("Second run started successfully")
	}

	lockPath := filepath.Join(env.projectDir, ".ralph", "ralph.lock")
	if _, err := os.Stat(lockPath); err == nil {
		t.Error("ralph.lock still exists after second run")
	}

	// Copy updated logs
	logsDir := filepath.Join(featureDir, "logs")
	if entries, err := os.ReadDir(logsDir); err == nil {
		for _, entry := range entries {
			copyArtifact(env, filepath.Join("logs", entry.Name()),
				filepath.Join(logsDir, entry.Name()))
		}
	}

	// Save PRD state after second run
	copyArtifact(env, "prd-after-run2.json", prdJsonPath)
}

// phase11StatusLogsResources runs status, logs, and resources commands.
func phase11StatusLogsResources(t *testing.T, env *testEnv) {
	// --- ralph status <feature> ---
	result := runRalph(t, env.projectDir, "status", env.featureName)
	savePhaseOutput(env, "11-status", result)
	if !result.Success() {
		t.Errorf("ralph status failed: %s", result.Combined())
	}
	t.Logf("Status output:\n%s", result.Stdout)

	// --- ralph status (no arg — all features) ---
	result = runRalph(t, env.projectDir, "status")
	savePhaseOutput(env, "11-status-all", result)
	if !result.Success() {
		t.Errorf("ralph status (all) failed: %s", result.Combined())
	} else {
		combined := result.Combined()
		if !strings.Contains(combined, "Features:") && !strings.Contains(combined, env.featureName) {
			t.Errorf("status (all) missing expected content")
		}
	}

	// --- ralph logs --list ---
	result = runRalph(t, env.projectDir, "logs", env.featureName, "--list")
	savePhaseOutput(env, "11-logs-list", result)
	if !result.Success() {
		t.Errorf("ralph logs --list failed: %s", result.Combined())
	} else if !strings.Contains(result.Combined(), "Run #1") {
		t.Errorf("logs --list missing 'Run #1': %s", result.Combined())
	}

	// --- ralph logs --summary ---
	result = runRalph(t, env.projectDir, "logs", env.featureName, "--summary")
	savePhaseOutput(env, "11-logs-summary", result)
	if !result.Success() {
		t.Errorf("ralph logs --summary failed: %s", result.Combined())
	} else if !strings.Contains(result.Combined(), "Run #") {
		t.Errorf("logs --summary missing 'Run #': %s", result.Combined())
	}

	// --- ralph logs --json ---
	result = runRalph(t, env.projectDir, "logs", env.featureName, "--json")
	savePhaseOutput(env, "11-logs-json", result)
	if !result.Success() {
		t.Errorf("ralph logs --json failed: %s", result.Combined())
	} else {
		scanner := bufio.NewScanner(strings.NewReader(result.Stdout))
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			if !json.Valid([]byte(line)) {
				t.Errorf("logs --json line %d is not valid JSON: %s", lineNum, truncate(line, 100))
			}
		}
		if lineNum > 0 {
			t.Logf("logs --json: %d lines, all valid JSON", lineNum)
		}
	}

	// --- ralph logs --run 1 ---
	result = runRalph(t, env.projectDir, "logs", env.featureName, "--run", "1")
	savePhaseOutput(env, "11-logs-run1", result)
	if !result.Success() {
		t.Errorf("ralph logs --run 1 failed: %s", result.Combined())
	}

	// --- ralph logs --type error ---
	result = runRalph(t, env.projectDir, "logs", env.featureName, "--type", "error")
	savePhaseOutput(env, "11-logs-type-error", result)
	if !result.Success() {
		t.Errorf("ralph logs --type error failed: %s", result.Combined())
	}

	// --- ralph logs --story (first story ID) ---
	featureDir := findFeatureDir(t, env.projectDir, env.featureName)
	prdJsonPath := filepath.Join(featureDir, "prd.json")
	if prd, err := LoadPRD(prdJsonPath); err == nil {
		storyID := firstStoryID(prd)
		if storyID != "" {
			result = runRalph(t, env.projectDir, "logs", env.featureName, "--story", storyID)
			savePhaseOutput(env, "11-logs-story", result)
			if !result.Success() {
				t.Errorf("ralph logs --story %s failed: %s", storyID, result.Combined())
			}
		}
	}

	// --- ralph resources list ---
	result = runRalph(t, env.projectDir, "resources", "list")
	savePhaseOutput(env, "11-resources-list", result)
	if !result.Success() {
		t.Logf("ralph resources list failed (exit %d): %s", result.ExitCode, result.Combined())
	} else {
		combined := result.Combined()
		if !strings.Contains(combined, "Cache directory:") {
			t.Errorf("resources list missing 'Cache directory:' in output")
		}
		t.Logf("Resources: %s", truncate(result.Stdout, 200))
	}

	// --- ralph resources path ---
	result = runRalph(t, env.projectDir, "resources", "path")
	savePhaseOutput(env, "11-resources-path", result)
	if !result.Success() {
		t.Logf("ralph resources path failed (exit %d)", result.ExitCode)
	}
}

// phase12Verify runs ralph verify if all stories passed.
func phase12Verify(t *testing.T, env *testEnv) {
	featureDir := findFeatureDir(t, env.projectDir, env.featureName)
	prdJsonPath := filepath.Join(featureDir, "prd.json")

	prd := loadPRD(t, prdJsonPath)
	_, passed, _, _, _ := countStories(prd)

	if passed != len(prd.UserStories) {
		saveSkippedPhase(env, "12-verify",
			fmt.Sprintf("Not all stories passed (%d/%d)", passed, len(prd.UserStories)))
		t.Skipf("Not all stories passed (%d/%d) — skipping verify", passed, len(prd.UserStories))
	}

	t.Log("All stories passed — running final verification (10 min timeout)...")

	result := runRalphWithTimeout(t, env.projectDir, 10*time.Minute, "verify", env.featureName)
	savePhaseOutput(env, "12-verify", result)

	combined := result.Combined()
	t.Logf("ralph verify exited with code %d", result.ExitCode)

	for _, cmd := range []string{"typecheck", "lint", "test"} {
		if strings.Contains(combined, cmd) {
			t.Logf("Verification included: %s", cmd)
		}
	}

	if result.Success() {
		t.Log("Final verification PASSED")
	} else {
		t.Errorf("Final verification FAILED (exit %d)", result.ExitCode)
	}

	// Copy updated logs after verify
	logsDir := filepath.Join(featureDir, "logs")
	if entries, err := os.ReadDir(logsDir); err == nil {
		for _, entry := range entries {
			copyArtifact(env, filepath.Join("logs", entry.Name()),
				filepath.Join(logsDir, entry.Name()))
		}
	}
}

// phase13PostRunDoctor runs ralph doctor after all runs to verify clean environment state.
func phase13PostRunDoctor(t *testing.T, env *testEnv) {
	result := runRalph(t, env.projectDir, "doctor")
	savePhaseOutput(env, "13-doctor-post", result)

	combined := result.Combined()

	// Lock should not exist
	if strings.Contains(combined, "currently running") || strings.Contains(combined, "Stale lock") {
		t.Error("doctor reports lock issue after all runs completed")
	}

	// Feature should be listed
	if !strings.Contains(combined, env.featureName) {
		t.Errorf("doctor post-run missing feature %q in output", env.featureName)
	}

	if !result.Success() {
		t.Logf("Post-run doctor found issues (exit %d):\n%s", result.ExitCode, combined)
	} else {
		t.Log("Post-run doctor: all checks passed")
	}
}

// writeReport generates the full artifact directory with report.md and per-story breakdowns.
// Called at the end of every run (pass or fail), and on early exit from fatal phases.
func writeReport(t *testing.T, env *testEnv) {
	t.Helper()

	if env.artifactDir == "" {
		return
	}

	var report strings.Builder

	report.WriteString("# E2E Test Report\n\n")
	report.WriteString(fmt.Sprintf("**Date:** %s\n", time.Now().Format("2006-01-02 15:04:05")))
	report.WriteString(fmt.Sprintf("**Feature:** %s\n", env.featureName))
	if env.projectDir != "" {
		report.WriteString(fmt.Sprintf("**Project dir:** %s\n", env.projectDir))
	}
	report.WriteString("\n")

	// Load config for report context
	var cfg *RalphConfig
	if env.projectDir != "" {
		configPath := filepath.Join(env.projectDir, "ralph.config.json")
		if data, err := os.ReadFile(configPath); err == nil {
			var c RalphConfig
			if json.Unmarshal(data, &c) == nil {
				cfg = &c
			}
		}
	}

	// Try to load PRD — may not exist if early phases failed
	var prd *PRD
	var featureDir string
	if env.projectDir != "" {
		ralphDir := filepath.Join(env.projectDir, ".ralph")
		if entries, err := os.ReadDir(ralphDir); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					parts := strings.SplitN(e.Name(), "-", 4)
					if len(parts) >= 4 && strings.EqualFold(parts[3], env.featureName) {
						featureDir = filepath.Join(ralphDir, e.Name())
						break
					}
					// Also try YYYYMMDD format
					if len(e.Name()) > 9 && e.Name()[8] == '-' {
						suffix := e.Name()[9:]
						if strings.EqualFold(suffix, env.featureName) {
							featureDir = filepath.Join(ralphDir, e.Name())
							break
						}
					}
				}
			}
		}
		if featureDir != "" {
			prdJsonPath := filepath.Join(featureDir, "prd.json")
			if p, err := LoadPRD(prdJsonPath); err == nil {
				prd = p
			}
		}
	}

	if prd == nil {
		report.WriteString("## Result: INCOMPLETE\n\n")
		report.WriteString("PRD was not created — test failed in early phases.\n")
		report.WriteString("Check `phases/` directory for command output.\n")
		saveArtifact(env, "report.md", report.String())
		saveArtifact(env, "result.txt", "INCOMPLETE\n")
		writeSummaryJSON(env, "INCOMPLETE", nil, cfg, nil, "", 0)
		t.Log(report.String())
		return
	}

	total, passed, blocked, pending, _ := countStories(prd)

	var result string
	switch {
	case passed == total:
		result = "PASS"
	case passed > 0:
		result = "PARTIAL"
	default:
		result = "FAIL"
	}

	report.WriteString(fmt.Sprintf("## Result: %s\n\n", result))
	report.WriteString(fmt.Sprintf("**Stories:** %d total, %d passed, %d blocked, %d pending\n",
		total, passed, blocked, pending))
	if prd.Description != "" {
		report.WriteString(fmt.Sprintf("**Description:** %s\n", prd.Description))
	}
	report.WriteString("\n")

	// ============================================================
	// Configuration — what was used
	// ============================================================
	if cfg != nil {
		report.WriteString("## Configuration\n\n")
		report.WriteString(fmt.Sprintf("- **Provider:** `%s`\n", cfg.Provider.Command))
		report.WriteString(fmt.Sprintf("- **Max retries:** %d\n", cfg.MaxRetries))
		if len(cfg.Verify.Default) > 0 {
			report.WriteString(fmt.Sprintf("- **Verify (default):** `%s`\n", strings.Join(cfg.Verify.Default, "`, `")))
		}
		if len(cfg.Verify.UI) > 0 {
			report.WriteString(fmt.Sprintf("- **Verify (ui):** `%s`\n", strings.Join(cfg.Verify.UI, "`, `")))
		}
		report.WriteString(fmt.Sprintf("- **Verify timeout:** %ds\n", cfg.Verify.Timeout))
		if len(cfg.Services) > 0 {
			for _, svc := range cfg.Services {
				report.WriteString(fmt.Sprintf("- **Service %s:** `%s` -> %s (restart=%v)\n",
					svc.Name, svc.Start, svc.Ready, svc.RestartBeforeVerify))
			}
		}
		if cfg.Browser != nil {
			report.WriteString(fmt.Sprintf("- **Browser:** enabled=%v headless=%v\n", cfg.Browser.Enabled, cfg.Browser.Headless))
		}
		report.WriteString("\n")
	}

	// Copy final PRD + config to artifacts
	if featureDir != "" {
		copyArtifact(env, "prd-final.json", filepath.Join(featureDir, "prd.json"))
		copyArtifact(env, "prd-final.md", filepath.Join(featureDir, "prd.md"))
		copyArtifact(env, "config.json", filepath.Join(env.projectDir, "ralph.config.json"))
	}

	// ============================================================
	// Git info
	// ============================================================
	var branch string
	var diffStat string
	var commitCount int
	if env.projectDir != "" {
		if cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD"); cmd != nil {
			cmd.Dir = env.projectDir
			if out, err := cmd.Output(); err == nil {
				branch = strings.TrimSpace(string(out))
			}
		}
		report.WriteString(fmt.Sprintf("**Branch:** %s\n", branch))

		if strings.HasPrefix(branch, "ralph/") {
			if logCmd := exec.Command("git", "log", "--oneline", "main...HEAD"); logCmd != nil {
				logCmd.Dir = env.projectDir
				if out, err := logCmd.Output(); err == nil {
					saveArtifact(env, "git-log.txt", string(out))
					lines := strings.Split(strings.TrimSpace(string(out)), "\n")
					if lines[0] != "" {
						commitCount = len(lines)
					}
					report.WriteString(fmt.Sprintf("**Commits on branch:** %d\n", commitCount))
				}
			}

			if dsCmd := exec.Command("git", "diff", "--stat", "main...HEAD"); dsCmd != nil {
				dsCmd.Dir = env.projectDir
				if out, err := dsCmd.Output(); err == nil {
					diffStat = string(out)
					saveArtifact(env, "git-diff-stat.txt", diffStat)
				}
			}

			if diffCmd := exec.Command("git", "diff", "main...HEAD"); diffCmd != nil {
				diffCmd.Dir = env.projectDir
				if out, err := diffCmd.Output(); err == nil {
					saveArtifact(env, "git-diff.txt", string(out))
				}
			}
		}

		// Capture working tree status
		if statusCmd := exec.Command("git", "status", "--short"); statusCmd != nil {
			statusCmd.Dir = env.projectDir
			if out, err := statusCmd.Output(); err == nil {
				wtStatus := string(out)
				saveArtifact(env, "git-status.txt", wtStatus)
				if wtStatus != "" {
					report.WriteString(fmt.Sprintf("**Working tree:** %d uncommitted file(s)\n",
						len(strings.Split(strings.TrimSpace(wtStatus), "\n"))))
				} else {
					report.WriteString("**Working tree:** clean\n")
				}
			}
		}
	}

	// Show diff stat in report for scope-at-a-glance
	if diffStat != "" {
		report.WriteString("\n**Changes:**\n```\n")
		report.WriteString(diffStat)
		report.WriteString("```\n")
	}
	report.WriteString("\n---\n\n")

	// ============================================================
	// Parse JSONL logs for per-story detail extraction
	// ============================================================
	logEvents := parseLogEvents(env, featureDir)

	// ============================================================
	// Copy browser screenshots
	// ============================================================
	screenshotDir := filepath.Join(env.projectDir, ".ralph", "screenshots")
	var screenshotsByStory = make(map[string][]string)
	if entries, err := os.ReadDir(screenshotDir); err == nil && len(entries) > 0 {
		for _, entry := range entries {
			src := filepath.Join(screenshotDir, entry.Name())
			copyArtifact(env, filepath.Join("screenshots", entry.Name()), src)
			// Screenshots named like US-001-20260210-150405-_path.png
			name := entry.Name()
			if idx := strings.Index(name, "-"); idx > 0 {
				// Extract story ID prefix: find second dash group matching US-NNN
				if strings.HasPrefix(strings.ToUpper(name), "US-") {
					parts := strings.SplitN(name, "-", 4)
					if len(parts) >= 3 {
						storyID := strings.ToUpper(parts[0] + "-" + parts[1])
						screenshotsByStory[storyID] = append(screenshotsByStory[storyID], entry.Name())
					}
				}
			}
		}
		report.WriteString(fmt.Sprintf("**Screenshots:** %d saved in `screenshots/` directory\n\n", len(entries)))
	}

	// ============================================================
	// Run duration from JSONL logs
	// ============================================================
	var totalDurationSecs float64
	for _, ev := range logEvents {
		if ev.Type == "run_end" && ev.Duration != nil {
			totalDurationSecs = float64(*ev.Duration) / 1e9
			report.WriteString(fmt.Sprintf("**Total run duration:** %.0fs\n\n", totalDurationSecs))
			break
		}
	}

	// ============================================================
	// Run number from JSONL logs
	// ============================================================
	var runNumber int
	for _, ev := range logEvents {
		if ev.Type == "run_start" && ev.Iteration > 0 {
			runNumber = ev.Iteration
			break
		}
	}

	// ============================================================
	// Per-story breakdown — organized by "idea"
	// ============================================================
	report.WriteString("## Stories (Ideas)\n\n")

	for _, s := range prd.UserStories {
		status := storyStatus(s)
		icon := "?"
		switch status {
		case "PASSED":
			icon = "PASS"
		case "BLOCKED":
			icon = "BLOCK"
		case "PENDING":
			icon = "PEND"
		}

		report.WriteString(fmt.Sprintf("### [%s] %s: %s\n\n", icon, s.ID, s.Title))
		report.WriteString(fmt.Sprintf("**Description:** %s\n\n", s.Description))

		if len(s.Tags) > 0 {
			report.WriteString(fmt.Sprintf("**Tags:** %s\n", strings.Join(s.Tags, ", ")))
		}
		report.WriteString(fmt.Sprintf("**Priority:** %d | **Retries:** %d\n\n", s.Priority, s.Retries))

		// Acceptance criteria as checklist
		report.WriteString("**Acceptance Criteria:**\n")
		for _, ac := range s.AcceptanceCriteria {
			check := " "
			if s.Passes {
				check = "x"
			}
			report.WriteString(fmt.Sprintf("- [%s] %s\n", check, ac))
		}
		report.WriteString("\n")

		if s.LastResult != nil {
			if s.LastResult.Commit != "" {
				report.WriteString(fmt.Sprintf("**Last commit:** `%s`\n", s.LastResult.Commit))
			}
			if s.LastResult.Summary != "" {
				report.WriteString(fmt.Sprintf("**Summary:** %s\n", s.LastResult.Summary))
			}
			if s.LastResult.CompletedAt != "" {
				report.WriteString(fmt.Sprintf("**Completed at:** %s\n", s.LastResult.CompletedAt))
			}
			report.WriteString("\n")
		}

		// Failed/blocked: include failure details inline
		if !s.Passes && s.Notes != "" {
			report.WriteString("**Failure Details:**\n```\n")
			report.WriteString(s.Notes)
			report.WriteString("\n```\n\n")
		}

		// Screenshots for this story
		if shots, ok := screenshotsByStory[s.ID]; ok && len(shots) > 0 {
			report.WriteString(fmt.Sprintf("**Screenshots:** %d — see `screenshots/` directory\n\n", len(shots)))
		}

		report.WriteString("---\n\n")

		// Write per-story artifacts
		writeStoryArtifact(env, s, logEvents, screenshotsByStory[s.ID])
	}

	// ============================================================
	// Learnings
	// ============================================================
	if len(prd.Run.Learnings) > 0 {
		report.WriteString("## Learnings\n\n")
		for _, l := range prd.Run.Learnings {
			report.WriteString(fmt.Sprintf("- %s\n", l))
		}
		report.WriteString("\n")

		var learningsBuf strings.Builder
		for i, l := range prd.Run.Learnings {
			fmt.Fprintf(&learningsBuf, "%d. %s\n\n", i+1, l)
		}
		saveArtifact(env, "learnings.txt", learningsBuf.String())
	}

	// ============================================================
	// Copy JSONL logs
	// ============================================================
	if featureDir != "" {
		logsDir := filepath.Join(featureDir, "logs")
		if entries, err := os.ReadDir(logsDir); err == nil {
			for _, entry := range entries {
				copyArtifact(env, filepath.Join("logs", entry.Name()),
					filepath.Join(logsDir, entry.Name()))
			}
			report.WriteString(fmt.Sprintf("## Logs\n\n%d log file(s) saved in `logs/` directory.\n\n", len(entries)))
		}
	}

	// ============================================================
	// Write result files
	// ============================================================
	saveArtifact(env, "report.md", report.String())
	saveArtifact(env, "result.txt", result+"\n")
	writeSummaryJSON(env, result, prd, cfg, logEvents, branch, runNumber)

	// Print summary to test output
	t.Log("============================================================")
	t.Log(" E2E Test Report")
	t.Log("============================================================")
	t.Logf(" Result: %s", result)
	t.Logf(" Stories: %d total, %d passed, %d blocked, %d pending", total, passed, blocked, pending)
	t.Log("")

	for _, s := range prd.UserStories {
		t.Logf("   [%s] %s: %s (retries=%d)", storyStatus(s), s.ID, s.Title, s.Retries)
		if !s.Passes && s.Notes != "" {
			lines := strings.SplitN(s.Notes, "\n", 3)
			for _, line := range lines[:min(len(lines), 2)] {
				t.Logf("         %s", truncate(line, 120))
			}
		}
	}

	if len(prd.Run.Learnings) > 0 {
		t.Logf("\n Learnings: %d captured", len(prd.Run.Learnings))
	}

	absArtifactDir, _ := filepath.Abs(env.artifactDir)
	t.Logf("\n Artifacts: %s", absArtifactDir)
	t.Log("============================================================")
}

// parseLogEvents reads all JSONL log files from the feature directory and returns parsed events.
func parseLogEvents(env *testEnv, featureDir string) []Event {
	if featureDir == "" {
		return nil
	}
	logsDir := filepath.Join(featureDir, "logs")
	entries, err := os.ReadDir(logsDir)
	if err != nil || len(entries) == 0 {
		return nil
	}

	var allEvents []Event
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(logsDir, entry.Name()))
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			var ev Event
			if json.Unmarshal([]byte(line), &ev) == nil {
				allEvents = append(allEvents, ev)
			}
		}
	}
	return allEvents
}

// extractStoryProviderOutput extracts provider output for a specific story from JSONL events.
func extractStoryProviderOutput(events []Event, storyID string) string {
	var buf strings.Builder
	inStory := false
	for _, ev := range events {
		if ev.Type == "story_start" && ev.StoryID == storyID {
			inStory = true
			continue
		}
		if ev.Type == "story_end" && ev.StoryID == storyID {
			inStory = false
			continue
		}
		if inStory && ev.Type == "provider_output" && ev.Data != nil {
			if stdout, ok := ev.Data["stdout"].(string); ok && stdout != "" {
				buf.WriteString(stdout)
			}
			if stderr, ok := ev.Data["stderr"].(string); ok && stderr != "" {
				buf.WriteString(stderr)
			}
		}
	}
	return buf.String()
}

// extractStoryVerifyOutput extracts verification command output for a specific story from JSONL events.
func extractStoryVerifyOutput(events []Event, storyID string) string {
	var buf strings.Builder
	inStory := false
	for _, ev := range events {
		if ev.Type == "story_start" && ev.StoryID == storyID {
			inStory = true
			continue
		}
		if ev.Type == "story_end" && ev.StoryID == storyID {
			inStory = false
			continue
		}
		if inStory && ev.Type == "verify_cmd_end" && ev.Data != nil {
			cmd, _ := ev.Data["cmd"].(string)
			output, _ := ev.Data["output"].(string)
			success := ev.Success != nil && *ev.Success
			status := "PASS"
			if !success {
				status = "FAIL"
			}
			fmt.Fprintf(&buf, "=== [%s] %s ===\n%s\n\n", status, cmd, output)
		}
	}
	return buf.String()
}

// extractStoryBrowserOutput extracts browser step results for a specific story from JSONL events.
func extractStoryBrowserOutput(events []Event, storyID string) string {
	var buf strings.Builder
	inStory := false
	for _, ev := range events {
		if ev.Type == "story_start" && ev.StoryID == storyID {
			inStory = true
			continue
		}
		if ev.Type == "story_end" && ev.StoryID == storyID {
			inStory = false
			continue
		}
		if inStory && ev.Type == "browser_step" && ev.Data != nil {
			action, _ := ev.Data["action"].(string)
			success := ev.Success != nil && *ev.Success
			details, _ := ev.Data["details"].(string)
			status := "PASS"
			if !success {
				status = "FAIL"
			}
			fmt.Fprintf(&buf, "[%s] %s", status, action)
			if details != "" {
				fmt.Fprintf(&buf, " — %s", details)
			}
			buf.WriteString("\n")
		}
	}
	return buf.String()
}

// writeStoryArtifact writes per-story files to stories/<id>/.
func writeStoryArtifact(env *testEnv, s UserStory, logEvents []Event, screenshots []string) {
	dir := strings.ToLower(s.ID)
	var buf strings.Builder

	status := storyStatus(s)
	fmt.Fprintf(&buf, "# %s: %s\n\n", s.ID, s.Title)
	fmt.Fprintf(&buf, "**Status:** %s\n", status)
	fmt.Fprintf(&buf, "**Priority:** %d\n", s.Priority)
	fmt.Fprintf(&buf, "**Retries:** %d\n", s.Retries)
	if len(s.Tags) > 0 {
		fmt.Fprintf(&buf, "**Tags:** %s\n", strings.Join(s.Tags, ", "))
	}
	buf.WriteString("\n")

	fmt.Fprintf(&buf, "## Description\n\n%s\n\n", s.Description)

	buf.WriteString("## Acceptance Criteria\n\n")
	for _, ac := range s.AcceptanceCriteria {
		check := " "
		if s.Passes {
			check = "x"
		}
		fmt.Fprintf(&buf, "- [%s] %s\n", check, ac)
	}
	buf.WriteString("\n")

	if len(s.BrowserSteps) > 0 {
		buf.WriteString("## Browser Steps\n\n")
		for _, step := range s.BrowserSteps {
			fmt.Fprintf(&buf, "- **%s**", step.Action)
			if step.URL != "" {
				fmt.Fprintf(&buf, " url=%s", step.URL)
			}
			if step.Selector != "" {
				fmt.Fprintf(&buf, " selector=%s", step.Selector)
			}
			if step.Value != "" {
				fmt.Fprintf(&buf, " value=%s", step.Value)
			}
			buf.WriteString("\n")
		}
		buf.WriteString("\n")
	}

	if s.LastResult != nil {
		buf.WriteString("## Last Result\n\n")
		if s.LastResult.Commit != "" {
			fmt.Fprintf(&buf, "- **Commit:** `%s`\n", s.LastResult.Commit)
		}
		if s.LastResult.Summary != "" {
			fmt.Fprintf(&buf, "- **Summary:** %s\n", s.LastResult.Summary)
		}
		if s.LastResult.CompletedAt != "" {
			fmt.Fprintf(&buf, "- **Completed at:** %s\n", s.LastResult.CompletedAt)
		}
		buf.WriteString("\n")
	}

	if len(screenshots) > 0 {
		buf.WriteString("## Screenshots\n\n")
		for _, shot := range screenshots {
			fmt.Fprintf(&buf, "- `screenshots/%s`\n", shot)
		}
		buf.WriteString("\n")
	}

	// Failure info
	if !s.Passes && s.Notes != "" {
		buf.WriteString("## Failure Details\n\n")
		buf.WriteString("```\n")
		buf.WriteString(s.Notes)
		buf.WriteString("\n```\n")
	}

	saveArtifact(env, filepath.Join("stories", dir, "summary.md"), buf.String())

	// Save raw failure notes for easy grep
	if !s.Passes && s.Notes != "" {
		saveArtifact(env, filepath.Join("stories", dir, "failure.txt"), s.Notes)
	}

	// Save per-story git diff (from commit)
	if s.LastResult != nil && s.LastResult.Commit != "" && env.projectDir != "" {
		showCmd := exec.Command("git", "show", "--stat", "--patch", s.LastResult.Commit)
		showCmd.Dir = env.projectDir
		if out, err := showCmd.Output(); err == nil {
			saveArtifact(env, filepath.Join("stories", dir, "commit-diff.txt"), string(out))
		}
	}

	// Extract and save provider output for this story from JSONL logs
	if len(logEvents) > 0 {
		providerOutput := extractStoryProviderOutput(logEvents, s.ID)
		if providerOutput != "" {
			saveArtifact(env, filepath.Join("stories", dir, "provider-output.txt"), providerOutput)
		}

		verifyOutput := extractStoryVerifyOutput(logEvents, s.ID)
		if verifyOutput != "" {
			saveArtifact(env, filepath.Join("stories", dir, "verify-output.txt"), verifyOutput)
		}

		browserOutput := extractStoryBrowserOutput(logEvents, s.ID)
		if browserOutput != "" {
			saveArtifact(env, filepath.Join("stories", dir, "browser-steps.txt"), browserOutput)
		}
	}
}

// storySummaryForJSON returns a JSON-serializable story summary.
type storySummaryJSON struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Status        string   `json:"status"`
	Retries       int      `json:"retries"`
	Commit        string   `json:"commit,omitempty"`
	FailureReason string   `json:"failure_reason,omitempty"`
	Tags          []string `json:"tags,omitempty"`
}

type summaryJSON struct {
	Result         string             `json:"result"`
	Feature        string             `json:"feature"`
	Description    string             `json:"description,omitempty"`
	Date           string             `json:"date"`
	Branch         string             `json:"branch,omitempty"`
	RunNumber      int                `json:"run_number,omitempty"`
	DurationSecs   float64            `json:"duration_seconds,omitempty"`
	Provider       string             `json:"provider,omitempty"`
	MaxRetries     int                `json:"max_retries,omitempty"`
	VerifyDefault  []string           `json:"verify_default,omitempty"`
	VerifyUI       []string           `json:"verify_ui,omitempty"`
	TotalStories   int                `json:"total_stories"`
	PassedStories  int                `json:"passed_stories"`
	BlockedStories int                `json:"blocked_stories"`
	PendingStories int                `json:"pending_stories"`
	Stories        []storySummaryJSON `json:"stories"`
	Learnings      []string           `json:"learnings,omitempty"`
	Commits        int                `json:"commits,omitempty"`
}

// writeSummaryJSON writes a machine-readable summary.json for AI agent consumption.
func writeSummaryJSON(env *testEnv, result string, prd *PRD, cfg *RalphConfig, logEvents []Event, branch string, runNumber int) {
	s := summaryJSON{
		Result:    result,
		Feature:   env.featureName,
		Date:      time.Now().Format(time.RFC3339),
		Branch:    branch,
		RunNumber: runNumber,
	}

	if prd != nil {
		s.Description = prd.Description
		total, passed, blocked, pending, _ := countStories(prd)
		s.TotalStories = total
		s.PassedStories = passed
		s.BlockedStories = blocked
		s.PendingStories = pending
		s.Learnings = prd.Run.Learnings

		for _, story := range prd.UserStories {
			ss := storySummaryJSON{
				ID:      story.ID,
				Title:   story.Title,
				Status:  storyStatus(story),
				Retries: story.Retries,
				Tags:    story.Tags,
			}
			if story.LastResult != nil {
				ss.Commit = story.LastResult.Commit
			}
			if !story.Passes && story.Notes != "" {
				ss.FailureReason = truncate(story.Notes, 500)
			}
			s.Stories = append(s.Stories, ss)
		}
	}

	if cfg != nil {
		s.Provider = cfg.Provider.Command
		s.MaxRetries = cfg.MaxRetries
		s.VerifyDefault = cfg.Verify.Default
		s.VerifyUI = cfg.Verify.UI
	}

	// Extract total duration from log events
	for _, ev := range logEvents {
		if ev.Type == "run_end" && ev.Duration != nil {
			s.DurationSecs = float64(*ev.Duration) / 1e9
			break
		}
	}

	// Count commits from git-log.txt if already saved
	if data, err := os.ReadFile(filepath.Join(env.artifactDir, "git-log.txt")); err == nil {
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if lines[0] != "" {
			s.Commits = len(lines)
		}
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	saveArtifact(env, "summary.json", string(data))
}

// truncate shortens a string to maxLen characters, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
