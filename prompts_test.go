package main

import (
	"fmt"
	"testing"
)

func TestGetPrompt_NotFound(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for non-existent prompt")
		}
	}()

	getPrompt("nonexistent", nil)
}

func TestBuildLearnings_Empty(t *testing.T) {
	result := buildLearnings(nil, "## Learnings")
	if result != "" {
		t.Errorf("expected empty string for nil learnings, got %q", result)
	}
}

func TestBuildLearnings_WithItems(t *testing.T) {
	learnings := []string{"first", "second", "third"}
	result := buildLearnings(learnings, "## Learnings from Previous Work")
	if result == "" {
		t.Error("expected non-empty output")
	}
	if len(result) < 10 {
		t.Error("expected heading and items in output")
	}
	// Check heading
	if result[:len("## Learnings from Previous Work")] != "## Learnings from Previous Work" {
		t.Error("expected heading in output")
	}
	// Check items present
	for _, l := range learnings {
		if !contains(result, "- "+l) {
			t.Errorf("expected learning %q in output", l)
		}
	}
	if contains(result, "showing") {
		t.Error("should not show truncation notice for small list")
	}
}

func TestBuildLearnings_Capped(t *testing.T) {
	// Create more than maxLearningsInPrompt learnings
	learnings := make([]string, maxLearningsInPrompt+10)
	for i := range learnings {
		learnings[i] = fmt.Sprintf("learning %d", i)
	}

	result := buildLearnings(learnings, "## Learnings")
	if !contains(result, "showing") {
		t.Error("expected truncation notice for large list")
	}
	// Should contain the last one but not the first
	if !contains(result, fmt.Sprintf("learning %d", maxLearningsInPrompt+9)) {
		t.Error("expected most recent learning present")
	}
	if contains(result, "- learning 0\n") {
		t.Error("expected oldest learning to be truncated")
	}
}

// contains is a helper to avoid importing strings just for this.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
