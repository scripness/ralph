package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// BrowserRunner handles browser-based verification
type BrowserRunner struct {
	config        *BrowserConfig
	projectRoot   string
	ctx           context.Context
	cancel        context.CancelFunc
	consoleErrors []string
}

// BrowserCheckResult contains results from browser verification
type BrowserCheckResult struct {
	URL           string
	StatusCode    int
	ConsoleErrors []string
	Screenshot    string
	Error         error
	StepResults   []StepResult
}

// StepResult contains the result of a single browser step
type StepResult struct {
	Step    BrowserStep
	Passed  bool
	Error   string
	Elapsed time.Duration
}

// NewBrowserRunner creates a new browser runner
func NewBrowserRunner(projectRoot string, config *BrowserConfig) *BrowserRunner {
	return &BrowserRunner{
		config:      config,
		projectRoot: projectRoot,
	}
}

// RunSteps runs interactive browser steps for a story
func (br *BrowserRunner) RunSteps(story *UserStory, baseURL string) (*BrowserCheckResult, error) {
	if br.config == nil || !br.config.Enabled {
		return nil, nil
	}

	// If no browser steps defined, fall back to URL checks
	if len(story.BrowserSteps) == 0 {
		results, err := br.RunChecks(story, baseURL)
		if err != nil {
			return nil, err
		}
		if len(results) > 0 {
			return &results[0], nil
		}
		return nil, nil
	}

	// Initialize browser
	if err := br.init(); err != nil {
		return nil, fmt.Errorf("failed to initialize browser: %w", err)
	}
	defer br.close()

	result := &BrowserCheckResult{
		URL: baseURL,
	}

	// Execute each step
	for _, step := range story.BrowserSteps {
		stepResult := br.executeStep(step, baseURL, story.ID)
		result.StepResults = append(result.StepResults, stepResult)

		if !stepResult.Passed {
			result.Error = fmt.Errorf("step %s failed: %s", step.Action, stepResult.Error)
			break
		}
	}

	result.ConsoleErrors = br.consoleErrors

	return result, nil
}

// executeStep executes a single browser step
func (br *BrowserRunner) executeStep(step BrowserStep, baseURL, storyID string) StepResult {
	start := time.Now()
	result := StepResult{Step: step, Passed: true}

	timeout := time.Duration(step.Timeout) * time.Second
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	ctx, cancel := context.WithTimeout(br.ctx, timeout)
	defer cancel()

	var err error

	switch step.Action {
	case "navigate":
		url := step.URL
		if !strings.HasPrefix(url, "http") {
			url = baseURL + step.URL
		}
		err = chromedp.Run(ctx,
			chromedp.Navigate(url),
			chromedp.WaitReady("body", chromedp.ByQuery),
		)

	case "click":
		err = chromedp.Run(ctx,
			chromedp.WaitVisible(step.Selector, chromedp.ByQuery),
			chromedp.Click(step.Selector, chromedp.ByQuery),
		)

	case "type":
		err = chromedp.Run(ctx,
			chromedp.WaitVisible(step.Selector, chromedp.ByQuery),
			chromedp.Clear(step.Selector, chromedp.ByQuery),
			chromedp.SendKeys(step.Selector, step.Value, chromedp.ByQuery),
		)

	case "waitFor":
		err = chromedp.Run(ctx,
			chromedp.WaitVisible(step.Selector, chromedp.ByQuery),
		)

	case "assertVisible":
		var nodes []*cdp.Node
		err = chromedp.Run(ctx,
			chromedp.Nodes(step.Selector, &nodes, chromedp.ByQuery),
		)
		if err == nil && len(nodes) == 0 {
			err = fmt.Errorf("element not found: %s", step.Selector)
		}

	case "assertText":
		var text string
		err = chromedp.Run(ctx,
			chromedp.WaitVisible(step.Selector, chromedp.ByQuery),
			chromedp.Text(step.Selector, &text, chromedp.ByQuery),
		)
		if err == nil && !strings.Contains(text, step.Contains) {
			err = fmt.Errorf("text '%s' not found in element (got: '%s')", step.Contains, truncateText(text, 100))
		}

	case "assertNotVisible":
		var nodes []*cdp.Node
		err = chromedp.Run(ctx,
			chromedp.Nodes(step.Selector, &nodes, chromedp.ByQuery, chromedp.AtLeast(0)),
		)
		if err == nil && len(nodes) > 0 {
			// Check if actually visible
			var visible bool
			chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(
				`document.querySelector(%q).offsetParent !== null`, step.Selector), &visible))
			if visible {
				err = fmt.Errorf("element should not be visible: %s", step.Selector)
			}
		}

	case "screenshot":
		var buf []byte
		err = chromedp.Run(ctx,
			chromedp.FullScreenshot(&buf, 90),
		)
		if err == nil && len(buf) > 0 {
			br.saveScreenshot(storyID, step.Selector, buf)
		}

	case "wait":
		waitTime := time.Duration(step.Timeout) * time.Second
		if waitTime == 0 {
			waitTime = 1 * time.Second
		}
		time.Sleep(waitTime)

	case "submit":
		// Click submit and wait for navigation
		err = chromedp.Run(ctx,
			chromedp.WaitVisible(step.Selector, chromedp.ByQuery),
			chromedp.Click(step.Selector, chromedp.ByQuery),
			chromedp.WaitReady("body", chromedp.ByQuery),
		)

	default:
		err = fmt.Errorf("unknown action: %s", step.Action)
	}

	result.Elapsed = time.Since(start)

	if err != nil {
		result.Passed = false
		result.Error = err.Error()
	}

	return result
}

