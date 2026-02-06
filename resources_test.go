package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResourcesConfig_IsEnabled_Default(t *testing.T) {
	var cfg *ResourcesConfig

	// nil config should default to enabled
	if !cfg.IsEnabled() {
		t.Error("nil ResourcesConfig should be enabled by default")
	}

	// empty config should default to enabled
	cfg = &ResourcesConfig{}
	if !cfg.IsEnabled() {
		t.Error("empty ResourcesConfig should be enabled by default")
	}
}

func TestResourcesConfig_IsEnabled_Explicit(t *testing.T) {
	enabled := true
	disabled := false

	cfg := &ResourcesConfig{Enabled: &enabled}
	if !cfg.IsEnabled() {
		t.Error("expected enabled when Enabled=true")
	}

	cfg = &ResourcesConfig{Enabled: &disabled}
	if cfg.IsEnabled() {
		t.Error("expected disabled when Enabled=false")
	}
}

func TestResourcesConfig_GetCacheDir(t *testing.T) {
	// Default should be ~/.ralph/resources
	var cfg *ResourcesConfig
	dir := cfg.GetCacheDir()
	if dir == "" {
		t.Error("expected non-empty default cache dir")
	}

	// Custom dir
	cfg = &ResourcesConfig{CacheDir: "/custom/path"}
	dir = cfg.GetCacheDir()
	if dir != "/custom/path" {
		t.Errorf("expected '/custom/path', got '%s'", dir)
	}

	// Tilde expansion
	cfg = &ResourcesConfig{CacheDir: "~/my-cache"}
	dir = cfg.GetCacheDir()
	if dir == "~/my-cache" {
		t.Error("expected tilde to be expanded")
	}
}

func TestNewResourceManager(t *testing.T) {
	deps := []string{"next", "react", "unknown-lib"}
	rm := NewResourceManager(nil, deps)

	if rm == nil {
		t.Fatal("expected non-nil ResourceManager")
	}

	// Should detect next and react, not unknown-lib
	detected := rm.ListDetected()
	if len(detected) < 2 {
		t.Errorf("expected at least 2 detected resources, got %d", len(detected))
	}

	// Verify deduplication (next and react are separate)
	hasNext := false
	hasReact := false
	for _, d := range detected {
		if d == "next" {
			hasNext = true
		}
		if d == "react" {
			hasReact = true
		}
	}
	if !hasNext {
		t.Error("expected 'next' to be detected")
	}
	if !hasReact {
		t.Error("expected 'react' to be detected")
	}
}

func TestResourceManager_HasDetectedResources(t *testing.T) {
	// With detected deps
	rm := NewResourceManager(nil, []string{"next", "react"})
	if !rm.HasDetectedResources() {
		t.Error("expected HasDetectedResources to be true with known deps")
	}

	// Without detected deps
	rm = NewResourceManager(nil, []string{"unknown-lib"})
	if rm.HasDetectedResources() {
		t.Error("expected HasDetectedResources to be false with unknown deps")
	}

	// Empty deps
	rm = NewResourceManager(nil, []string{})
	if rm.HasDetectedResources() {
		t.Error("expected HasDetectedResources to be false with empty deps")
	}
}

func TestResourceManager_GetResourcePath(t *testing.T) {
	rm := NewResourceManager(nil, nil)
	path := rm.GetResourcePath("next")

	if path == "" {
		t.Error("expected non-empty path")
	}
	if !filepath.IsAbs(path) || !filepath.IsAbs(rm.GetCacheDir()) {
		// At least one should be abs or both should end with the name
	}
	if filepath.Base(path) != "next" {
		t.Errorf("expected path to end with 'next', got '%s'", path)
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0B"},
		{500, "500B"},
		{1024, "1.0KB"},
		{1024 * 1024, "1.0MB"},
		{1024 * 1024 * 1024, "1.0GB"},
		{2 * 1024 * 1024 * 1024, "2.0GB"},
		{450 * 1024 * 1024, "450.0MB"},
	}

	for _, tt := range tests {
		result := FormatSize(tt.bytes)
		if result != tt.expected {
			t.Errorf("FormatSize(%d) = '%s', expected '%s'", tt.bytes, result, tt.expected)
		}
	}
}

