package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// BrowserRunner handles browser-based verification
type BrowserRunner struct {
	config      *BrowserConfig
	projectRoot string
	ctx         context.Context
	cancel      context.CancelFunc
}

// BrowserCheckResult contains results from browser verification
type BrowserCheckResult struct {
	URL           string
	StatusCode    int
	ConsoleErrors []string
	Screenshot    string
	Error         error
}

// NewBrowserRunner creates a new browser runner
func NewBrowserRunner(projectRoot string, config *BrowserConfig) *BrowserRunner {
	return &BrowserRunner{
		config:      config,
		projectRoot: projectRoot,
	}
}

// RunChecks runs browser checks for a story's acceptance criteria
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

	// Collect console errors
	var consoleErrors []string
	chromedp.ListenTarget(br.ctx, func(ev interface{}) {
		if ev, ok := ev.(*runtime.EventExceptionThrown); ok {
			consoleErrors = append(consoleErrors, ev.ExceptionDetails.Text)
		}
	})

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

	result.ConsoleErrors = consoleErrors

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
		// "Navigate to /dashboard"
		// "Visit the /settings page"
		// "Go to /auth/login"
		// "Check /api/health"
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
				// Extract path (starts with /)
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
			// Extract URL from criterion
			words := strings.Fields(criterion)
			for _, word := range words {
				if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") {
					// Clean up trailing punctuation
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
	
	// Must start with /
	if !strings.HasPrefix(text, "/") {
		return ""
	}

	// Find end of path (space or end of string)
	end := strings.IndexAny(text, " \t\n,;:!?\"')")
	if end == -1 {
		end = len(text)
	}

	path := text[:end]
	
	// Basic validation
	if len(path) < 2 {
		return ""
	}

	return path
}

// saveScreenshot saves a screenshot to the screenshots directory
func (br *BrowserRunner) saveScreenshot(storyID, url string, data []byte) string {
	screenshotDir := br.config.ScreenshotDir
	if screenshotDir == "" {
		screenshotDir = ".ralph/screenshots"
	}

	// Make path absolute if relative
	if !filepath.IsAbs(screenshotDir) {
		screenshotDir = filepath.Join(br.projectRoot, screenshotDir)
	}

	// Ensure directory exists
	if err := os.MkdirAll(screenshotDir, 0755); err != nil {
		fmt.Printf("Warning: failed to create screenshot dir: %v\n", err)
		return ""
	}

	// Generate filename
	urlSafe := strings.ReplaceAll(url, "/", "_")
	urlSafe = strings.ReplaceAll(urlSafe, ":", "_")
	urlSafe = strings.ReplaceAll(urlSafe, "?", "_")
	if len(urlSafe) > 50 {
		urlSafe = urlSafe[:50]
	}
	
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("%s-%s-%s.png", storyID, timestamp, urlSafe)
	filepath := filepath.Join(screenshotDir, filename)

	if err := os.WriteFile(filepath, data, 0644); err != nil {
		fmt.Printf("Warning: failed to save screenshot: %v\n", err)
		return ""
	}

	return filepath
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

// FormatResults formats browser check results for display
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
		if r.Screenshot != "" {
			sb.WriteString(fmt.Sprintf("    📸 Screenshot: %s\n", r.Screenshot))
		}
	}
	return sb.String()
}

// GetBaseURL extracts base URL from services config
func GetBaseURL(services []ServiceConfig) string {
	for _, svc := range services {
		if svc.Ready != "" {
			// Use the ready URL as base
			return strings.TrimSuffix(svc.Ready, "/")
		}
	}
	return ""
}