// RunChecks runs browser checks for a story's acceptance criteria (legacy/fallback)
func (br *BrowserRunner) RunChecks(story *UserStory, baseURL string) ([]BrowserCheckResult, error) {
	if br.config == nil || !br.config.Enabled {
		return nil, nil
	}

	// Extract URLs from acceptance criteria
	urls := br.extractURLs(story, baseURL)
	if len(urls) == 0 {
		return nil, nil
	}

	// Initialize browser
	if err := br.init(); err != nil {
		return nil, fmt.Errorf("failed to initialize browser: %w", err)
	}
	defer br.close()

	var results []BrowserCheckResult
	for _, url := range urls {
		result := br.checkURL(url, story.ID)
		results = append(results, result)
	}

	return results, nil
}

// init initializes the browser context
func (br *BrowserRunner) init() error {
	opts := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.DisableGPU,
	}

	if br.config.Headless {
		opts = append(opts, chromedp.Headless)
	}

	if br.config.ExecutablePath != "" {
		opts = append(opts, chromedp.ExecPath(br.config.ExecutablePath))
	}

	// Create allocator context
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)

	// Create browser context
	ctx, cancel := chromedp.NewContext(allocCtx)

	// Store for cleanup
	br.ctx = ctx
	br.cancel = func() {
		cancel()
		allocCancel()
	}

	// Listen for console errors
	br.consoleErrors = nil
	chromedp.ListenTarget(br.ctx, func(ev interface{}) {
		if ev, ok := ev.(*runtime.EventExceptionThrown); ok {
			br.consoleErrors = append(br.consoleErrors, ev.ExceptionDetails.Text)
		}
	})

	return nil
}

// close closes the browser
func (br *BrowserRunner) close() {
	if br.cancel != nil {
		br.cancel()
	}
}

// checkURL checks a single URL
func (br *BrowserRunner) checkURL(url, storyID string) BrowserCheckResult {
	result := BrowserCheckResult{URL: url}

	// Create timeout context
	ctx, cancel := context.WithTimeout(br.ctx, 30*time.Second)
	defer cancel()

	// Navigate and capture screenshot
	var buf []byte
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(1*time.Second), // Wait for any async content
		chromedp.FullScreenshot(&buf, 90),
	)

	if err != nil {
		result.Error = err
		return result
	}

	result.ConsoleErrors = br.consoleErrors

	// Save screenshot
	if len(buf) > 0 {
		screenshotPath := br.saveScreenshot(storyID, url, buf)
		result.Screenshot = screenshotPath
	}

	return result
}

// extractURLs extracts URLs to check from acceptance criteria
func (br *BrowserRunner) extractURLs(story *UserStory, baseURL string) []string {
	var urls []string
	seen := make(map[string]bool)

	for _, criterion := range story.AcceptanceCriteria {
		lower := strings.ToLower(criterion)

		// Look for common patterns
		patterns := []string{
			"navigate to ",
			"visit ",
			"go to ",
			"open ",
			"check ",
			"verify ",
			"loads at ",
			"accessible at ",
		}

		for _, pattern := range patterns {
			if idx := strings.Index(lower, pattern); idx != -1 {
				rest := criterion[idx+len(pattern):]
				if path := extractPath(rest); path != "" {
					fullURL := baseURL + path
					if !seen[fullURL] {
						urls = append(urls, fullURL)
						seen[fullURL] = true
					}
				}
			}
		}

		// Also look for explicit URLs
		if strings.Contains(criterion, "http://") || strings.Contains(criterion, "https://") {
			words := strings.Fields(criterion)
			for _, word := range words {
				if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") {
					word = strings.TrimRight(word, ".,;:!?\"')")
					if !seen[word] {
						urls = append(urls, word)
						seen[word] = true
					}
				}
			}
		}
	}

	// If no URLs found, at least check the base URL for UI stories
	if len(urls) == 0 && IsUIStory(story) && baseURL != "" {
		urls = append(urls, baseURL)
	}

	return urls
}

// extractPath extracts a URL path from text
func extractPath(text string) string {
	text = strings.TrimSpace(text)

	if !strings.HasPrefix(text, "/") {
		return ""
	}

	end := strings.IndexAny(text, " \t\n,;:!?\"')")
	if end == -1 {
		end = len(text)
	}

	path := text[:end]
	if len(path) < 2 {
		return ""
	}

	return path
}