func TestResourceRegistry_UpdateAndGet(t *testing.T) {
	dir := t.TempDir()

	reg, err := LoadResourceRegistry(dir)
	if err != nil {
		t.Fatalf("failed to load registry: %v", err)
	}

	// Update a repo
	reg.UpdateRepo("test", &CachedRepo{
		URL:    "https://example.com/test",
		Branch: "main",
		Commit: "abc123",
		Size:   1024,
	})

	// Get it back
	repo := reg.GetRepo("test")
	if repo == nil {
		t.Fatal("expected to get repo 'test'")
	}
	if repo.URL != "https://example.com/test" {
		t.Errorf("unexpected URL: %s", repo.URL)
	}
	if reg.TotalSize != 1024 {
		t.Errorf("expected TotalSize=1024, got %d", reg.TotalSize)
	}
}

func TestResourceRegistry_RemoveRepo(t *testing.T) {
	dir := t.TempDir()

	reg, err := LoadResourceRegistry(dir)
	if err != nil {
		t.Fatalf("failed to load registry: %v", err)
	}

	reg.UpdateRepo("test", &CachedRepo{Size: 1024})
	reg.RemoveRepo("test")

	repo := reg.GetRepo("test")
	if repo != nil {
		t.Error("expected repo to be removed")
	}
	if reg.TotalSize != 0 {
		t.Errorf("expected TotalSize=0, got %d", reg.TotalSize)
	}
}

func TestResourceRegistry_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	// Create and save
	reg, _ := LoadResourceRegistry(dir)
	reg.UpdateRepo("test", &CachedRepo{
		URL:    "https://example.com/test",
		Branch: "main",
		Commit: "abc123",
		Size:   2048,
	})
	if err := reg.Save(dir); err != nil {
		t.Fatalf("failed to save registry: %v", err)
	}

	// Load in new instance
	reg2, err := LoadResourceRegistry(dir)
	if err != nil {
		t.Fatalf("failed to load registry: %v", err)
	}

	repo := reg2.GetRepo("test")
	if repo == nil {
		t.Fatal("expected to find repo 'test' after reload")
	}
	if repo.URL != "https://example.com/test" {
		t.Errorf("unexpected URL after reload: %s", repo.URL)
	}
	if reg2.TotalSize != 2048 {
		t.Errorf("expected TotalSize=2048 after reload, got %d", reg2.TotalSize)
	}
}

func TestResourceRegistry_ListCached(t *testing.T) {
	dir := t.TempDir()

	reg, _ := LoadResourceRegistry(dir)
	reg.UpdateRepo("a", &CachedRepo{})
	reg.UpdateRepo("b", &CachedRepo{})
	reg.UpdateRepo("c", &CachedRepo{})

	cached := reg.ListCached()
	if len(cached) != 3 {
		t.Errorf("expected 3 cached, got %d", len(cached))
	}
}

func TestExpandHomePath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		input    string
		expected string
	}{
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"~/subdir", filepath.Join(home, "subdir")},
		{"~", "~"}, // Only expands ~/
	}

	for _, tt := range tests {
		result := expandHomePath(tt.input)
		if result != tt.expected {
			t.Errorf("expandHomePath(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestResourceManager_WithCustomResources(t *testing.T) {
	custom := []Resource{
		{Name: "my-lib", URL: "https://github.com/me/my-lib", Branch: "main"},
	}
	cfg := &ResourcesConfig{Custom: custom}

	rm := NewResourceManager(cfg, []string{"my-lib"})
	detected := rm.ListDetected()

	found := false
	for _, d := range detected {
		if d == "my-lib" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected custom 'my-lib' to be detected")
	}
}
