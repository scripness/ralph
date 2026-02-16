package main

import (
	"fmt"
	"sync"
	"testing"

	"github.com/go-rod/rod/lib/proto"
	"github.com/ysmood/gson"
)

func TestExtractPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/dashboard", "/dashboard"},
		{"/auth/login page", "/auth/login"},
		{"/settings should show", "/settings"},
		{"/api/health endpoint", "/api/health"},
		{"no path here", ""},
		{"", ""},
		{"/", ""},
		{"/a", "/a"},
		{"/path?query=1", "/path"}, // Query params get cut off
	}

	for _, tt := range tests {
		result := extractPath(tt.input)
		if result != tt.expected {
			t.Errorf("extractPath(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestExtractURLs(t *testing.T) {
	br := &BrowserRunner{
		config: &BrowserConfig{Enabled: true},
	}

	story := &StoryDefinition{
		ID:    "US-001",
		Title: "Dashboard page",
		Tags:  []string{"ui"},
		AcceptanceCriteria: []string{
			"Navigate to /dashboard",
			"User can visit /settings page",
			"Check /api/health returns 200",
			"Go to /auth/login",
		},
	}

	urls := br.extractURLs(story, "http://localhost:3000")

	expected := []string{
		"http://localhost:3000/dashboard",
		"http://localhost:3000/settings",
		"http://localhost:3000/api/health",
		"http://localhost:3000/auth/login",
	}

	if len(urls) != len(expected) {
		t.Errorf("expected %d URLs, got %d: %v", len(expected), len(urls), urls)
		return
	}

	for i, url := range urls {
		if url != expected[i] {
			t.Errorf("URL %d: expected %q, got %q", i, expected[i], url)
		}
	}
}

func TestExtractURLs_ExplicitURLs(t *testing.T) {
	br := &BrowserRunner{
		config: &BrowserConfig{Enabled: true},
	}

	story := &StoryDefinition{
		ID:    "US-001",
		Title: "External API",
		Tags:  []string{"ui"},
		AcceptanceCriteria: []string{
			"Verify https://api.example.com/health works",
			"Check http://localhost:8080/ready endpoint",
		},
	}

	urls := br.extractURLs(story, "http://localhost:3000")

	if len(urls) < 2 {
		t.Errorf("expected at least 2 URLs, got %d: %v", len(urls), urls)
	}

	// Should contain explicit URLs
	found := make(map[string]bool)
	for _, u := range urls {
		found[u] = true
	}

	if !found["https://api.example.com/health"] {
		t.Error("expected to find https://api.example.com/health")
	}
	if !found["http://localhost:8080/ready"] {
		t.Error("expected to find http://localhost:8080/ready")
	}
}

func TestExtractURLs_FallbackToBaseURL(t *testing.T) {
	br := &BrowserRunner{
		config: &BrowserConfig{Enabled: true},
	}

	story := &StoryDefinition{
		ID:    "US-001",
		Title: "Home page",
		Tags:  []string{"ui"},
		AcceptanceCriteria: []string{
			"Page loads successfully",
			"No console errors",
		},
	}

	urls := br.extractURLs(story, "http://localhost:3000")

	// Should fall back to base URL for UI stories with no explicit paths
	if len(urls) != 1 || urls[0] != "http://localhost:3000" {
		t.Errorf("expected base URL fallback, got: %v", urls)
	}
}

func TestExtractURLs_NonUIStory(t *testing.T) {
	br := &BrowserRunner{
		config: &BrowserConfig{Enabled: true},
	}

	story := &StoryDefinition{
		ID:    "US-001",
		Title: "Backend API",
		Tags:  []string{}, // Not a UI story
		AcceptanceCriteria: []string{
			"API returns 200",
		},
	}

	urls := br.extractURLs(story, "http://localhost:3000")

	// Non-UI story should not get base URL fallback
	if len(urls) != 0 {
		t.Errorf("expected no URLs for non-UI story, got: %v", urls)
	}
}

func TestGetBaseURL(t *testing.T) {
	services := []ServiceConfig{
		{
			Name:  "dev",
			Ready: "http://localhost:3000/",
		},
	}

	url := GetBaseURL(services)
	if url != "http://localhost:3000" {
		t.Errorf("expected http://localhost:3000, got %s", url)
	}
}

func TestGetBaseURL_NoServices(t *testing.T) {
	url := GetBaseURL(nil)
	if url != "" {
		t.Errorf("expected empty string, got %s", url)
	}
}

func TestTruncateText(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc..."},
	}

	for _, tt := range tests {
		result := truncateText(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncateText(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

func TestFormatStepResult(t *testing.T) {
	result := &BrowserCheckResult{
		URL: "http://localhost:3000",
		StepResults: []StepResult{
			{
				Step:    BrowserStep{Action: "navigate", URL: "/login"},
				Passed:  true,
				Elapsed: 100 * 1000000, // 100ms
			},
			{
				Step:    BrowserStep{Action: "click", Selector: "#submit"},
				Passed:  false,
				Error:   "element not found",
				Elapsed: 50 * 1000000,
			},
		},
	}

	output := FormatStepResult(result)

	if output == "" {
		t.Error("expected non-empty output")
	}
	if !containsString(output, "navigate") {
		t.Error("output should contain navigate action")
	}
	if !containsString(output, "click") {
		t.Error("output should contain click action")
	}
	if !containsString(output, "element not found") {
		t.Error("output should contain error message")
	}
}

func TestFormatStepResult_Nil(t *testing.T) {
	output := FormatStepResult(nil)
	if output != "" {
		t.Errorf("expected empty string for nil, got %q", output)
	}
}

func TestFormatStepResult_AllPassed(t *testing.T) {
	result := &BrowserCheckResult{
		URL: "http://localhost:3000",
		StepResults: []StepResult{
			{Step: BrowserStep{Action: "navigate"}, Passed: true},
			{Step: BrowserStep{Action: "click"}, Passed: true},
		},
	}

	output := FormatStepResult(result)

	if !containsString(output, "All browser steps passed") {
		t.Error("output should indicate all steps passed")
	}
}

func TestFormatBrowserResults(t *testing.T) {
	results := []BrowserCheckResult{
		{
			URL:        "http://localhost:3000/dashboard",
			Screenshot: "/path/to/screenshot.png",
		},
		{
			URL:           "http://localhost:3000/broken",
			ConsoleErrors: []string{"TypeError: undefined"},
		},
	}

	output := FormatBrowserResults(results)

	if output == "" {
		t.Error("expected non-empty output")
	}

	// Should contain URLs
	if !containsString(output, "http://localhost:3000/dashboard") {
		t.Error("expected output to contain dashboard URL")
	}
	if !containsString(output, "TypeError: undefined") {
		t.Error("expected output to contain console error")
	}
}

func TestBrowserRunner_ConsoleErrorMutexSafety(t *testing.T) {
	br := &BrowserRunner{
		config: &BrowserConfig{Enabled: true},
	}

	var wg sync.WaitGroup

	// 10 goroutines each writing 100 errors
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				br.mu.Lock()
				br.consoleErrors = append(br.consoleErrors, "error")
				br.mu.Unlock()
			}
		}(i)
	}

	// 10 goroutines each reading
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				br.mu.Lock()
				_ = append([]string{}, br.consoleErrors...)
				br.mu.Unlock()
			}
		}()
	}

	wg.Wait()

	br.mu.Lock()
	count := len(br.consoleErrors)
	br.mu.Unlock()

	if count != 1000 {
		t.Errorf("expected 1000 errors, got %d", count)
	}
}

