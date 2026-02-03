package main

import (
	"testing"
)

func TestProcessLine_DoneMarker(t *testing.T) {
	result := &ProviderResult{}
	processLine("Some output <ralph>DONE</ralph> more text", result)

	if !result.Done {
		t.Error("expected Done=true")
	}
}

func TestProcessLine_VerifiedMarker(t *testing.T) {
	result := &ProviderResult{}
	processLine("Review complete <ralph>VERIFIED</ralph>", result)

	if !result.Verified {
		t.Error("expected Verified=true")
	}
}

func TestProcessLine_LearningMarker(t *testing.T) {
	result := &ProviderResult{}
	processLine("<ralph>LEARNING:Always use escapeHtml for user data</ralph>", result)

	if len(result.Learnings) != 1 {
		t.Fatalf("expected 1 learning, got %d", len(result.Learnings))
	}
	if result.Learnings[0] != "Always use escapeHtml for user data" {
		t.Errorf("unexpected learning: %s", result.Learnings[0])
	}
}

func TestProcessLine_MultipleLearnings(t *testing.T) {
	result := &ProviderResult{}
	processLine("<ralph>LEARNING:First learning</ralph>", result)
	processLine("<ralph>LEARNING:Second learning</ralph>", result)

	if len(result.Learnings) != 2 {
		t.Fatalf("expected 2 learnings, got %d", len(result.Learnings))
	}
}

func TestProcessLine_ResetMarker(t *testing.T) {
	result := &ProviderResult{}
	processLine("<ralph>RESET:US-001,US-003</ralph>", result)

	if len(result.Resets) != 2 {
		t.Fatalf("expected 2 resets, got %d", len(result.Resets))
	}
	if result.Resets[0] != "US-001" {
		t.Errorf("expected first reset='US-001', got '%s'", result.Resets[0])
	}
	if result.Resets[1] != "US-003" {
		t.Errorf("expected second reset='US-003', got '%s'", result.Resets[1])
	}
}

func TestProcessLine_ReasonMarker(t *testing.T) {
	result := &ProviderResult{}
	processLine("<ralph>REASON:Missing test coverage for auth module</ralph>", result)

	if result.Reason != "Missing test coverage for auth module" {
		t.Errorf("unexpected reason: %s", result.Reason)
	}
}

func TestProcessLine_NoMarkers(t *testing.T) {
	result := &ProviderResult{}
	processLine("Regular output without any markers", result)

	if result.Done || result.Verified || len(result.Learnings) > 0 || len(result.Resets) > 0 {
		t.Error("expected no markers to be detected")
	}
}

func TestProcessLine_MarkerWithWhitespace(t *testing.T) {
	result := &ProviderResult{}
	processLine("<ralph>LEARNING:  Trimmed learning  </ralph>", result)

	if len(result.Learnings) != 1 {
		t.Fatalf("expected 1 learning, got %d", len(result.Learnings))
	}
	if result.Learnings[0] != "Trimmed learning" {
		t.Errorf("expected trimmed learning, got '%s'", result.Learnings[0])
	}
}

func TestLearningPattern(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<ralph>LEARNING:test</ralph>", "test"},
		{"<ralph>LEARNING:multi word learning</ralph>", "multi word learning"},
		{"<ralph>LEARNING: spaces around </ralph>", "spaces around"},
	}

	for _, tt := range tests {
		matches := LearningPattern.FindStringSubmatch(tt.input)
		if len(matches) < 2 {
			t.Errorf("expected match for %s", tt.input)
			continue
		}
		// Note: actual trimming happens in processLine
		if matches[1] != tt.expected && matches[1] != " "+tt.expected+" " {
			t.Errorf("for %s: expected '%s', got '%s'", tt.input, tt.expected, matches[1])
		}
	}
}

func TestResetPattern(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<ralph>RESET:US-001</ralph>", "US-001"},
		{"<ralph>RESET:US-001,US-002</ralph>", "US-001,US-002"},
		{"<ralph>RESET:US-001, US-002, US-003</ralph>", "US-001, US-002, US-003"},
	}

	for _, tt := range tests {
		matches := ResetPattern.FindStringSubmatch(tt.input)
		if len(matches) < 2 {
			t.Errorf("expected match for %s", tt.input)
			continue
		}
		if matches[1] != tt.expected {
			t.Errorf("for %s: expected '%s', got '%s'", tt.input, tt.expected, matches[1])
		}
	}
}
