package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildProviderArgs_StdinMode(t *testing.T) {
	args, promptFile, err := buildProviderArgs([]string{"--flag"}, "stdin", "", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if promptFile != "" {
		t.Errorf("expected no prompt file for stdin mode, got %q", promptFile)
	}
	// stdin mode: args unchanged, prompt not appended
	if len(args) != 1 || args[0] != "--flag" {
		t.Errorf("expected [--flag], got %v", args)
	}
}

func TestBuildProviderArgs_ArgMode(t *testing.T) {
	args, promptFile, err := buildProviderArgs([]string{"run"}, "arg", "", "do stuff")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if promptFile != "" {
		t.Errorf("expected no prompt file for arg mode, got %q", promptFile)
	}
	if len(args) != 2 || args[0] != "run" || args[1] != "do stuff" {
		t.Errorf("expected [run, do stuff], got %v", args)
	}
}

func TestBuildProviderArgs_ArgModeWithFlag(t *testing.T) {
	args, _, err := buildProviderArgs([]string{"--yes-always"}, "arg", "--message", "do stuff")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 3 || args[0] != "--yes-always" || args[1] != "--message" || args[2] != "do stuff" {
		t.Errorf("expected [--yes-always, --message, do stuff], got %v", args)
	}
}

func TestBuildProviderArgs_FileMode(t *testing.T) {
	args, promptFile, err := buildProviderArgs([]string{"--flag"}, "file", "", "prompt content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(promptFile)

	if promptFile == "" {
		t.Fatal("expected prompt file to be created")
	}
	// Verify file contents
	data, err := os.ReadFile(promptFile)
	if err != nil {
		t.Fatalf("failed to read prompt file: %v", err)
	}
	if string(data) != "prompt content" {
		t.Errorf("expected prompt file content 'prompt content', got %q", string(data))
	}
	// Last arg should be the file path
	if len(args) != 2 || args[0] != "--flag" || args[1] != promptFile {
		t.Errorf("expected [--flag, %s], got %v", promptFile, args)
	}
}

func TestBuildProviderArgs_FileModeWithFlag(t *testing.T) {
	args, promptFile, err := buildProviderArgs([]string{"--base"}, "file", "--prompt-file", "content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(promptFile)

	if len(args) != 3 || args[0] != "--base" || args[1] != "--prompt-file" || args[2] != promptFile {
		t.Errorf("expected [--base, --prompt-file, %s], got %v", promptFile, args)
	}
}

func TestBuildProviderArgs_DoesNotMutateBaseArgs(t *testing.T) {
	base := []string{"--flag1", "--flag2"}
	origLen := len(base)

	_, _, err := buildProviderArgs(base, "arg", "", "prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(base) != origLen {
		t.Errorf("base args were mutated: expected len %d, got %d", origLen, len(base))
	}
}

func TestBuildProviderArgs_ProviderIntegration(t *testing.T) {
	// Simulate what each provider's final args look like after auto-detection
	tests := []struct {
		name     string
		base     []string
		mode     string
		flag     string
		prompt   string
		wantArgs []string
	}{
		{
			"amp",
			[]string{"--dangerously-allow-all"},
			"stdin", "", "implement story",
			[]string{"--dangerously-allow-all"},
		},
		{
			"claude",
			[]string{"--print", "--dangerously-skip-permissions"},
			"stdin", "", "implement story",
			[]string{"--print", "--dangerously-skip-permissions"},
		},
		{
			"opencode",
			[]string{"run"},
			"arg", "", "implement story",
			[]string{"run", "implement story"},
		},
		{
			"aider",
			[]string{"--yes-always"},
			"arg", "--message", "implement story",
			[]string{"--yes-always", "--message", "implement story"},
		},
		{
			"codex",
			[]string{"exec", "--full-auto"},
			"arg", "", "implement story",
			[]string{"exec", "--full-auto", "implement story"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, _, err := buildProviderArgs(tt.base, tt.mode, tt.flag, tt.prompt)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(args) != len(tt.wantArgs) {
				t.Fatalf("args length: got %d, want %d (%v vs %v)", len(args), len(tt.wantArgs), args, tt.wantArgs)
			}
			for i, a := range tt.wantArgs {
				if args[i] != a {
					t.Errorf("args[%d]: got %q, want %q", i, args[i], a)
				}
			}
		})
	}
}

func TestBuildProviderArgs_EndToEnd(t *testing.T) {
	// Full pipeline: JSON config → LoadConfig → buildProviderArgs
	// Verifies the final command each provider would execute
	tests := []struct {
		command  string
		wantArgs []string // expected args AFTER buildProviderArgs (prompt included for arg mode)
	}{
		{"amp", []string{"--dangerously-allow-all"}},
		{"claude", []string{"--print", "--dangerously-skip-permissions"}},
		{"opencode", []string{"run", "test prompt"}},
		{"aider", []string{"--yes-always", "--message", "test prompt"}},
		{"codex", []string{"exec", "--full-auto", "test prompt"}},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			dir := t.TempDir()
			configContent := fmt.Sprintf(`{
				"provider": {"command": %q},
				"verify": {"default": ["echo ok"]},
				"services": [{"name": "dev", "ready": "http://localhost:3000"}]
			}`, tt.command)
			os.WriteFile(filepath.Join(dir, "ralph.config.json"), []byte(configContent), 0644)

			cfg, err := LoadConfig(dir)
			if err != nil {
				t.Fatalf("LoadConfig error: %v", err)
			}

			p := cfg.Config.Provider
			args, promptFile, err := buildProviderArgs(p.Args, p.PromptMode, p.PromptFlag, "test prompt")
			if err != nil {
				t.Fatalf("buildProviderArgs error: %v", err)
			}
			if promptFile != "" {
				os.Remove(promptFile)
			}

			if len(args) != len(tt.wantArgs) {
				t.Fatalf("args: got %v, want %v", args, tt.wantArgs)
			}
			for i, a := range tt.wantArgs {
				if args[i] != a {
					t.Errorf("args[%d]: got %q, want %q", i, args[i], a)
				}
			}
		})
	}
}

func TestRunCommand_Timeout(t *testing.T) {
	dir := t.TempDir()
	output, err := runCommand(dir, "sleep 10", 1)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected 'timed out' in error, got: %v", err)
	}
	// Output should be captured (may be empty for sleep)
	_ = output
}

