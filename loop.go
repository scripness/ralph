package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
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
}

// buildProviderArgs builds the final argument list for a provider subprocess.
func buildProviderArgs(baseArgs []string, promptMode, promptFlag, prompt string) (args []string, promptFile string, err error) {
	args = append([]string{}, baseArgs...)

	switch promptMode {
	case "arg":
		if promptFlag != "" {
			args = append(args, promptFlag)
		}
		args = append(args, prompt)
	case "file":
		f, ferr := os.CreateTemp("", "scrip-prompt-*.md")
		if ferr != nil {
			return nil, "", fmt.Errorf("failed to create temp prompt file: %w", ferr)
		}
		promptFile = f.Name()
		if _, ferr := f.WriteString(prompt); ferr != nil {
			f.Close()
			os.Remove(promptFile)
			return nil, "", fmt.Errorf("failed to write prompt file: %w", ferr)
		}
		f.Close()
		if promptFlag != "" {
			args = append(args, promptFlag)
		}
		args = append(args, promptFile)
	}
	// "stdin" mode doesn't modify args

	return args, promptFile, nil
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

