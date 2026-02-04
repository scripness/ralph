package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// BrowserRunner handles browser-based verification
type BrowserRunner struct {
	config        *BrowserConfig
	projectRoot   string
	browser       *rod.Browser
	page          *rod.Page
	mu            sync.Mutex // protects consoleErrors
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

	br.mu.Lock()
	result.ConsoleErrors = append([]string{}, br.consoleErrors...)
	br.mu.Unlock()

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

	page := br.page.Timeout(timeout)
	var err error

	switch step.Action {
	case "navigate":
		url := step.URL
		if !strings.HasPrefix(url, "http") {
			url = baseURL + step.URL
		}
		err = page.Navigate(url)
		if err == nil {
			err = page.WaitLoad()
		}

	case "click":
		el, findErr := page.Element(step.Selector)
		if findErr != nil {
			err = findErr
		} else {
			err = el.Click(proto.InputMouseButtonLeft, 1)
		}

	case "type":
		el, findErr := page.Element(step.Selector)
		if findErr != nil {
			err = findErr
		} else {
			el.SelectAllText()
			err = el.Input(step.Value)
		}

	case "waitFor":
		_, err = page.Element(step.Selector)

	case "assertVisible":
		elements, findErr := page.Elements(step.Selector)
		if findErr != nil {
			err = findErr
		} else if len(elements) == 0 {
			err = fmt.Errorf("element not found: %s", step.Selector)
		}

	case "assertText":
		el, findErr := page.Element(step.Selector)
		if findErr != nil {
			err = findErr
		} else {
			text, textErr := el.Text()
			if textErr != nil {
				err = textErr
			} else if !strings.Contains(text, step.Contains) {
				err = fmt.Errorf("text '%s' not found in element (got: '%s')", step.Contains, truncateText(text, 100))
			}
		}

	case "assertNotVisible":
		elements, _ := page.Elements(step.Selector)
		if len(elements) > 0 {
			visible, _ := elements[0].Visible()
			if visible {
				err = fmt.Errorf("element should not be visible: %s", step.Selector)
			}
		}

	case "screenshot":
		buf, screenshotErr := page.Screenshot(true, nil)
		if screenshotErr != nil {
			err = screenshotErr
		} else if len(buf) > 0 {
			br.saveScreenshot(storyID, step.Selector, buf)
		}

	case "wait":
		waitTime := time.Duration(step.Timeout) * time.Second
		if waitTime == 0 {
			waitTime = 1 * time.Second
		}
		time.Sleep(waitTime)

	case "submit":
		el, findErr := page.Element(step.Selector)
		if findErr != nil {
			err = findErr
		} else {
			err = el.Click(proto.InputMouseButtonLeft, 1)
			if err == nil {
				err = page.WaitLoad()
			}
		}

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

// init initializes the browser
func (br *BrowserRunner) init() error {
	l := launcher.New()

	if br.config.Headless {
		l = l.Headless(true)
	} else {
		l = l.Headless(false)
	}

	if br.config.ExecutablePath != "" {
		l = l.Bin(br.config.ExecutablePath)
	}

	controlURL, err := l.Launch()
	if err != nil {
		return fmt.Errorf("failed to launch browser: %w", err)
	}

	br.browser = rod.New().ControlURL(controlURL)
	if err := br.browser.Connect(); err != nil {
		return fmt.Errorf("failed to connect to browser: %w", err)
	}

	page, err := br.browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return fmt.Errorf("failed to create page: %w", err)
	}
	br.page = page

	// Listen for console errors (uncaught exceptions)
	br.consoleErrors = nil
	go br.page.EachEvent(func(e *proto.RuntimeExceptionThrown) {
		br.mu.Lock()
		br.consoleErrors = append(br.consoleErrors, e.ExceptionDetails.Text)
		br.mu.Unlock()
	})()

	// Listen for console.error() calls
	go br.page.EachEvent(func(e *proto.RuntimeConsoleAPICalled) {
		if e.Type == proto.RuntimeConsoleAPICalledTypeError {
			msg := formatConsoleArgs(e.Args)
			br.mu.Lock()
			br.consoleErrors = append(br.consoleErrors, msg)
			br.mu.Unlock()
		}
	})()

	return nil
}

// close closes the browser
func (br *BrowserRunner) close() {
	if br.browser != nil {
		br.browser.Close()
	}
}

// checkURL checks a single URL
func (br *BrowserRunner) checkURL(url, storyID string) BrowserCheckResult {
	// Reset console errors so each URL gets a clean slate
	br.mu.Lock()
	br.consoleErrors = nil
	br.mu.Unlock()

	result := BrowserCheckResult{URL: url}

	page := br.page.Timeout(30 * time.Second)

	err := page.Navigate(url)
	if err != nil {
		result.Error = err
		return result
	}

	if err := page.WaitLoad(); err != nil {
		result.Error = err
		return result
	}

	time.Sleep(1 * time.Second) // Wait for any async content

	buf, err := page.Screenshot(true, nil)
	if err != nil {
		result.Error = err
		return result
	}

	br.mu.Lock()
	result.ConsoleErrors = append([]string{}, br.consoleErrors...)
	br.mu.Unlock()

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
			sb.WriteString(fmt.Sprintf("    Screenshot: %s\n", r.Screenshot))
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

// formatConsoleArgs converts RuntimeRemoteObject args to a readable string.
func formatConsoleArgs(args []*proto.RuntimeRemoteObject) string {
	var parts []string
	for _, arg := range args {
		if arg.Value.Raw() != nil {
			parts = append(parts, fmt.Sprintf("%v", arg.Value.Val()))
		} else if arg.Description != "" {
			parts = append(parts, arg.Description)
		}
	}
	if len(parts) == 0 {
		return "console.error()"
	}
	return strings.Join(parts, " ")
}

// browserLogger adapts rod's Logger interface to Ralph-style indented output.
type browserLogger struct{}

func (browserLogger) Println(args ...interface{}) {
	fmt.Print("  ")
	fmt.Println(args...)
}

// EnsureBrowser pre-resolves the browser binary, downloading Chromium if needed.
// Mutates config in-place: sets ExecutablePath on success, sets Enabled=false on failure.
// Skips entirely if browser is disabled, executablePath is already set, or no UI stories exist.
func EnsureBrowser(config *BrowserConfig, prd *PRD) {
	if config == nil || !config.Enabled || config.ExecutablePath != "" {
		return
	}

	// Gate: only download if PRD has UI stories
	hasUI := false
	if prd != nil {
		for i := range prd.UserStories {
			if IsUIStory(&prd.UserStories[i]) {
				hasUI = true
				break
			}
		}
	}
	if !hasUI {
		return
	}

	b := launcher.NewBrowser()

	// Fast path: binary already exists (cheap os.Stat, no Chrome process launch).
	if _, err := os.Stat(b.BinPath()); err == nil {
		config.ExecutablePath = b.BinPath()
		fmt.Println("✓ Browser ready")
		return
	}

	// Slow path: download needed
	fmt.Println("Downloading Chromium for browser verification...")
	b.Logger = browserLogger{}
	binPath, err := b.Get()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ✗ Failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "  Set browser.executablePath in ralph.config.json to use an existing browser.")
		config.Enabled = false
		return
	}

	config.ExecutablePath = binPath
	fmt.Println("✓ Browser ready")
}

// CheckBrowserStatus returns a status string for cmdDoctor (no download, no mutation).
func CheckBrowserStatus(config *BrowserConfig) (status string, ok bool) {
	if config == nil || !config.Enabled {
		return "disabled", true
	}
	if config.ExecutablePath != "" {
		if fileExists(config.ExecutablePath) {
			return config.ExecutablePath, true
		}
		return fmt.Sprintf("not found: %s", config.ExecutablePath), false
	}
	b := launcher.NewBrowser()
	if _, err := os.Stat(b.BinPath()); err == nil {
		return "Chromium available", true
	}
	return "Chromium not yet downloaded (will auto-download on first run)", true
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
