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

// Provider signal markers (ralph v2 — kept for test compatibility)
const (
	DoneMarker  = "<ralph>DONE</ralph>"
	StuckMarker = "<ralph>STUCK</ralph>"
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

// --- VerifyReport types (used by legacy prompt generators in prompts.go) ---

// VerifyItem represents a single verification check result
type VerifyItem struct {
	Name    string
	Passed  bool
	Output  string // truncated command output for failures
	Warning bool   // true for WARN-type items
}

// VerifyReport collects all verification results
type VerifyReport struct {
	Items     []VerifyItem
	AllPassed bool
	FailCount int
	WarnCount int
}

func (r *VerifyReport) AddPass(name string) {
	r.Items = append(r.Items, VerifyItem{Name: name, Passed: true})
}

func (r *VerifyReport) AddFail(name, output string) {
	r.Items = append(r.Items, VerifyItem{Name: name, Passed: false, Output: output})
}

func (r *VerifyReport) AddWarn(name, detail string) {
	r.Items = append(r.Items, VerifyItem{Name: name, Warning: true, Output: detail})
}

func (r *VerifyReport) Finalize() {
	r.AllPassed = true
	r.FailCount = 0
	r.WarnCount = 0
	for _, item := range r.Items {
		if item.Warning {
			r.WarnCount++
		} else if !item.Passed {
			r.FailCount++
			r.AllPassed = false
		}
	}
}

// FormatForConsole returns human-readable output for terminal
func (r *VerifyReport) FormatForConsole() string {
	var lines []string
	lines = append(lines, "Verification Results")
	lines = append(lines, strings.Repeat("=", 60))
	for _, item := range r.Items {
		if item.Warning {
			lines = append(lines, fmt.Sprintf("  ⚠ %s", item.Name))
			if item.Output != "" {
				lines = append(lines, fmt.Sprintf("      %s", item.Output))
			}
		} else if item.Passed {
			lines = append(lines, fmt.Sprintf("  ✓ %s", item.Name))
		} else {
			lines = append(lines, fmt.Sprintf("  ✗ %s", item.Name))
			if item.Output != "" {
				outputLines := strings.Split(item.Output, "\n")
				if len(outputLines) > 10 {
					outputLines = outputLines[len(outputLines)-10:]
				}
				for _, ol := range outputLines {
					if ol != "" {
						lines = append(lines, fmt.Sprintf("      %s", ol))
					}
				}
			}
		}
	}
	lines = append(lines, strings.Repeat("=", 60))

	passCount := 0
	for _, item := range r.Items {
		if item.Passed && !item.Warning {
			passCount++
		}
	}
	lines = append(lines, fmt.Sprintf("  %d passed, %d failed, %d warnings", passCount, r.FailCount, r.WarnCount))
	return strings.Join(lines, "\n")
}

// FormatForPrompt returns detailed output suitable for AI prompt context
func (r *VerifyReport) FormatForPrompt() string {
	var lines []string
	for _, item := range r.Items {
		if item.Warning {
			lines = append(lines, fmt.Sprintf("WARN: %s — %s", item.Name, item.Output))
		} else if item.Passed {
			lines = append(lines, fmt.Sprintf("PASS: %s", item.Name))
		} else {
			lines = append(lines, fmt.Sprintf("FAIL: %s", item.Name))
			if item.Output != "" {
				lines = append(lines, item.Output)
			}
		}
	}
	return strings.Join(lines, "\n")
}