func TestFormatConsoleArgs_Nil(t *testing.T) {
	result := formatConsoleArgs(nil)
	if result != "console.error()" {
		t.Errorf("expected 'console.error()', got %q", result)
	}
}

func TestFormatConsoleArgs_Empty(t *testing.T) {
	result := formatConsoleArgs([]*proto.RuntimeRemoteObject{})
	if result != "console.error()" {
		t.Errorf("expected 'console.error()', got %q", result)
	}
}

func TestFormatConsoleArgs_StringValue(t *testing.T) {
	args := []*proto.RuntimeRemoteObject{
		{Value: gson.NewFrom(`"hello world"`)},
	}
	result := formatConsoleArgs(args)
	if result != "hello world" {
		t.Errorf("expected 'hello world', got %q", result)
	}
}

func TestFormatConsoleArgs_Description(t *testing.T) {
	args := []*proto.RuntimeRemoteObject{
		{Description: "Error: something broke"},
	}
	result := formatConsoleArgs(args)
	if result != "Error: something broke" {
		t.Errorf("expected 'Error: something broke', got %q", result)
	}
}

func TestFormatConsoleArgs_Mixed(t *testing.T) {
	args := []*proto.RuntimeRemoteObject{
		{Value: gson.NewFrom(`"prefix"`)},
		{Description: "Error object"},
	}
	result := formatConsoleArgs(args)
	if result != "prefix Error object" {
		t.Errorf("expected 'prefix Error object', got %q", result)
	}
}

func TestFormatConsoleArgs_EmptyDescription(t *testing.T) {
	// Arg with neither value nor description should be skipped
	args := []*proto.RuntimeRemoteObject{
		{},
	}
	result := formatConsoleArgs(args)
	if result != "console.error()" {
		t.Errorf("expected 'console.error()', got %q", result)
	}
}

func TestEnsureBrowser_Disabled(t *testing.T) {
	// nil config → no-op
	EnsureBrowser(nil, nil)

	// Enabled=false → no-op
	cfg := &BrowserConfig{Enabled: false}
	EnsureBrowser(cfg, nil)
	if cfg.Enabled {
		t.Error("expected Enabled to remain false")
	}
}

