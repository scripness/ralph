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
	// Default should be ~/.scrip/resources
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

func TestGetCachedResources_Empty(t *testing.T) {
	rm := NewResourceManager(nil, nil, "npm", t.TempDir())
	cached := rm.GetCachedResources()
	if len(cached) != 0 {
		t.Errorf("expected 0 cached resources, got %d", len(cached))
	}
}

func TestEnsureResources_VersionedRepoEmptyBranch_SkipsSync(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	os.MkdirAll(cacheDir, 0755)

	rm := &ResourceManager{
		cacheDir: cacheDir,
		detected: map[string]*Resource{
			"pino-pretty@13.0.0": {
				Name:    "pino-pretty",
				URL:     "https://github.com/pinojs/pino-pretty",
				Branch:  "", // empty — resolved via URL cache, no tag info
				Version: "13.0.0",
			},
		},
	}

	// Create a fake cached repo so Exists() returns true
	repoPath := filepath.Join(cacheDir, "pino-pretty@13.0.0")
	os.MkdirAll(filepath.Join(repoPath, ".git"), 0755)

	// Load a minimal registry
	reg, _ := LoadResourceRegistry(cacheDir)
	rm.registry = reg

	// EnsureResources should skip sync (no IsUpToDate/Pull calls)
	// because Version is set — content is immutable
	err := rm.EnsureResources()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// If it tried to sync, it would fail with git errors on the fake repo.
	// Success means it correctly skipped.
}

func TestDefaultScripResourcesCacheDir(t *testing.T) {
	dir := DefaultScripResourcesCacheDir()
	if dir == "" {
		t.Fatal("expected non-empty scrip cache dir")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback path when home is unavailable
		if dir != ".scrip/resources" {
			t.Errorf("expected fallback '.scrip/resources', got '%s'", dir)
		}
		return
	}
	expected := filepath.Join(home, ".scrip", "resources")
	if dir != expected {
		t.Errorf("expected '%s', got '%s'", expected, dir)
	}
}

func TestEnsureScripResourceSync_NoDeps(t *testing.T) {
	cfg := &ScripResolvedConfig{
		ProjectRoot: t.TempDir(),
		Config: ScripConfig{
			Project: ProjectConfig{Name: "test", Type: "go"},
		},
	}
	codebaseCtx := &CodebaseContext{
		TechStack:    "go",
		Dependencies: []Dependency{},
	}
	rm := ensureScripResourceSync(cfg, codebaseCtx)
	if rm == nil {
		t.Error("expected non-nil ResourceManager")
	}
	if rm.HasDetectedResources() {
		t.Error("expected no detected resources")
	}
}

func TestEnsureScripResourceSync_UsesScripCacheDir(t *testing.T) {
	cfg := &ScripResolvedConfig{
		ProjectRoot: t.TempDir(),
		Config: ScripConfig{
			Project: ProjectConfig{Name: "test", Type: "node"},
		},
	}
	codebaseCtx := &CodebaseContext{
		TechStack:    "typescript",
		Dependencies: nil,
	}
	rm := ensureScripResourceSync(cfg, codebaseCtx)
	if rm == nil {
		t.Fatal("expected non-nil ResourceManager")
	}
	expected := DefaultScripResourcesCacheDir()
	if rm.GetCacheDir() != expected {
		t.Errorf("expected cache dir '%s', got '%s'", expected, rm.GetCacheDir())
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
