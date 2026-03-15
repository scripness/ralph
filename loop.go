package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// ProviderResult contains the result of a provider iteration
type ProviderResult struct {
	Output    string
	Done      bool
	Stuck     bool
	StuckNote string
	Learnings []string
	ExitCode  int
	TimedOut  bool
}

// StoryVerifyResult contains the result of story verification
type StoryVerifyResult struct {
	passed bool
	reason string
	output string // combined command output (available even on success)
}

// runCommand runs a shell command with a per-command timeout.
func runCommand(dir, cmdStr string, timeoutSec int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 100 * time.Millisecond
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return truncateOutput(buf.String(), 50), fmt.Errorf("timed out after %ds", timeoutSec)
	}
	return truncateOutput(buf.String(), 50), err
}

// truncateOutput keeps the last N lines of output for diagnostic context.
func truncateOutput(output string, maxLines int) string {
	lines := strings.Split(output, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}

