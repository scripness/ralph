package main

import (
	"strings"
	"testing"
)

func TestUpdateCheckCachePath(t *testing.T) {
	path := updateCheckCachePath()
	if !strings.HasSuffix(path, "ralph/update-check.json") && !strings.HasSuffix(path, "ralph-update-check.json") {
		t.Errorf("expected path ending with ralph/update-check.json or ralph-update-check.json, got '%s'", path)
	}
}
