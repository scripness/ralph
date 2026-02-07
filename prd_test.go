package main

import (
	"strings"
	"testing"
)

func TestPrdEditManual_EditorNotFound(t *testing.T) {
	t.Setenv("EDITOR", "nonexistent-editor-xyz123")

	err := prdEditManual("/dev/null")
	if err == nil {
		t.Fatal("expected error when editor is not found")
	}
	if !strings.Contains(err.Error(), "not found in PATH") {
		t.Errorf("expected 'not found in PATH' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "nonexistent-editor-xyz123") {
		t.Errorf("expected editor name in error, got: %v", err)
	}
}
