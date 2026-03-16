package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Scrip provider signal markers
const (
	ScripDoneMarker  = "<scrip>DONE</scrip>"
	ScripStuckMarker = "<scrip>STUCK</scrip>"
	scripMaxRetries  = 3
)

var (
	scripLearningRe  = regexp.MustCompile(`^<scrip>LEARNING:(.+?)</scrip>$`)
	scripStuckNoteRe = regexp.MustCompile(`^<scrip>STUCK:(.+?)</scrip>$`)
)

// Scrip verification markers (used by plan verification and landing analysis)
var (
	scripVerifyPassRe = regexp.MustCompile(`^\s*<scrip>VERIFY_PASS</scrip>\s*$`)
	scripVerifyFailRe = regexp.MustCompile(`^\s*<scrip>VERIFY_FAIL:(.+)</scrip>\s*$`)
)

// scripSpawnProvider spawns claude with scrip-specific args and processes output.
// Always uses stdin mode (claude reads prompt from stdin).
// stallTimeoutSec controls idle-output detection: if the provider produces no output
// for this many seconds, it is killed (same as hard timeout). 0 disables stall detection.
func scripSpawnProvider(projectRoot, prompt string, timeoutSec, stallTimeoutSec int, autonomous bool, logger *RunLogger, cleanup *CleanupCoordinator, sessState *SessionState, statePath string) (*ProviderResult, error) {
	timeout := time.Duration(timeoutSec) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := ScripProviderArgs(autonomous)
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = projectRoot
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 5 * time.Second

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start claude: %w", err)
	}

	if cleanup != nil {
		cleanup.SetProvider(cmd)
		defer cleanup.ClearProvider()
	}

	// Record provider PID in session state for crash recovery
	if sessState != nil && statePath != "" {
		sessState.SetProvider(cmd.Process.Pid)
		_ = SaveSessionState(statePath, sessState)
	}

	// Write prompt to stdin
	go func() {
		defer stdinPipe.Close()
		io.WriteString(stdinPipe, prompt)
	}()

	// Collect output with marker detection
	var mu sync.Mutex
	var outputBuilder strings.Builder
	result := &ProviderResult{}

	// Stall timeout: kill provider if no output for stallTimeoutSec seconds.
	// Activity channel is buffered so scanner loops never block on send.
	var stallTimedOut atomic.Bool
	activityCh := make(chan struct{}, 1)
	if stallTimeoutSec > 0 {
		stallDuration := time.Duration(stallTimeoutSec) * time.Second
		go func() {
			stallTimer := time.NewTimer(stallDuration)
			defer stallTimer.Stop()
			for {
				select {
				case <-activityCh:
					if !stallTimer.Stop() {
						select {
						case <-stallTimer.C:
						default:
						}
					}
					stallTimer.Reset(stallDuration)
				case <-stallTimer.C:
					stallTimedOut.Store(true)
					cancel()
					return
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// notifyActivity sends a non-blocking signal on the activity channel.
	notifyActivity := func() {
		select {
		case activityCh <- struct{}{}:
		default:
		}
	}

	var wg sync.WaitGroup
	wg.Add(1)

	// Read stderr
	go func() {
		defer wg.Done()
		s := bufio.NewScanner(stderr)
		s.Buffer(make([]byte, 64*1024), 1024*1024)
		for s.Scan() {
			line := s.Text()
			notifyActivity()
			if logger != nil {
				logger.ProviderLine("stderr", line)
			}
			mu.Lock()
			outputBuilder.WriteString(line + "\n")
			scripProcessLine(line, result, logger)
			mu.Unlock()
		}
	}()

	// Read stdout
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		notifyActivity()
		if logger != nil {
			logger.ProviderLine("stdout", line)
		}
		mu.Lock()
		outputBuilder.WriteString(line + "\n")
		scripProcessLine(line, result, logger)
		mu.Unlock()
	}

	if scanErr := scanner.Err(); scanErr != nil && logger != nil {
		logger.Warning(fmt.Sprintf("stdout scanner error (possible line >1MB): %v", scanErr))
	}

	wg.Wait()

	err = cmd.Wait()
	result.Output = outputBuilder.String()

	if ctx.Err() == context.DeadlineExceeded || stallTimedOut.Load() {
		result.TimedOut = true
		if stallTimedOut.Load() {
			return result, fmt.Errorf("provider stalled (no output for %ds)", stallTimeoutSec)
		}
		return result, fmt.Errorf("provider timed out after %v", timeout)
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
		return result, nil // Non-zero exit is not an error — markers determine outcome
	}

	result.ExitCode = 0
	return result, nil
}

// scripProcessLine detects scrip markers in provider output.
// Uses whole-line matching (after trimming) to prevent marker spoofing.
func scripProcessLine(line string, result *ProviderResult, logger *RunLogger) {
	trimmed := strings.TrimSpace(line)

	if trimmed == ScripDoneMarker {
		result.Done = true
		if logger != nil {
			logger.MarkerDetected("DONE", "")
			logger.LogPrintln("  ◆ DONE")
		}
	}

	if trimmed == ScripStuckMarker {
		result.Stuck = true
		if logger != nil {
			logger.MarkerDetected("STUCK", "")
			logger.LogPrintln("  ◆ STUCK")
		}
	}

	if matches := scripStuckNoteRe.FindStringSubmatch(trimmed); len(matches) > 1 {
		result.Stuck = true
		result.StuckNote = strings.TrimSpace(matches[1])
		if logger != nil {
			logger.MarkerDetected("STUCK", result.StuckNote)
			logger.LogPrint("  ◆ STUCK: %s\n", result.StuckNote)
		}
	}

	if matches := scripLearningRe.FindStringSubmatch(trimmed); len(matches) > 1 {
		value := strings.TrimSpace(matches[1])
		result.Learnings = append(result.Learnings, value)
		if logger != nil {
			logger.MarkerDetected("LEARNING", value)
			logger.LogPrint("  ~ LEARNING: %s\n", value)
		}
	}
}

// scripRunVerify runs verification commands from ScripVerifyConfig.
// Returns a result indicating pass/fail with reason.
func scripRunVerify(projectRoot string, verify *ScripVerifyConfig, timeoutSec int, logger *RunLogger) *ItemVerifyResult {
	result := &ItemVerifyResult{passed: true}
	var allOutput []string

	for _, cmd := range verify.VerifyCommands() {
		if logger != nil {
			logger.LogPrint("  → %s\n", cmd)
			logger.VerifyCmdStart(cmd)
		}
		startTime := time.Now()
		output, err := runCommand(projectRoot, cmd, timeoutSec)
		duration := time.Since(startTime)

		if err != nil {
			if logger != nil {
				logger.VerifyCmdEnd(cmd, false, output, duration.Nanoseconds())
			}
			result.passed = false
			result.reason = fmt.Sprintf("%s failed: %v\n\n--- Output (last 50 lines) ---\n%s", cmd, err, output)
			return result
		}

		allOutput = append(allOutput, fmt.Sprintf("$ %s\n%s", cmd, output))

		if logger != nil {
			logger.VerifyCmdEnd(cmd, true, output, duration.Nanoseconds())
			if logger.config != nil && logger.config.ConsoleDurations {
				logger.LogPrint("    ✓ (%s)\n", FormatDuration(duration))
			}
		}
	}

	result.output = strings.Join(allOutput, "\n\n")
	return result
}

// landParseAnalysis extracts VERIFY_PASS/VERIFY_FAIL markers from analysis output.
// Multiple VERIFY_FAIL markers may be present. Failures override pass.
func landParseAnalysis(result *ProviderResult) (passed bool, failures []string) {
	if result == nil || result.Output == "" {
		return false, []string{"analysis produced no output"}
	}

	for _, line := range strings.Split(result.Output, "\n") {
		trimmed := strings.TrimSpace(line)
		if scripVerifyPassRe.MatchString(trimmed) {
			passed = true
		}
		if m := scripVerifyFailRe.FindStringSubmatch(trimmed); len(m) == 2 {
			failures = append(failures, strings.TrimSpace(m[1]))
		}
	}

	// Failures override pass
	if len(failures) > 0 {
		passed = false
	}

	// Non-empty output with no markers at all — synthetic failure
	if !passed && len(failures) == 0 && result.Output != "" {
		failures = append(failures, "Analysis produced output but no VERIFY_PASS/VERIFY_FAIL markers — provider may have truncated or failed to follow instructions")
	}

	return passed, failures
}
