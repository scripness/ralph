package main

import (
	"testing"
	"time"
)

func TestResourceType_HasFields(t *testing.T) {
	r := Resource{
		Name:    "next",
		URL:     "https://github.com/vercel/next.js",
		Branch:  "v15.0.0",
		Version: "15.0.0",
	}
	if r.Name != "next" {
		t.Errorf("unexpected Name: %s", r.Name)
	}
	if r.Version != "15.0.0" {
		t.Errorf("unexpected Version: %s", r.Version)
	}
}

func TestResourceRegistry_ResolvedURLCache(t *testing.T) {
	dir := t.TempDir()
	reg, err := LoadResourceRegistry(dir)
	if err != nil {
		t.Fatalf("failed to load registry: %v", err)
	}

	// Not cached yet
	if _, ok := reg.GetResolvedURL("zod"); ok {
		t.Error("expected no cached URL for 'zod'")
	}

	// Set and get
	reg.SetResolvedURL("zod", "https://github.com/colinhacks/zod")
	url, ok := reg.GetResolvedURL("zod")
	if !ok {
		t.Fatal("expected cached URL for 'zod'")
	}
	if url != "https://github.com/colinhacks/zod" {
		t.Errorf("unexpected URL: %s", url)
	}

	// Persist and reload
	if err := reg.Save(dir); err != nil {
		t.Fatalf("failed to save: %v", err)
	}
	reg2, err := LoadResourceRegistry(dir)
	if err != nil {
		t.Fatalf("failed to reload: %v", err)
	}
	url2, ok := reg2.GetResolvedURL("zod")
	if !ok || url2 != url {
		t.Errorf("expected persisted URL, got ok=%v url=%s", ok, url2)
	}
}

func TestResourceRegistry_Unresolvable(t *testing.T) {
	dir := t.TempDir()
	reg, _ := LoadResourceRegistry(dir)

	if reg.IsUnresolvable("unknown-pkg") {
		t.Error("should not be unresolvable before marking")
	}

	reg.MarkUnresolvable("unknown-pkg")

	if !reg.IsUnresolvable("unknown-pkg") {
		t.Error("should be unresolvable after marking")
	}

	// Persist and reload
	reg.Save(dir)
	reg2, _ := LoadResourceRegistry(dir)
	if !reg2.IsUnresolvable("unknown-pkg") {
		t.Error("unresolvable status should persist")
	}
}

func TestResourceRegistry_UnresolvableExpiry(t *testing.T) {
	reg := &ResourceRegistry{
		Repos:        make(map[string]*CachedRepo),
		Unresolvable: map[string]time.Time{
			"old-pkg": time.Now().Add(-8 * 24 * time.Hour), // 8 days ago → expired
		},
	}

	if reg.IsUnresolvable("old-pkg") {
		t.Error("expired unresolvable entry should not be considered unresolvable")
	}
}

func TestResourceRegistry_ResolvedURLExpiry(t *testing.T) {
	reg := &ResourceRegistry{
		Repos: make(map[string]*CachedRepo),
		Resolved: map[string]*ResolvedEntry{
			"old-pkg": {
				URL:        "https://github.com/old/repo",
				ResolvedAt: time.Now().Add(-31 * 24 * time.Hour), // 31 days ago → expired
			},
			"fresh-pkg": {
				URL:        "https://github.com/fresh/repo",
				ResolvedAt: time.Now().Add(-1 * 24 * time.Hour), // 1 day ago → fresh
			},
		},
	}

	// Old entry should be expired
	if _, ok := reg.GetResolvedURL("old-pkg"); ok {
		t.Error("expired resolved URL should not be returned")
	}

	// Fresh entry should still be valid
	url, ok := reg.GetResolvedURL("fresh-pkg")
	if !ok || url != "https://github.com/fresh/repo" {
		t.Errorf("fresh resolved URL should be returned, got ok=%v url=%s", ok, url)
	}
}

func TestCachedRepo_VersionField(t *testing.T) {
	dir := t.TempDir()
	reg, _ := LoadResourceRegistry(dir)

	reg.UpdateRepo("zod@3.24.4", &CachedRepo{
		URL:     "https://github.com/colinhacks/zod",
		Tag:     "v3.24.4",
		Version: "3.24.4",
		Commit:  "abc123",
		Size:    1024,
	})

	repo := reg.GetRepo("zod@3.24.4")
	if repo == nil {
		t.Fatal("expected to get repo")
	}
	if repo.Version != "3.24.4" {
		t.Errorf("expected version '3.24.4', got '%s'", repo.Version)
	}
	if repo.Tag != "v3.24.4" {
		t.Errorf("expected tag 'v3.24.4', got '%s'", repo.Tag)
	}

	// Save and reload
	reg.Save(dir)
	reg2, _ := LoadResourceRegistry(dir)
	repo2 := reg2.GetRepo("zod@3.24.4")
	if repo2 == nil {
		t.Fatal("expected to find repo after reload")
	}
	if repo2.Version != "3.24.4" {
		t.Errorf("version not persisted: %s", repo2.Version)
	}
}