// saveScreenshot saves a screenshot to the screenshots directory
func (br *BrowserRunner) saveScreenshot(storyID, identifier string, data []byte) string {
	screenshotDir := br.config.ScreenshotDir
	if screenshotDir == "" {
		screenshotDir = ".ralph/screenshots"
	}

	if !filepath.IsAbs(screenshotDir) {
		screenshotDir = filepath.Join(br.projectRoot, screenshotDir)
	}

	if err := os.MkdirAll(screenshotDir, 0755); err != nil {
		fmt.Printf("Warning: failed to create screenshot dir: %v\n", err)
		return ""
	}

	idSafe := strings.ReplaceAll(identifier, "/", "_")
	idSafe = strings.ReplaceAll(idSafe, ":", "_")
	idSafe = strings.ReplaceAll(idSafe, "?", "_")
	if len(idSafe) > 50 {
		idSafe = idSafe[:50]
	}

	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("%s-%s-%s.png", storyID, timestamp, idSafe)
	screenshotPath := filepath.Join(screenshotDir, filename)

	if err := os.WriteFile(screenshotPath, data, 0644); err != nil {
		fmt.Printf("Warning: failed to save screenshot: %v\n", err)
		return ""
	}

	return screenshotPath
}

// truncateText truncates text to maxLen characters
func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

// BrowserResultsHaveErrors returns true if any check had errors
func BrowserResultsHaveErrors(results []BrowserCheckResult) bool {
	for _, r := range results {
		if r.Error != nil || len(r.ConsoleErrors) > 0 {
			return true
		}
	}
	return false
}

// FormatBrowserResults formats browser check results for display
func FormatBrowserResults(results []BrowserCheckResult) string {
	if len(results) == 0 {
		return "No browser checks performed"
	}

	var sb strings.Builder
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("  URL: %s\n", r.URL))
		if r.Error != nil {
			sb.WriteString(fmt.Sprintf("    ✗ Error: %v\n", r.Error))
		} else {
			sb.WriteString("    ✓ Loaded successfully\n")
		}
		if len(r.ConsoleErrors) > 0 {
			sb.WriteString(fmt.Sprintf("    ⚠ Console errors: %d\n", len(r.ConsoleErrors)))
			for _, err := range r.ConsoleErrors {
				sb.WriteString(fmt.Sprintf("      - %s\n", err))
			}
		}
		if len(r.StepResults) > 0 {
			sb.WriteString("    Steps:\n")
			for _, step := range r.StepResults {
				status := "✓"
				if !step.Passed {
					status = "✗"
				}
				sb.WriteString(fmt.Sprintf("      %s %s", status, step.Step.Action))
				if step.Step.Selector != "" {
					sb.WriteString(fmt.Sprintf(" (%s)", step.Step.Selector))
				}
				sb.WriteString(fmt.Sprintf(" [%v]", step.Elapsed.Round(time.Millisecond)))
				if !step.Passed {
					sb.WriteString(fmt.Sprintf(" - %s", step.Error))
				}
				sb.WriteString("\n")
			}
		}
		if r.Screenshot != "" {
			sb.WriteString(fmt.Sprintf("    📸 Screenshot: %s\n", r.Screenshot))
		}
	}
	return sb.String()
}

// FormatStepResult formats a single step result
func FormatStepResult(result *BrowserCheckResult) string {
	if result == nil {
		return ""
	}

	var sb strings.Builder
	allPassed := true

	for _, step := range result.StepResults {
		status := "✓"
		if !step.Passed {
			status = "✗"
			allPassed = false
		}
		sb.WriteString(fmt.Sprintf("    %s %s", status, step.Step.Action))
		if step.Step.Selector != "" {
			sb.WriteString(fmt.Sprintf(" %s", step.Step.Selector))
		}
		if step.Step.URL != "" {
			sb.WriteString(fmt.Sprintf(" %s", step.Step.URL))
		}
		sb.WriteString(fmt.Sprintf(" (%v)", step.Elapsed.Round(time.Millisecond)))
		if !step.Passed {
			sb.WriteString(fmt.Sprintf("\n      Error: %s", step.Error))
		}
		sb.WriteString("\n")
	}

	if allPassed && len(result.StepResults) > 0 {
		sb.WriteString("    ✓ All browser steps passed\n")
	}

	if len(result.ConsoleErrors) > 0 {
		sb.WriteString(fmt.Sprintf("    ⚠ Console errors: %d\n", len(result.ConsoleErrors)))
	}

	return sb.String()
}

// GetBaseURL extracts base URL from services config
func GetBaseURL(services []ServiceConfig) string {
	for _, svc := range services {
		if svc.Ready != "" {
			return strings.TrimSuffix(svc.Ready, "/")
		}
	}
	return ""
}
