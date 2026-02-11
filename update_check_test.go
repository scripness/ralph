package main

import (
	"strings"
	"testing"
)

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		latest  string
		current string
		want    bool
	}{
		{"2.0.8", "2.0.6", true},
		{"2.0.6", "2.0.8", false},
		{"2.0.8", "2.0.8", false},
		{"2.1.0", "2.0.9", true},
		{"3.0.0", "2.9.9", true},
		{"v2.0.8", "2.0.6", true},
		{"2.0.8", "v2.0.6", true},
		{"v2.0.8", "v2.0.8", false},
		{"2.0.10", "2.0.9", true},
		{"2.0.9", "2.0.10", false},
	}

	for _, tt := range tests {
		t.Run(tt.latest+"_vs_"+tt.current, func(t *testing.T) {
			got := isNewerVersion(tt.latest, tt.current)
			if got != tt.want {
				t.Errorf("isNewerVersion(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
			}
		})
	}
}

func TestUpdateCheckCachePath(t *testing.T) {
	path := updateCheckCachePath()
	if !strings.HasSuffix(path, "ralph/update-check.json") && !strings.HasSuffix(path, "ralph-update-check.json") {
		t.Errorf("expected path ending with ralph/update-check.json or ralph-update-check.json, got '%s'", path)
	}
}
