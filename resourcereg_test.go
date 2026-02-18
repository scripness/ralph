package main

import (
	"testing"
)

func TestMapDependencyToResource_DirectMatch(t *testing.T) {
	resources := DefaultResources

	// Test direct match
	r := MapDependencyToResource("next", resources)
	if r == nil {
		t.Fatal("expected to find 'next' resource")
	}
	if r.Name != "next" {
		t.Errorf("expected name 'next', got '%s'", r.Name)
	}
	if r.URL != "https://github.com/vercel/next.js" {
		t.Errorf("unexpected URL: %s", r.URL)
	}
}

func TestMapDependencyToResource_ScopedPackage(t *testing.T) {
	resources := DefaultResources

	// Test scoped package match
	r := MapDependencyToResource("@sveltejs/kit", resources)
	if r == nil {
		t.Fatal("expected to find '@sveltejs/kit' resource")
	}
	if r.Name != "@sveltejs/kit" {
		t.Errorf("expected name '@sveltejs/kit', got '%s'", r.Name)
	}
}

func TestMapDependencyToResource_RelatedPackage(t *testing.T) {
	resources := DefaultResources

	// Test related package match (react-dom -> react)
	r := MapDependencyToResource("react-dom", resources)
	if r == nil {
		t.Fatal("expected to find resource for 'react-dom'")
	}
	if r.Name != "react" {
		t.Errorf("expected 'react-dom' to map to 'react', got '%s'", r.Name)
	}
}

func TestMapDependencyToResource_NoMatch(t *testing.T) {
	resources := DefaultResources

	// Test no match
	r := MapDependencyToResource("some-unknown-package", resources)
	if r != nil {
		t.Errorf("expected nil for unknown package, got %+v", r)
	}
}

func TestMapDependencyToResource_ScopedToBase(t *testing.T) {
	resources := DefaultResources

	// Test @next/font -> next
	r := MapDependencyToResource("@next/font", resources)
	if r == nil {
		t.Fatal("expected to find resource for '@next/font'")
	}
	if r.Name != "next" {
		t.Errorf("expected '@next/font' to map to 'next', got '%s'", r.Name)
	}
}

func TestMergeWithCustom_Override(t *testing.T) {
	custom := []Resource{
		{Name: "next", URL: "https://custom.url/next", Branch: "custom-branch"},
	}

	merged := MergeWithCustom(custom)

	// Find next in merged
	var next *Resource
	for i := range merged {
		if merged[i].Name == "next" {
			next = &merged[i]
			break
		}
	}

	if next == nil {
		t.Fatal("expected to find 'next' in merged resources")
	}
	if next.URL != "https://custom.url/next" {
		t.Errorf("expected custom URL, got '%s'", next.URL)
	}
	if next.Branch != "custom-branch" {
		t.Errorf("expected custom branch, got '%s'", next.Branch)
	}
}

func TestMergeWithCustom_Add(t *testing.T) {
	custom := []Resource{
		{Name: "my-custom-lib", URL: "https://github.com/me/my-lib", Branch: "main"},
	}

	merged := MergeWithCustom(custom)

	// Find custom lib in merged
	var customLib *Resource
	for i := range merged {
		if merged[i].Name == "my-custom-lib" {
			customLib = &merged[i]
			break
		}
	}

	if customLib == nil {
		t.Fatal("expected to find 'my-custom-lib' in merged resources")
	}
	if customLib.URL != "https://github.com/me/my-lib" {
		t.Errorf("expected custom URL, got '%s'", customLib.URL)
	}

	// Verify defaults are still present
	var next *Resource
	for i := range merged {
		if merged[i].Name == "next" {
			next = &merged[i]
			break
		}
	}
	if next == nil {
		t.Error("expected default 'next' resource to still be present")
	}
}

func TestDefaultResourcesHasExpectedEntries(t *testing.T) {
	expected := []string{"next", "react", "svelte", "vue", "tailwindcss", "prisma", "vitest", "vite", "phoenix", "ecto", "phoenix_live_view"}

	for _, name := range expected {
		found := false
		for _, r := range DefaultResources {
			if r.Name == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected default resource '%s' to exist", name)
		}
	}
}

func TestDefaultResourcesHaveValidFields(t *testing.T) {
	for _, r := range DefaultResources {
		if r.Name == "" {
			t.Error("resource has empty name")
		}
		if r.URL == "" {
			t.Errorf("resource '%s' has empty URL", r.Name)
		}
		if r.Branch == "" {
			t.Errorf("resource '%s' has empty branch", r.Name)
		}
	}
}
