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

// loadPRDAndState loads a PRDDefinition and RunState from a feature directory.
func loadPRDAndState(t *testing.T, featureDir string) (*PRDDefinition, *RunState) {
	t.Helper()
	prdPath := filepath.Join(featureDir, "prd.json")
	statePath := filepath.Join(featureDir, "run-state.json")
	def, err := LoadPRDDefinition(prdPath)
	if err != nil {
		t.Fatalf("Failed to load PRDDefinition from %s: %v", featureDir, err)
	}
	state, err := LoadRunState(statePath)
	if err != nil {
		t.Fatalf("Failed to load RunState from %s: %v", featureDir, err)
	}
	return def, state
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

// countStories returns (total, passed, skipped, pending, withRetries) from a PRDDefinition and RunState.
func countStories(def *PRDDefinition, state *RunState) (total, passed, skipped, pending, withRetries int) {
	total = len(def.UserStories)
	for _, s := range def.UserStories {
		switch {
		case state.IsPassed(s.ID):
			passed++
		case state.IsSkipped(s.ID):
			skipped++
		default:
			pending++
		}
		if state.GetRetries(s.ID) > 0 {
			withRetries++
		}
	}
	return
}

// hasUIStory returns true if any story has a "ui" tag.
func hasUIStory(def *PRDDefinition) bool {
	for _, s := range def.UserStories {
		for _, tag := range s.Tags {
			if strings.EqualFold(tag, "ui") {
				return true
			}
		}
	}
	return false
}

// storyStatusStr returns a human-readable status string.
func storyStatusStr(id string, state *RunState) string {
	if state.IsPassed(id) {
		return "PASSED"
	}
	if state.IsSkipped(id) {
		return "SKIPPED"
	}
	return "PENDING"
}

// firstStoryID returns the ID of the first story in the PRD, or "" if none.
func firstStoryID(def *PRDDefinition) string {
	if len(def.UserStories) > 0 {
		return def.UserStories[0].ID
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
	if t.Failed() {
		writeReport(t, env)
		t.Fatal("Phase 8 failed (run made no progress) — cannot proceed")
	}

	// ==================== Phase 9: PRD Refinement (conditional) ====================
	t.Run("Phase9_PrdRefine", func(t *testing.T) {
		phase9PrdRefine(t, env)
	})

	// ==================== Phase 10: Second Run (conditional) ====================
	t.Run("Phase10_SecondRun", func(t *testing.T) {
		phase10SecondRun(t, env)
	})
	if t.Failed() {
		writeReport(t, env)
		t.Fatal("Phase 10 failed — cannot proceed")
	}

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
				featureDirPath := filepath.Dir(prdJsonPath)
				statePath := filepath.Join(featureDirPath, "run-state.json")
				if def, err := LoadPRDDefinition(prdJsonPath); err == nil {
					state, _ := LoadRunState(statePath)
					_, passed, _, _, _ := countStories(def, state)
					if passed == len(def.UserStories) {
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

	// Generate initial CSS (Tailwind needs this before dev server can start)
	t.Log("Generating initial CSS...")
	runCmd(t, env.projectDir, 60*time.Second, "bunx", "@tailwindcss/cli",
		"-i", "public/input.css", "-o", "public/styles.css")

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

// phase3ConfigEnhancement adds services and verify.ui.
func phase3ConfigEnhancement(t *testing.T, env *testEnv) {
	configPath := filepath.Join(env.projectDir, "ralph.config.json")
	cfg := loadConfig(t, configPath)

	cfg.Services = []ServiceConfig{
		{
			Name:                "dev",
			Start:               "bun run dev",
			Ready:               "http://localhost:3000",
			ReadyTimeout:        60,
			RestartBeforeVerify: true,
		},
	}
	// Use the project's real Playwright e2e test command. This is the primary UI
	// verification mechanism now that browser.go/rod is removed — the project's own
	// e2e test suite validates UI stories through verify.ui commands.
	cfg.Verify.UI = []string{"bun run test:e2e"}
	if cfg.Verify.Timeout == 0 {
		cfg.Verify.Timeout = 300
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
		t.Errorf("verify.ui should be [\"bun run test:e2e\"], got %v", reloaded.Verify.UI)
	}

	t.Log("Config enhanced with services and verify.ui")
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

// phase5PrdCreate creates a PRD programmatically.
// We write prd.md and prd.json directly rather than relying on Claude's brainstorming,
// because `ralph prd` spawns Claude interactively (inheriting stdin), which causes
// hangs in a pipe-based test environment. The real value of the E2E test is exercising
// `ralph run` (the agent loop), not Claude's PRD brainstorming.
// We still validate the PRD through `ralph validate` in Phase 6.
func phase5PrdCreate(t *testing.T, env *testEnv) {
	// Create the feature directory (YYYY-MM-DD-<feature>)
	today := time.Now().Format("2006-01-02")
	featureDirName := today + "-" + env.featureName
	featureDir := filepath.Join(env.projectDir, ".ralph", featureDirName)
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatalf("Failed to create feature dir: %v", err)
	}

	// Write prd.md
	prdMd := `# Certificate Search

## Overview
Add search and filtering functionality to the certificates page (/certificates) so business users can quickly find certificates by customer name, product, or status.

## User Stories

### US-001: Add search input to certificates page
Add a text search input at the top of the certificates list that filters by customer name and product name. Server-side filtering with form submission. Write Playwright e2e tests that verify the search functionality.

### US-002: Add status filter dropdown and combined e2e tests
Add a dropdown filter for certificate status (active/expired/voided) next to the search input. Works in combination with text search. Write Playwright e2e tests that verify filtering and combined search+filter functionality.
`
	prdMdPath := filepath.Join(featureDir, "prd.md")
	if err := os.WriteFile(prdMdPath, []byte(prdMd), 0o644); err != nil {
		t.Fatalf("Failed to write prd.md: %v", err)
	}

	// Write prd.json (v3 definition — no runtime fields)
	// Each UI story is responsible for writing its own e2e tests — this is the
	// e2e-first verification strategy. verify.ui runs "bun run test:e2e" which
	// executes the project's Playwright suite to validate UI stories.
	def := &PRDDefinition{
		SchemaVersion: 3,
		Project:       "warrantycert",
		BranchName:    "ralph/" + env.featureName,
		Description:   "Add search and filtering to the certificates page so users can find certificates by name, product, or status.",
		UserStories: []StoryDefinition{
			{
				ID:          "US-001",
				Title:       "Add search input to certificates page",
				Description: "Add a text search input field at the top of the certificates list page (/certificates). When a user types a query and submits the form, the page reloads with certificates filtered by customer name or product name (case-insensitive partial match). The search term should be preserved in the URL as a query parameter (?q=term) so the filtered view is bookmarkable. When the search field is cleared, all certificates are shown again. Write Playwright e2e tests that verify the search functionality works end-to-end.",
				AcceptanceCriteria: []string{
					"A search input is visible at the top of the /certificates page",
					"Submitting a search term filters certificates by customer name or product name",
					"Search is case-insensitive and supports partial matches",
					"The search term is preserved in the URL as ?q=<term>",
					"Clearing the search shows all certificates",
					"Playwright e2e tests exist and pass for search functionality",
					"Typecheck passes",
					"Lint passes",
				},
				Tags:     []string{"ui"},
				Priority: 1,
			},
			{
				ID:          "US-002",
				Title:       "Add status filter dropdown",
				Description: "Add a dropdown/select element next to the search input that allows filtering certificates by status (all/active/expired/voided). The filter works in combination with the text search. The selected status is preserved in the URL as a query parameter (?status=active). Default is 'all' (no filtering). Write Playwright e2e tests that verify filtering and combined search+filter functionality.",
				AcceptanceCriteria: []string{
					"A status filter dropdown is visible next to the search input on /certificates",
					"Filter options include: All, Active, Expired, Voided",
					"Selecting a status filters the certificate list to only show matching certificates",
					"Status filter works in combination with text search",
					"The selected status is preserved in the URL as ?status=<value>",
					"Playwright e2e tests exist and pass for filter and combined search+filter",
					"Typecheck passes",
					"Lint passes",
				},
				Tags:     []string{"ui"},
				Priority: 2,
			},
		},
	}

	prdJsonPath := filepath.Join(featureDir, "prd.json")
	data, err := json.MarshalIndent(def, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal prd.json: %v", err)
	}
	if err := os.WriteFile(prdJsonPath, data, 0o644); err != nil {
		t.Fatalf("Failed to write prd.json: %v", err)
	}

	// Copy PRD files to artifacts
	copyArtifact(env, "prd.md", prdMdPath)
	copyArtifact(env, "prd.json", prdJsonPath)

	// Validate by loading through ralph's own loader
	loaded, loadErr := LoadPRDDefinition(prdJsonPath)
	if loadErr != nil {
		t.Fatalf("Failed to load PRDDefinition: %v", loadErr)
	}

	if loaded.SchemaVersion != 3 {
		t.Errorf("Expected schemaVersion=3, got %d", loaded.SchemaVersion)
	}
	if len(loaded.UserStories) != 2 {
		t.Errorf("Expected 2 user stories, got %d", len(loaded.UserStories))
	}
	hasUI := false
	for _, s := range loaded.UserStories {
		for _, tag := range s.Tags {
			if strings.EqualFold(tag, "ui") {
				hasUI = true
			}
		}
	}
	if !hasUI {
		t.Error("No UI-tagged stories found")
	}
	expectedBranch := "ralph/" + env.featureName
	if loaded.BranchName != expectedBranch {
		t.Errorf("Expected branchName=%q, got %q", expectedBranch, loaded.BranchName)
	}

	t.Logf("PRD created: %d stories, branch=%s", len(loaded.UserStories), loaded.BranchName)
	for _, s := range loaded.UserStories {
		t.Logf("  %s [P%d] %s (tags: %v)", s.ID, s.Priority, s.Title, s.Tags)
	}
}

// phase6PreRunChecks runs status commands.
func phase6PreRunChecks(t *testing.T, env *testEnv) {
	// ralph status <feature>
	result := runRalph(t, env.projectDir, "status", env.featureName)
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

	def, state := loadPRDAndState(t, featureDir)
	total, passed, skipped, pending, withRetries := countStories(def, state)

	t.Logf("Post-run story status: %d total, %d passed, %d skipped, %d pending, %d with retries",
		total, passed, skipped, pending, withRetries)

	// Check if run made any progress
	runStarted := len(state.Passed) > 0 || len(state.Skipped) > 0 || len(state.Learnings) > 0
	for _, r := range state.Retries {
		if r > 0 {
			runStarted = true
			break
		}
	}

	stateChanged := passed > 0 || skipped > 0 || withRetries > 0
	if !stateChanged && runStarted {
		t.Error("No story changed state during the run — nothing was attempted")
	} else if !stateChanged {
		t.Log("No story changed state (run did not start — check Phase 7 output)")
	}

	if len(state.Learnings) > 0 {
		t.Logf("Learnings captured: %d", len(state.Learnings))
		for i, l := range state.Learnings {
			t.Logf("  Learning %d: %s", i+1, truncate(l, 120))
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

		// Only assert on critical events if the run actually started
		if runStarted {
			assertLogEventExists(t, logEvents, EventRunStart, "run should have started")
			assertLogEventExists(t, logEvents, EventProviderStart, "provider should have been spawned")
			assertLogEventExists(t, logEvents, EventVerifyStart, "verification should have run")
			assertLogEventExists(t, logEvents, EventIterationStart, "at least one iteration should have started")

			// Check for service lifecycle events (configured in phase3)
			if countLogEventType(logEvents, EventServiceStart) == 0 {
				t.Error("No service_start events — dev server may not have started")
			}
		} else {
			t.Log("Skipping event assertions — run did not start")
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
	} else if runStarted {
		t.Error("Run started but no log files found")
	} else {
		t.Log("No log files found (run did not start)")
	}

	for _, s := range def.UserStories {
		t.Logf("  %s: %s [%s] retries=%d", s.ID, s.Title, storyStatusStr(s.ID, state), state.GetRetries(s.ID))
	}
}

// phase9PrdRefine resets failed/skipped stories so the second run can retry them.
// We skip interactive `ralph prd` refinement because it spawns Claude with inherited
// stdin, which hangs in a pipe-based test environment. Instead, we directly reset
// story state in prd.json — this is what a user would do before re-running.
func phase9PrdRefine(t *testing.T, env *testEnv) {
	featureDir := findFeatureDir(t, env.projectDir, env.featureName)
	prdJsonPath := filepath.Join(featureDir, "prd.json")

	statePath := filepath.Join(featureDir, "run-state.json")
	def, defErr := LoadPRDDefinition(prdJsonPath)
	if defErr != nil {
		t.Fatalf("Failed to load PRDDefinition: %v", defErr)
	}
	state, stateErr := LoadRunState(statePath)
	if stateErr != nil {
		t.Fatalf("Failed to load RunState: %v", stateErr)
	}
	_, passed, skipped, _, _ := countStories(def, state)

	if passed == len(def.UserStories) {
		saveSkippedPhase(env, "09-refine", fmt.Sprintf("All %d stories passed — no refinement needed", passed))
		t.Skip("All stories passed — skipping refinement")
	}

	t.Logf("Not all stories passed (%d/%d, %d skipped) — resetting for retry", passed, len(def.UserStories), skipped)

	// Reset skipped stories so they can be retried
	modified := false
	for _, s := range def.UserStories {
		if state.IsSkipped(s.ID) {
			state.UnmarkPassed(s.ID) // clear from skipped
			// Remove from Skipped list
			newSkipped := make([]string, 0, len(state.Skipped))
			for _, id := range state.Skipped {
				if id != s.ID {
					newSkipped = append(newSkipped, id)
				}
			}
			state.Skipped = newSkipped
			delete(state.Retries, s.ID)
			delete(state.LastFailure, s.ID)
			modified = true
			t.Logf("  Reset skipped story: %s", s.ID)
		}
	}

	if modified {
		if err := SaveRunState(statePath, state); err != nil {
			t.Fatalf("Failed to save run-state.json: %v", err)
		}
		copyArtifact(env, "prd-refined.json", statePath)
	} else {
		savePhaseOutput(env, "09-refine", ralphResult{Stdout: "No skipped stories to reset"})
		t.Log("No skipped stories to reset — second run will retry failed stories")
	}
}

// phase10SecondRun runs ralph again if stories are still pending.
func phase10SecondRun(t *testing.T, env *testEnv) {
	featureDir := findFeatureDir(t, env.projectDir, env.featureName)
	prdJsonPath := filepath.Join(featureDir, "prd.json")

	def, state := loadPRDAndState(t, featureDir)
	_, passed, _, pending, _ := countStories(def, state)

	if passed == len(def.UserStories) {
		saveSkippedPhase(env, "10-second-run", fmt.Sprintf("All %d stories already passed", passed))
		t.Skip("All stories already passed — skipping second run")
	}
	if pending == 0 {
		saveSkippedPhase(env, "10-second-run", "No pending stories — all are passed or skipped")
		t.Skip("No pending stories — all are passed or skipped")
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
	if def, err := LoadPRDDefinition(prdJsonPath); err == nil {
		storyID := ""
		if len(def.UserStories) > 0 {
			storyID = def.UserStories[0].ID
		}
		if storyID != "" {
			result = runRalph(t, env.projectDir, "logs", env.featureName, "--story", storyID)
			savePhaseOutput(env, "11-logs-story", result)
			if !result.Success() {
				t.Errorf("ralph logs --story %s failed: %s", storyID, result.Combined())
			}
		}
	}

}

// phase12Verify runs ralph verify if all stories passed.
func phase12Verify(t *testing.T, env *testEnv) {
	featureDir := findFeatureDir(t, env.projectDir, env.featureName)

	def, state := loadPRDAndState(t, featureDir)
	_, passed, _, _, _ := countStories(def, state)

	if passed != len(def.UserStories) {
		saveSkippedPhase(env, "12-verify",
			fmt.Sprintf("Not all stories passed (%d/%d)", passed, len(def.UserStories)))
		t.Skipf("Not all stories passed (%d/%d) — skipping verify", passed, len(def.UserStories))
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
	var def *PRDDefinition
	var state *RunState
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
			statePath := filepath.Join(featureDir, "run-state.json")
			if d, err := LoadPRDDefinition(prdJsonPath); err == nil {
				def = d
				state, _ = LoadRunState(statePath)
			}
		}
	}

	if def == nil {
		report.WriteString("## Result: INCOMPLETE\n\n")
		report.WriteString("PRD was not created — test failed in early phases.\n")
		report.WriteString("Check `phases/` directory for command output.\n")
		saveArtifact(env, "report.md", report.String())
		saveArtifact(env, "result.txt", "INCOMPLETE\n")
		writeSummaryJSON(env, "INCOMPLETE", nil, nil, cfg, nil, "", 0)
		t.Log(report.String())
		return
	}

	total, passed, skipped, pending, _ := countStories(def, state)

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
	report.WriteString(fmt.Sprintf("**Stories:** %d total, %d passed, %d skipped, %d pending\n",
		total, passed, skipped, pending))
	if def.Description != "" {
		report.WriteString(fmt.Sprintf("**Description:** %s\n", def.Description))
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

	for _, s := range def.UserStories {
		status := storyStatusStr(s.ID, state)
		icon := "?"
		switch status {
		case "PASSED":
			icon = "PASS"
		case "SKIPPED":
			icon = "SKIP"
		case "PENDING":
			icon = "PEND"
		}

		report.WriteString(fmt.Sprintf("### [%s] %s: %s\n\n", icon, s.ID, s.Title))
		report.WriteString(fmt.Sprintf("**Description:** %s\n\n", s.Description))

		if len(s.Tags) > 0 {
			report.WriteString(fmt.Sprintf("**Tags:** %s\n", strings.Join(s.Tags, ", ")))
		}
		report.WriteString(fmt.Sprintf("**Priority:** %d | **Retries:** %d\n\n", s.Priority, state.GetRetries(s.ID)))

		// Acceptance criteria as checklist
		report.WriteString("**Acceptance Criteria:**\n")
		for _, ac := range s.AcceptanceCriteria {
			check := " "
			if state.IsPassed(s.ID) {
				check = "x"
			}
			report.WriteString(fmt.Sprintf("- [%s] %s\n", check, ac))
		}
		report.WriteString("\n")

		// Failed/skipped: include failure details inline
		if !state.IsPassed(s.ID) && state.GetLastFailure(s.ID) != "" {
			report.WriteString("**Failure Details:**\n```\n")
			report.WriteString(state.GetLastFailure(s.ID))
			report.WriteString("\n```\n\n")
		}

		report.WriteString("---\n\n")

		// Write per-story artifacts
		writeStoryArtifact(env, s, state, logEvents)
	}

	// ============================================================
	// Learnings
	// ============================================================
	if len(state.Learnings) > 0 {
		report.WriteString("## Learnings\n\n")
		for _, l := range state.Learnings {
			report.WriteString(fmt.Sprintf("- %s\n", l))
		}
		report.WriteString("\n")

		var learningsBuf strings.Builder
		for i, l := range state.Learnings {
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
	writeSummaryJSON(env, result, def, state, cfg, logEvents, branch, runNumber)

	// Print summary to test output
	t.Log("============================================================")
	t.Log(" E2E Test Report")
	t.Log("============================================================")
	t.Logf(" Result: %s", result)
	t.Logf(" Stories: %d total, %d passed, %d skipped, %d pending", total, passed, skipped, pending)
	t.Log("")

	for _, s := range def.UserStories {
		t.Logf("   [%s] %s: %s (retries=%d)", storyStatusStr(s.ID, state), s.ID, s.Title, state.GetRetries(s.ID))
		if !state.IsPassed(s.ID) {
			if failMsg := state.GetLastFailure(s.ID); failMsg != "" {
				lines := strings.SplitN(failMsg, "\n", 3)
				for _, line := range lines[:min(len(lines), 2)] {
					t.Logf("         %s", truncate(line, 120))
				}
			}
		}
	}

	if len(state.Learnings) > 0 {
		t.Logf("\n Learnings: %d captured", len(state.Learnings))
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
		if ev.Type == "iteration_start" && ev.StoryID == storyID {
			inStory = true
			continue
		}
		if ev.Type == "iteration_end" && ev.StoryID == storyID {
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
		if ev.Type == "iteration_start" && ev.StoryID == storyID {
			inStory = true
			continue
		}
		if ev.Type == "iteration_end" && ev.StoryID == storyID {
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

// writeStoryArtifact writes per-story files to stories/<id>/.
func writeStoryArtifact(env *testEnv, s StoryDefinition, state *RunState, logEvents []Event) {
	dir := strings.ToLower(s.ID)
	var buf strings.Builder

	status := storyStatusStr(s.ID, state)
	retries := state.GetRetries(s.ID)
	lastFailure := state.GetLastFailure(s.ID)
	passed := state.IsPassed(s.ID)

	fmt.Fprintf(&buf, "# %s: %s\n\n", s.ID, s.Title)
	fmt.Fprintf(&buf, "**Status:** %s\n", status)
	fmt.Fprintf(&buf, "**Priority:** %d\n", s.Priority)
	fmt.Fprintf(&buf, "**Retries:** %d\n", retries)
	if len(s.Tags) > 0 {
		fmt.Fprintf(&buf, "**Tags:** %s\n", strings.Join(s.Tags, ", "))
	}
	buf.WriteString("\n")

	fmt.Fprintf(&buf, "## Description\n\n%s\n\n", s.Description)

	buf.WriteString("## Acceptance Criteria\n\n")
	for _, ac := range s.AcceptanceCriteria {
		check := " "
		if passed {
			check = "x"
		}
		fmt.Fprintf(&buf, "- [%s] %s\n", check, ac)
	}
	buf.WriteString("\n")

	// Failure info
	if !passed && lastFailure != "" {
		buf.WriteString("## Failure Details\n\n")
		buf.WriteString("```\n")
		buf.WriteString(lastFailure)
		buf.WriteString("\n```\n")
	}

	saveArtifact(env, filepath.Join("stories", dir, "summary.md"), buf.String())

	// Save raw failure notes for easy grep
	if !passed && lastFailure != "" {
		saveArtifact(env, filepath.Join("stories", dir, "failure.txt"), lastFailure)
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
	SkippedStories int                `json:"skipped_stories"`
	PendingStories int                `json:"pending_stories"`
	Stories        []storySummaryJSON `json:"stories"`
	Learnings      []string           `json:"learnings,omitempty"`
	Commits        int                `json:"commits,omitempty"`
}

// writeSummaryJSON writes a machine-readable summary.json for AI agent consumption.
func writeSummaryJSON(env *testEnv, result string, def *PRDDefinition, state *RunState, cfg *RalphConfig, logEvents []Event, branch string, runNumber int) {
	s := summaryJSON{
		Result:    result,
		Feature:   env.featureName,
		Date:      time.Now().Format(time.RFC3339),
		Branch:    branch,
		RunNumber: runNumber,
	}

	if def != nil {
		s.Description = def.Description
		total, passed, skipped, pending, _ := countStories(def, state)
		s.TotalStories = total
		s.PassedStories = passed
		s.SkippedStories = skipped
		s.PendingStories = pending
		if state != nil {
			s.Learnings = state.Learnings
		}

		for _, story := range def.UserStories {
			ss := storySummaryJSON{
				ID:      story.ID,
				Title:   story.Title,
				Status:  storyStatusStr(story.ID, state),
				Retries: state.GetRetries(story.ID),
				Tags:    story.Tags,
			}
			if !state.IsPassed(story.ID) {
				if failure := state.GetLastFailure(story.ID); failure != "" {
					ss.FailureReason = truncate(failure, 500)
				}
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
