//go:build e2e

package main

import (
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

// scripBin is the path to the compiled scrip binary, set in TestMain.
var scripBin string

func TestMain(m *testing.M) {
	// Build scrip binary into a temp directory
	tmpDir, err := os.MkdirTemp("", "scrip-e2e-*")
	if err != nil {
		log.Fatalf("Failed to create temp dir: %v", err)
	}

	scripBin = filepath.Join(tmpDir, "scrip")
	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", scripBin, ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tmpDir)
		log.Fatalf("Failed to build scrip: %v", err)
	}

	// Verify prerequisites
	for _, bin := range []string{"claude", "git"} {
		if _, err := exec.LookPath(bin); err != nil {
			os.RemoveAll(tmpDir)
			log.Fatalf("E2E tests require %s in PATH", bin)
		}
	}

	code := m.Run()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

// processTracker keeps track of running scrip subprocesses for cleanup on test abort.
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
	scripBin    string
	projectDir  string
	featureName string
	artifactDir string // persistent output directory for this run
}

// scripResult captures the output and exit code of a scrip invocation.
type scripResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

func (r scripResult) Success() bool {
	return r.ExitCode == 0
}

func (r scripResult) Combined() string {
	return r.Stdout + r.Stderr
}

// buildEnv returns environment variables for subprocess isolation.
func buildEnv(projectDir string) []string {
	env := os.Environ()
	env = append(env,
		"GIT_AUTHOR_NAME=Scrip E2E Test",
		"GIT_AUTHOR_EMAIL=scrip-e2e@test.local",
		"GIT_COMMITTER_NAME=Scrip E2E Test",
		"GIT_COMMITTER_EMAIL=scrip-e2e@test.local",
	)
	return env
}

// initArtifactDir creates the artifact directory for this run.
// Override with SCRIP_E2E_ARTIFACT_DIR env var.
func initArtifactDir(t *testing.T) string {
	t.Helper()

	base := os.Getenv("SCRIP_E2E_ARTIFACT_DIR")
	if base == "" {
		base = "e2e-runs"
	}

	timestamp := time.Now().Format("2006-01-02T15-04-05")
	dir := filepath.Join(base, timestamp)

	for _, sub := range []string{"phases", "logs"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			t.Fatalf("Failed to create artifact dir %s: %v", filepath.Join(dir, sub), err)
		}
	}

	t.Logf("Artifact directory: %s", dir)
	return dir
}