func TestEnsureBrowser_ExplicitPath(t *testing.T) {
	cfg := &BrowserConfig{Enabled: true, ExecutablePath: "/usr/bin/chromium"}
	def := &PRDDefinition{UserStories: []StoryDefinition{{Tags: []string{"ui"}}}}

	EnsureBrowser(cfg, def)

	// Should not overwrite an explicit path
	if cfg.ExecutablePath != "/usr/bin/chromium" {
		t.Errorf("expected path unchanged, got %q", cfg.ExecutablePath)
	}
}

func TestEnsureBrowser_NoUIStories(t *testing.T) {
	cfg := &BrowserConfig{Enabled: true}
	def := &PRDDefinition{UserStories: []StoryDefinition{
		{Tags: []string{"backend"}},
		{Tags: []string{"api"}},
	}}

	EnsureBrowser(cfg, def)

	// No UI stories → ExecutablePath should remain empty (no download attempted)
	if cfg.ExecutablePath != "" {
		t.Errorf("expected empty ExecutablePath for non-UI PRD, got %q", cfg.ExecutablePath)
	}
}

func TestEnsureBrowser_NilPRD(t *testing.T) {
	cfg := &BrowserConfig{Enabled: true}

	EnsureBrowser(cfg, nil)

	// nil PRD → no UI stories → no download
	if cfg.ExecutablePath != "" {
		t.Errorf("expected empty ExecutablePath for nil PRD, got %q", cfg.ExecutablePath)
	}
}

func TestCheckBrowserStatus_Disabled(t *testing.T) {
	status, ok := CheckBrowserStatus(nil)
	if status != "disabled" || !ok {
		t.Errorf("expected (disabled, true), got (%q, %v)", status, ok)
	}

	status, ok = CheckBrowserStatus(&BrowserConfig{Enabled: false})
	if status != "disabled" || !ok {
		t.Errorf("expected (disabled, true), got (%q, %v)", status, ok)
	}
}

func TestCheckBrowserStatus_ExplicitPathMissing(t *testing.T) {
	cfg := &BrowserConfig{Enabled: true, ExecutablePath: "/nonexistent/chromium"}
	status, ok := CheckBrowserStatus(cfg)
	if ok {
		t.Error("expected ok=false for missing executable")
	}
	if status != "not found: /nonexistent/chromium" {
		t.Errorf("unexpected status: %q", status)
	}
}

func TestCheckBrowserStatus_ExplicitPathExists(t *testing.T) {
	// Use a path that definitely exists
	cfg := &BrowserConfig{Enabled: true, ExecutablePath: "/dev/null"}
	status, ok := CheckBrowserStatus(cfg)
	if !ok {
		t.Error("expected ok=true for existing path")
	}
	if status != "/dev/null" {
		t.Errorf("unexpected status: %q", status)
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestAggregateBrowserResults_Empty(t *testing.T) {
	result := aggregateBrowserResults(nil)
	if result != nil {
		t.Error("expected nil for empty results")
	}

	result = aggregateBrowserResults([]BrowserCheckResult{})
	if result != nil {
		t.Error("expected nil for zero-length results")
	}
}

func TestAggregateBrowserResults_SingleResult(t *testing.T) {
	results := []BrowserCheckResult{
		{URL: "http://localhost:3000", ConsoleErrors: []string{"error1"}},
	}
	composite := aggregateBrowserResults(results)
	if composite == nil {
		t.Fatal("expected non-nil result")
	}
	if composite.URL != "http://localhost:3000" {
		t.Errorf("expected URL from first result, got %s", composite.URL)
	}
	if len(composite.ConsoleErrors) != 1 {
		t.Errorf("expected 1 console error, got %d", len(composite.ConsoleErrors))
	}
}

func TestAggregateBrowserResults_MergesConsoleErrors(t *testing.T) {
	results := []BrowserCheckResult{
		{URL: "http://localhost:3000/a", ConsoleErrors: []string{"error1"}},
		{URL: "http://localhost:3000/b", ConsoleErrors: []string{"error2", "error3"}},
	}
	composite := aggregateBrowserResults(results)
	if composite == nil {
		t.Fatal("expected non-nil result")
	}
	if len(composite.ConsoleErrors) != 3 {
		t.Errorf("expected 3 console errors merged, got %d", len(composite.ConsoleErrors))
	}
}

func TestAggregateBrowserResults_PropagatesFirstError(t *testing.T) {
	err1 := fmt.Errorf("first error")
	err2 := fmt.Errorf("second error")
	results := []BrowserCheckResult{
		{URL: "http://localhost:3000/a"},
		{URL: "http://localhost:3000/b", Error: err1},
		{URL: "http://localhost:3000/c", Error: err2},
	}
	composite := aggregateBrowserResults(results)
	if composite == nil {
		t.Fatal("expected non-nil result")
	}
	if composite.Error != err1 {
		t.Errorf("expected first error propagated, got %v", composite.Error)
	}
}
