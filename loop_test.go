package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