// savePhaseOutput writes a scrip result to the phases/ artifact directory.
func savePhaseOutput(env *testEnv, phase string, result scripResult) {
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

// runScrip executes scrip with no stdin and returns the result.
func runScrip(t *testing.T, dir string, args ...string) scripResult {
	t.Helper()
	return runScripWithTimeout(t, dir, 2*time.Minute, args...)
}

// runScripWithTimeout executes scrip with no stdin and a custom timeout.
func runScripWithTimeout(t *testing.T, dir string, timeout time.Duration, args ...string) scripResult {
	t.Helper()

	cmd := exec.Command(scripBin, args...)
	cmd.Dir = dir
	cmd.Env = buildEnv(dir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return scripResult{Err: err, ExitCode: -1}
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
		return scripResult{
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
		return scripResult{
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
	used     bool   // track if already triggered (each response fires at most once)
}

// runScripInteractive runs scrip with expect-style stdin interaction.
// It reads stdout/stderr byte-by-byte, checking partial lines for prompt
// patterns. Critical because prompts like "Finalize? (yes/no): " don't end
// with newlines — bufio.Scanner would deadlock.
func runScripInteractive(t *testing.T, dir string, timeout time.Duration,
	responses []promptResponse, args ...string) scripResult {
	t.Helper()

	cmd := exec.Command(scripBin, args...)
	cmd.Dir = dir
	cmd.Env = buildEnv(dir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return scripResult{Err: fmt.Errorf("stdin pipe: %w", err), ExitCode: -1}
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return scripResult{Err: fmt.Errorf("stdout pipe: %w", err), ExitCode: -1}
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return scripResult{Err: fmt.Errorf("stderr pipe: %w", err), ExitCode: -1}
	}

	if err := cmd.Start(); err != nil {
		return scripResult{Err: fmt.Errorf("start: %w", err), ExitCode: -1}
	}
	activeProcs.track(cmd.Process)

	var stdoutBuf, stderrBuf bytes.Buffer
	var mu sync.Mutex

	// readStream reads from a pipe in small chunks, processing bytes incrementally.
	// Checks partial lines against prompt patterns so prompts without trailing
	// newlines are matched immediately.
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
								time.Sleep(100 * time.Millisecond)
								io.WriteString(stdinPipe, responses[matchIdx].response+"\n")
								t.Logf("RESPOND: matched %q -> sent %q",
									responses[matchIdx].pattern, responses[matchIdx].response)
								currentLine.Reset()
								matchedOnCurrentLine = false
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
		return scripResult{
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
		return scripResult{
			Stdout:   stdoutBuf.String(),
			Stderr:   stderrBuf.String(),
			ExitCode: -1,
			Err:      fmt.Errorf("command timed out after %v", timeout),
		}
	}
}

// --- Test project setup ---

// setupTestProject creates a minimal Go project with intentional gaps for scrip to fix.
// Returns the project directory path.
func setupTestProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	// Initialize git repo
	for _, args := range [][]string{
		{"init"},
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
	writeTestFile(t, dir, "go.mod", `module example.com/e2e-test

go 1.21
`)

	// main.go — simple HTTP server
	writeTestFile(t, dir, "main.go", `package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/greet", greetHandler)
	http.HandleFunc("/items", itemsHandler)

	log.Printf("Listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "ok")
}

func greetHandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "world"
	}
	fmt.Fprintf(w, "Hello, %s!\n", name)
}

// TODO: implement items CRUD
func itemsHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
`)

	// Initial commit
	gitCmd := exec.Command("git", "add", "-A")
	gitCmd.Dir = dir
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	gitCmd = exec.Command("git", "commit", "-m", "initial commit")
	gitCmd.Dir = dir
	gitCmd.Env = buildEnv(dir)
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	return dir
}

// writeTestFile creates a file in the test project.
func writeTestFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
}

// --- Git helpers ---

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

// --- Scrip helpers ---

// findScripFeatureDir scans .scrip/ for a matching feature directory.
func findScripFeatureDir(t *testing.T, projectDir, feature string) string {
	t.Helper()
	scripDir := filepath.Join(projectDir, ".scrip")
	entries, err := os.ReadDir(scripDir)
	if err != nil {
		t.Fatalf("Failed to read .scrip dir: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Feature dirs are YYYY-MM-DD-<feature>
		parts := strings.SplitN(name, "-", 4)
		if len(parts) >= 4 && strings.EqualFold(parts[3], feature) {
			return filepath.Join(scripDir, name)
		}
	}
	t.Fatalf("No feature directory found for %q in %s", feature, scripDir)
	return ""
}

// loadScripConfig loads and parses a scrip config.json file.
func loadScripConfig(t *testing.T, path string) *ScripConfig {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read config from %s: %v", path, err)
	}
	var cfg ScripConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Failed to parse config from %s: %v", path, err)
	}
	return &cfg
}

// --- E2E Tests ---

// TestE2E runs the full scrip lifecycle: prep → plan → exec → land.
// Requires 'claude' in PATH with valid API key. Run with:
//
//	make test-e2e
func TestE2E(t *testing.T) {
	projectDir := setupTestProject(t)
	artifactDir := initArtifactDir(t)

	env := &testEnv{
		scripBin:    scripBin,
		projectDir:  projectDir,
		featureName: "items",
		artifactDir: artifactDir,
	}

	t.Cleanup(func() {
		activeProcs.killAll()
		// Save final state
		saveArtifact(env, "final-git-log.txt", gitLog(t, projectDir, 50))
		saveArtifact(env, "final-git-status.txt", gitWorkingTreeStatus(t, projectDir))

		// Copy scrip artifacts
		scripDir := filepath.Join(projectDir, ".scrip")
		if entries, err := os.ReadDir(scripDir); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					featurePath := filepath.Join(scripDir, e.Name())
					copyFeatureArtifacts(env, featurePath)
				} else {
					copyArtifact(env, filepath.Join("scrip", e.Name()),
						filepath.Join(scripDir, e.Name()))
				}
			}
		}
	})

	// Phase 1: prep
	t.Run("prep", func(t *testing.T) {
		result := runScrip(t, projectDir, "prep")
		savePhaseOutput(env, "prep", result)

		if !result.Success() {
			t.Fatalf("scrip prep failed (exit %d):\n%s", result.ExitCode, result.Combined())
		}

		// Verify config created
		configPath := filepath.Join(projectDir, ".scrip", "config.json")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Fatal("scrip prep did not create .scrip/config.json")
		}

		cfg := loadScripConfig(t, configPath)
		if cfg.Project.Type == "" {
			t.Error("config missing project type")
		}
		if cfg.Verify.Test == "" {
			t.Error("config missing verify.test command")
		}

		t.Logf("Project detected: type=%s", cfg.Project.Type)
	})

	// Phase 2: plan
	t.Run("plan", func(t *testing.T) {
		result := runScripInteractive(t, projectDir, 10*time.Minute,
			[]promptResponse{
				// Accept plan draft if offered
				{pattern: "Finalize? (yes/no):", response: "yes"},
				// Fall back to finalizing after first round
				{pattern: "Feedback (or 'done' to finalize", response: "done"},
			},
			"plan", env.featureName, "Implement items CRUD — list, create, delete endpoints with in-memory storage and tests",
		)
		savePhaseOutput(env, "plan", result)

		if !result.Success() {
			t.Fatalf("scrip plan failed (exit %d):\n%s", result.ExitCode, result.Combined())
		}

		// Verify plan artifacts
		featureDir := findScripFeatureDir(t, projectDir, env.featureName)

		planMd := filepath.Join(featureDir, "plan.md")
		if _, err := os.Stat(planMd); os.IsNotExist(err) {
			t.Fatal("scrip plan did not create plan.md")
		}

		planJsonl := filepath.Join(featureDir, "plan.jsonl")
		if _, err := os.Stat(planJsonl); os.IsNotExist(err) {
			t.Fatal("scrip plan did not create plan.jsonl")
		}

		// Parse plan.md to verify structure
		planContent, err := os.ReadFile(planMd)
		if err != nil {
			t.Fatalf("Failed to read plan.md: %v", err)
		}
		plan, err := ParsePlan(string(planContent))
		if err != nil {
			t.Fatalf("Failed to parse plan.md: %v", err)
		}
		if len(plan.Items) == 0 {
			t.Fatal("plan.md has no items")
		}
		t.Logf("Plan created with %d items", len(plan.Items))

		// Verify on feature branch
		branch := gitBranch(t, projectDir)
		if !strings.HasPrefix(branch, "plan/") {
			t.Errorf("Expected plan/ branch, got %s", branch)
		}
	})

	// Phase 3: exec
	t.Run("exec", func(t *testing.T) {
		result := runScripWithTimeout(t, projectDir, 30*time.Minute,
			"exec", env.featureName)
		savePhaseOutput(env, "exec", result)

		if !result.Success() {
			t.Fatalf("scrip exec failed (exit %d):\n%s", result.ExitCode, result.Combined())
		}

		// Verify progress events were written
		featureDir := findScripFeatureDir(t, projectDir, env.featureName)
		progressJsonl := filepath.Join(featureDir, "progress.jsonl")
		if _, err := os.Stat(progressJsonl); os.IsNotExist(err) {
			t.Fatal("scrip exec did not create progress.jsonl")
		}

		events, err := LoadProgressEvents(progressJsonl)
		if err != nil {
			t.Fatalf("Failed to load progress events: %v", err)
		}

		// Should have at least exec_start and exec_end
		hasStart := false
		hasEnd := false
		passedCount := 0
		for _, e := range events {
			switch e.Event {
			case ProgressExecStart:
				hasStart = true
			case ProgressExecEnd:
				hasEnd = true
			case ProgressItemDone:
				if e.Status == "passed" {
					passedCount++
				}
			}
		}
		if !hasStart {
			t.Error("progress.jsonl missing exec_start event")
		}
		if !hasEnd {
			t.Error("progress.jsonl missing exec_end event")
		}
		t.Logf("Exec completed: %d items passed, %d total events", passedCount, len(events))

		// Verify progress.md was written
		progressMd := filepath.Join(featureDir, "progress.md")
		if _, err := os.Stat(progressMd); os.IsNotExist(err) {
			t.Error("scrip exec did not create progress.md")
		}
	})

	// Phase 4: land
	t.Run("land", func(t *testing.T) {
		result := runScripWithTimeout(t, projectDir, 15*time.Minute,
			"land", env.featureName)
		savePhaseOutput(env, "land", result)

		if !result.Success() {
			t.Logf("scrip land output:\n%s", result.Combined())
			// Land may fail if verification finds issues — log but don't fail
			// the whole test. The artifacts will show what happened.
			t.Errorf("scrip land exited with code %d (see artifacts for details)", result.ExitCode)
			return
		}

		// Verify summary was generated
		featureDir := findScripFeatureDir(t, projectDir, env.featureName)
		summaryMd := filepath.Join(featureDir, "summary.md")
		if _, err := os.Stat(summaryMd); os.IsNotExist(err) {
			t.Error("scrip land did not create summary.md")
		}

		// Verify plan.md was purged
		planMd := filepath.Join(featureDir, "plan.md")
		if _, err := os.Stat(planMd); err == nil {
			t.Error("scrip land did not purge plan.md")
		}

		t.Log("Land completed successfully")
	})
}