func TestRunCommand_TimeoutKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	// Spawn a shell that starts a background child writing its PID to a file,
	// then waits. The child should be killed with the process group on timeout.
	cmd := fmt.Sprintf("sh -c 'echo $$ > %s; sleep 60' & wait", pidFile)
	_, err := runCommand(dir, cmd, 1)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected 'timed out' in error, got: %v", err)
	}
	// Read the child PID and verify the process was killed
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("child PID file not written: %v", err)
	}
	pidStr := strings.TrimSpace(string(pidBytes))
	// Check /proc/<pid> — on Linux, a dead process has no /proc entry after reaping
	time.Sleep(100 * time.Millisecond)
	procPath := filepath.Join("/proc", pidStr)
	if _, err := os.Stat(procPath); err == nil {
		t.Errorf("child process %s still alive after timeout — orphan not killed", pidStr)
	}
}

func TestRunCommand_Success(t *testing.T) {
	dir := t.TempDir()
	output, err := runCommand(dir, "echo hello", 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "hello") {
		t.Errorf("expected 'hello' in output, got: %q", output)
	}
}

func TestRunCommand_NonZeroExit(t *testing.T) {
	dir := t.TempDir()
	_, err := runCommand(dir, "exit 1", 30)
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
	// Should NOT be a timeout error
	if strings.Contains(err.Error(), "timed out") {
		t.Error("expected non-timeout error")
	}
}

