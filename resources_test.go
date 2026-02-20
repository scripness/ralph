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

func TestNewResourceManager_NoDeps(t *testing.T) {
	rm := NewResourceManager(nil, nil, "npm", "/tmp/test")
	if rm == nil {
		t.Fatal("expected non-nil ResourceManager")
	}
	if rm.HasDetectedResources() {
		t.Error("expected no detected resources with nil deps")
	}
}

func TestNewResourceManager_EmptyDeps(t *testing.T) {
	rm := NewResourceManager(nil, []Dependency{}, "npm", "/tmp/test")
	if rm == nil {
		t.Fatal("expected non-nil ResourceManager")
	}
	if rm.HasDetectedResources() {
		t.Error("expected no detected resources with empty deps")
	}
}

func TestResourceManager_GetResourcePath(t *testing.T) {
	rm := NewResourceManager(nil, nil, "npm", "/tmp/test")
	path := rm.GetResourcePath("next@15.0.0")

	if path == "" {
		t.Error("expected non-empty path")
	}
	if filepath.Base(path) != "next@15.0.0" {
		t.Errorf("expected path to end with 'next@15.0.0', got '%s'", path)
	}
}

func TestResourceRegistry_UpdateAndGet(t *testing.T) {
	dir := t.TempDir()

	reg, err := LoadResourceRegistry(dir)
	if err != nil {
		t.Fatalf("failed to load registry: %v", err)
	}

	// Update a repo
	reg.UpdateRepo("test@1.0.0", &CachedRepo{
		URL:     "https://example.com/test",
		Tag:     "v1.0.0",
		Version: "1.0.0",
		Commit:  "abc123",
		Size:    1024,
	})

	// Get it back
	repo := reg.GetRepo("test@1.0.0")
	if repo == nil {
		t.Fatal("expected to get repo 'test@1.0.0'")
	}
	if repo.URL != "https://example.com/test" {
		t.Errorf("unexpected URL: %s", repo.URL)
	}
	if reg.TotalSize != 1024 {
		t.Errorf("expected TotalSize=1024, got %d", reg.TotalSize)
	}
}

func TestResourceRegistry_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	// Create and save
	reg, _ := LoadResourceRegistry(dir)
	reg.UpdateRepo("test@2.0.0", &CachedRepo{
		URL:     "https://example.com/test",
		Tag:     "v2.0.0",
		Version: "2.0.0",
		Commit:  "abc123",
		Size:    2048,
	})
	if err := reg.Save(dir); err != nil {
		t.Fatalf("failed to save registry: %v", err)
	}

	// Load in new instance
	reg2, err := LoadResourceRegistry(dir)
	if err != nil {
		t.Fatalf("failed to load registry: %v", err)
	}

	repo := reg2.GetRepo("test@2.0.0")
	if repo == nil {
		t.Fatal("expected to find repo 'test@2.0.0' after reload")
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
	reg.UpdateRepo("a@1.0", &CachedRepo{})
	reg.UpdateRepo("b@2.0", &CachedRepo{})
	reg.UpdateRepo("c@3.0", &CachedRepo{})

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

func TestEnsureResourceSync_Disabled(t *testing.T) {
	disabled := false
	cfg := &ResolvedConfig{
		Config: RalphConfig{
			Resources: &ResourcesConfig{Enabled: &disabled},
		},
	}
	codebaseCtx := &CodebaseContext{}
	rm := ensureResourceSync(cfg, codebaseCtx)
	if rm != nil {
		t.Error("expected nil ResourceManager when resources disabled")
	}
}

func TestEnsureResourceSync_NoDeps(t *testing.T) {
	cfg := &ResolvedConfig{
		ProjectRoot: t.TempDir(),
		Config:      RalphConfig{},
	}
	codebaseCtx := &CodebaseContext{
		TechStack:    "typescript",
		Dependencies: []Dependency{},
	}
	rm := ensureResourceSync(cfg, codebaseCtx)
	// Should return a ResourceManager even with no deps (no sync needed)
	if rm == nil {
		t.Error("expected non-nil ResourceManager")
	}
	if rm.HasDetectedResources() {
		t.Error("expected no detected resources")
	}
}

func TestGetCachedResources_Empty(t *testing.T) {
	rm := NewResourceManager(nil, nil, "npm", t.TempDir())
	cached := rm.GetCachedResources()
	if len(cached) != 0 {
		t.Errorf("expected 0 cached resources, got %d", len(cached))
	}
}

func TestCachedResource_VersionField(t *testing.T) {
	cr := CachedResource{
		Name:    "zod",
		Version: "3.24.4",
		Path:    "/cache/zod@3.24.4",
		URL:     "https://github.com/colinhacks/zod",
		Ref:     "v3.24.4",
	}
	if cr.Version != "3.24.4" {
		t.Errorf("expected version '3.24.4', got '%s'", cr.Version)
	}
	if cr.Ref != "v3.24.4" {
		t.Errorf("expected ref 'v3.24.4', got '%s'", cr.Ref)
	}
}