// TestE2EResume verifies crash recovery: kill mid-exec, restart, resume from progress.jsonl.
func TestE2EResume(t *testing.T) {
	projectDir := setupTestProject(t)
	artifactDir := initArtifactDir(t)

	env := &testEnv{
		scripBin:    scripBin,
		projectDir:  projectDir,
		featureName: "resume",
		artifactDir: artifactDir,
	}

	t.Cleanup(func() {
		activeProcs.killAll()
	})

	// Prep
	result := runScrip(t, projectDir, "prep")
	if !result.Success() {
		t.Fatalf("scrip prep failed: %v", result.Err)
	}

	// Plan
	result = runScripInteractive(t, projectDir, 10*time.Minute,
		[]promptResponse{
			{pattern: "Finalize? (yes/no):", response: "yes"},
			{pattern: "Feedback (or 'done' to finalize", response: "done"},
		},
		"plan", env.featureName, "Add a /version endpoint that returns the app version from an env var",
	)
	if !result.Success() {
		t.Fatalf("scrip plan failed: %v", result.Err)
	}
	savePhaseOutput(env, "plan", result)

	featureDir := findScripFeatureDir(t, projectDir, env.featureName)
	progressJsonl := filepath.Join(featureDir, "progress.jsonl")

	// First exec — kill after first item_start event appears in progress.jsonl
	cmd := exec.Command(scripBin, "exec", env.featureName)
	cmd.Dir = projectDir
	cmd.Env = buildEnv(projectDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start exec: %v", err)
	}
	activeProcs.track(cmd.Process)

	// Wait for first item_start (poll progress.jsonl)
	deadline := time.After(5 * time.Minute)
	for {
		select {
		case <-deadline:
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			t.Fatal("Timeout waiting for first item_start")
		default:
		}

		events, _ := LoadProgressEvents(progressJsonl)
		hasItemStart := false
		for _, e := range events {
			if e.Event == ProgressItemStart {
				hasItemStart = true
				break
			}
		}
		if hasItemStart {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// Kill the process group to simulate crash
	t.Log("Killing exec process to simulate crash")
	syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	cmd.Wait()
	activeProcs.untrack(cmd.Process.Pid)

	// Record pre-resume state
	eventsBefore, _ := LoadProgressEvents(progressJsonl)
	saveArtifact(env, "pre-resume-events.txt",
		fmt.Sprintf("Events before resume: %d", len(eventsBefore)))

	// Second exec — should resume
	result = runScripWithTimeout(t, projectDir, 20*time.Minute,
		"exec", env.featureName)
	savePhaseOutput(env, "exec-resume", result)

	if !result.Success() {
		t.Errorf("scrip exec resume failed (exit %d)", result.ExitCode)
	}

	eventsAfter, _ := LoadProgressEvents(progressJsonl)
	if len(eventsAfter) <= len(eventsBefore) {
		t.Error("No new progress events after resume")
	}
	t.Logf("Resume: %d events before, %d after", len(eventsBefore), len(eventsAfter))
}

// --- Helpers ---

// copyFeatureArtifacts copies all files from a feature dir into artifacts.
func copyFeatureArtifacts(env *testEnv, featurePath string) {
	base := filepath.Base(featurePath)
	entries, err := os.ReadDir(featurePath)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		copyArtifact(env, filepath.Join("scrip", base, e.Name()),
			filepath.Join(featurePath, e.Name()))
	}
}
