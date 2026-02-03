package main

import (
	"testing"
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

	story := &UserStory{
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

	story := &UserStory{
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

	story := &UserStory{
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

	story := &UserStory{
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

func TestBrowserResultsHaveErrors(t *testing.T) {
	noErrors := []BrowserCheckResult{
		{URL: "http://localhost", Error: nil, ConsoleErrors: nil},
	}
	if BrowserResultsHaveErrors(noErrors) {
		t.Error("expected no errors")
	}

	withError := []BrowserCheckResult{
		{URL: "http://localhost", Error: nil},
		{URL: "http://localhost", Error: nil, ConsoleErrors: []string{"error"}},
	}
	if !BrowserResultsHaveErrors(withError) {
		t.Error("expected errors due to console errors")
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