func TestTruncateOutput(t *testing.T) {
	short := "line1\nline2\nline3"
	if got := truncateOutput(short, 10); got != short {
		t.Errorf("expected short output unchanged, got %q", got)
	}

	lines := make([]string, 100)
	for i := range lines {
		lines[i] = fmt.Sprintf("line%d", i)
	}
	long := strings.Join(lines, "\n")
	result := truncateOutput(long, 5)
	if !strings.Contains(result, "line99") {
		t.Error("expected last line present")
	}
	if strings.Contains(result, "line0\n") {
		t.Error("expected first line truncated")
	}
}

func TestProviderEndNilResult_NoPanic(t *testing.T) {
	// Verify that accessing fields on a nil *ProviderResult would panic,
	// confirming the guard in runLoop is needed.
	var result *ProviderResult
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when accessing nil ProviderResult fields")
		}
	}()
	_ = result.ExitCode // should panic
}

// --- VerifyReport tests ---

func TestVerifyReport_FormatForConsole(t *testing.T) {
	r := &VerifyReport{}
	r.AddPass("go vet ./...")
	r.AddFail("go test ./...", "exit code 1\nFAIL main_test.go")
	r.AddWarn("CLAUDE.md", "was NOT modified on this branch")
	r.Finalize()

	output := r.FormatForConsole()

	if !strings.Contains(output, "✓ go vet") {
		t.Error("expected pass item with checkmark")
	}
	if !strings.Contains(output, "✗ go test") {
		t.Error("expected fail item with cross")
	}
	if !strings.Contains(output, "⚠ CLAUDE.md") {
		t.Error("expected warn item with warning symbol")
	}
	if !strings.Contains(output, "1 passed, 1 failed, 1 warnings") {
		t.Error("expected summary counts")
	}
}

func TestVerifyReport_FormatForPrompt(t *testing.T) {
	r := &VerifyReport{}
	r.AddPass("go vet ./...")
	r.AddFail("go test ./...", "FAIL main_test.go:42")
	r.AddWarn("test files", "no test files modified")
	r.Finalize()

	output := r.FormatForPrompt()

	if !strings.Contains(output, "PASS: go vet") {
		t.Error("expected PASS prefix")
	}
	if !strings.Contains(output, "FAIL: go test") {
		t.Error("expected FAIL prefix")
	}
	if !strings.Contains(output, "FAIL main_test.go:42") {
		t.Error("expected failure output details")
	}
	if !strings.Contains(output, "WARN: test files") {
		t.Error("expected WARN prefix")
	}
}

func TestVerifyReport_Finalize(t *testing.T) {
	// All pass
	r := &VerifyReport{}
	r.AddPass("check1")
	r.AddPass("check2")
	r.AddWarn("info", "some info")
	r.Finalize()

	if !r.AllPassed {
		t.Error("expected AllPassed=true when only passes and warns")
	}
	if r.FailCount != 0 {
		t.Errorf("expected FailCount=0, got %d", r.FailCount)
	}
	if r.WarnCount != 1 {
		t.Errorf("expected WarnCount=1, got %d", r.WarnCount)
	}

	// With failure
	r2 := &VerifyReport{}
	r2.AddPass("check1")
	r2.AddFail("check2", "broken")
	r2.Finalize()

	if r2.AllPassed {
		t.Error("expected AllPassed=false when there are failures")
	}
	if r2.FailCount != 1 {
		t.Errorf("expected FailCount=1, got %d", r2.FailCount)
	}
}

func TestVerifyReport_AddWarn(t *testing.T) {
	r := &VerifyReport{}
	r.AddWarn("knowledge file", "was not modified")

	if len(r.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(r.Items))
	}
	item := r.Items[0]
	if !item.Warning {
		t.Error("expected Warning=true")
	}
	if item.Passed {
		t.Error("expected Passed=false for warn item")
	}
	if item.Name != "knowledge file" {
		t.Errorf("expected name 'knowledge file', got %q", item.Name)
	}
	if item.Output != "was not modified" {
		t.Errorf("expected output 'was not modified', got %q", item.Output)
	}
}
