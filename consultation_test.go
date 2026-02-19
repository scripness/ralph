package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRelevantFrameworks_TagMatching(t *testing.T) {
	cached := []CachedResource{
		{Name: "next", Path: "/cache/next"},
		{Name: "react", Path: "/cache/react"},
		{Name: "prisma", Path: "/cache/prisma"},
		{Name: "express", Path: "/cache/express"},
	}

	// UI tag should match next and react
	story := &StoryDefinition{
		ID:    "US-001",
		Title: "Add login form",
		Tags:  []string{"ui"},
	}
	result := relevantFrameworks(story, cached, 3)

	names := make(map[string]bool)
	for _, r := range result {
		names[r.Name] = true
	}
	if !names["next"] {
		t.Error("expected UI tag to match 'next'")
	}
	if !names["react"] {
		t.Error("expected UI tag to match 'react'")
	}

	// DB tag should match prisma
	story = &StoryDefinition{
		ID:    "US-002",
		Title: "Add user table",
		Tags:  []string{"db"},
	}
	result = relevantFrameworks(story, cached, 3)

	names = make(map[string]bool)
	for _, r := range result {
		names[r.Name] = true
	}
	if !names["prisma"] {
		t.Error("expected DB tag to match 'prisma'")
	}
}

func TestRelevantFrameworks_KeywordMatching(t *testing.T) {
	cached := []CachedResource{
		{Name: "next", Path: "/cache/next"},
		{Name: "prisma", Path: "/cache/prisma"},
		{Name: "express", Path: "/cache/express"},
	}

	// Story with multiple Next.js keywords
	story := &StoryDefinition{
		ID:                 "US-001",
		Title:              "Add server action for form",
		Description:        "Create a server component with app router",
		AcceptanceCriteria: []string{"Page renders correctly"},
	}
	result := relevantFrameworks(story, cached, 3)

	found := false
	for _, r := range result {
		if r.Name == "next" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'server action' + 'server component' + 'app router' to match next")
	}
}

func TestRelevantFrameworks_RequiresTwoHits(t *testing.T) {
	cached := []CachedResource{
		{Name: "prisma", Path: "/cache/prisma"},
	}

	// Story with only 1 keyword hit — should NOT match
	story := &StoryDefinition{
		ID:          "US-001",
		Title:       "Fix the button",
		Description: "Update the button color",
	}
	result := relevantFrameworks(story, cached, 3)
	if len(result) != 0 {
		t.Errorf("expected 0 matches with no relevant keywords, got %d", len(result))
	}
}

func TestRelevantFrameworks_Cap(t *testing.T) {
	cached := []CachedResource{
		{Name: "next", Path: "/cache/next"},
		{Name: "react", Path: "/cache/react"},
		{Name: "prisma", Path: "/cache/prisma"},
		{Name: "express", Path: "/cache/express"},
		{Name: "vitest", Path: "/cache/vitest"},
	}

	// Story with tags that match many frameworks
	story := &StoryDefinition{
		ID:    "US-001",
		Title: "Build the entire stack with prisma model and express middleware and react component using vitest describe",
		Tags:  []string{"ui", "db", "api", "test"},
	}
	result := relevantFrameworks(story, cached, 3)
	if len(result) > 3 {
		t.Errorf("expected max 3 frameworks, got %d", len(result))
	}
}

func TestRelevantFrameworks_NoResources(t *testing.T) {
	story := &StoryDefinition{ID: "US-001", Title: "Test"}

	result := relevantFrameworks(story, nil, 3)
	if result != nil {
		t.Error("expected nil for empty cache")
	}

	result = relevantFrameworks(story, []CachedResource{}, 3)
	if result != nil {
		t.Error("expected nil for empty cache")
	}
}

func TestRelevantFrameworks_NilStory(t *testing.T) {
	cached := []CachedResource{{Name: "next", Path: "/cache/next"}}
	result := relevantFrameworks(nil, cached, 3)
	if result != nil {
		t.Error("expected nil for nil story")
	}
}

func TestFormatGuidance_Success(t *testing.T) {
	result := &ConsultationResult{
		Consultations: []ResourceConsultation{
			{Framework: "next", Guidance: "Use app router for server components.\n\nSource: packages/next/src/server/app-render.tsx"},
			{Framework: "prisma", Guidance: "Use findMany with where clause.\n\nSource: packages/client/src/runtime/core.ts"},
		},
	}

	output := FormatGuidance(result)

	if !strings.Contains(output, "## Framework Implementation Guidance") {
		t.Error("expected guidance heading")
	}
	if !strings.Contains(output, "### next") {
		t.Error("expected next section")
	}
	if !strings.Contains(output, "### prisma") {
		t.Error("expected prisma section")
	}
	if !strings.Contains(output, "app router") {
		t.Error("expected guidance content for next")
	}
	if !strings.Contains(output, "findMany") {
		t.Error("expected guidance content for prisma")
	}
}

func TestFormatGuidance_Failed(t *testing.T) {
	result := &ConsultationResult{
		FallbackPaths: []CachedResource{
			{Name: "next", Path: "/cache/next"},
			{Name: "prisma", Path: "/cache/prisma"},
		},
	}

	output := FormatGuidance(result)

	if !strings.Contains(output, "Additional Framework References") {
		t.Error("expected fallback heading")
	}
	if !strings.Contains(output, "**next**") {
		t.Error("expected next in fallback")
	}
	if !strings.Contains(output, "/cache/prisma") {
		t.Error("expected prisma path in fallback")
	}
}

func TestFormatGuidance_Mixed(t *testing.T) {
	result := &ConsultationResult{
		Consultations: []ResourceConsultation{
			{Framework: "next", Guidance: "Use app router.\n\nSource: src/server.ts"},
		},
		FallbackPaths: []CachedResource{
			{Name: "prisma", Path: "/cache/prisma"},
		},
	}

	output := FormatGuidance(result)

	if !strings.Contains(output, "Framework Implementation Guidance") {
		t.Error("expected guidance heading")
	}
	if !strings.Contains(output, "Additional Framework References") {
		t.Error("expected fallback heading")
	}
}

func TestFormatGuidance_Empty(t *testing.T) {
	result := &ConsultationResult{}
	output := FormatGuidance(result)

	if !strings.Contains(output, "Documentation Verification") {
		t.Error("expected web search fallback for empty result")
	}
	if !strings.Contains(output, "web search") {
		t.Error("expected web search instructions")
	}
}

func TestFormatGuidance_Nil(t *testing.T) {
	output := FormatGuidance(nil)
	if !strings.Contains(output, "Documentation Verification") {
		t.Error("expected web search fallback for nil result")
	}
}

func TestExtractBetweenMarkers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		ok       bool
	}{
		{
			"normal extraction",
			"some output\n<ralph>GUIDANCE_START</ralph>\nUse app router.\nSource: src/file.ts\n<ralph>GUIDANCE_END</ralph>\nmore output",
			"Use app router.\nSource: src/file.ts",
			true,
		},
		{
			"no start marker",
			"some output\nUse app router.\n<ralph>GUIDANCE_END</ralph>",
			"",
			false,
		},
		{
			"no end marker",
			"<ralph>GUIDANCE_START</ralph>\nUse app router.",
			"",
			false,
		},
		{
			"empty content",
			"<ralph>GUIDANCE_START</ralph>\n<ralph>GUIDANCE_END</ralph>",
			"",
			true,
		},
		{
			"no markers at all",
			"some random output",
			"",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := extractBetweenMarkers(tt.input)
			if ok != tt.ok {
				t.Errorf("expected ok=%v, got ok=%v", tt.ok, ok)
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestConsultCacheKey_Deterministic(t *testing.T) {
	key1 := consultCacheKey("US-001", "next", "abc123", "Add login form")
	key2 := consultCacheKey("US-001", "next", "abc123", "Add login form")

	if key1 != key2 {
		t.Error("same inputs should produce same key")
	}

	// Different inputs should produce different keys
	key3 := consultCacheKey("US-002", "next", "abc123", "Add login form")
	if key1 == key3 {
		t.Error("different story ID should produce different key")
	}

	key4 := consultCacheKey("US-001", "react", "abc123", "Add login form")
	if key1 == key4 {
		t.Error("different framework should produce different key")
	}

	key5 := consultCacheKey("US-001", "next", "def456", "Add login form")
	if key1 == key5 {
		t.Error("different commit should produce different key")
	}

	key6 := consultCacheKey("US-001", "next", "abc123", "Different description")
	if key1 == key6 {
		t.Error("different description should produce different key")
	}
}

func TestFeatureConsultCacheKey_Deterministic(t *testing.T) {
	key1 := featureConsultCacheKey("auth", "next", "abc123")
	key2 := featureConsultCacheKey("auth", "next", "abc123")

	if key1 != key2 {
		t.Error("same inputs should produce same key")
	}

	key3 := featureConsultCacheKey("billing", "next", "abc123")
	if key1 == key3 {
		t.Error("different feature should produce different key")
	}
}

func TestConsultationCache_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	cacheKey := "US-001-next-abc123"
	guidance := "Use app router for server components.\n\nSource: src/server.ts"

	saveCachedConsultation(dir, cacheKey, guidance)

	loaded, ok := loadCachedConsultation(dir, cacheKey)
	if !ok {
		t.Fatal("expected to load cached consultation")
	}
	if loaded != guidance {
		t.Errorf("expected %q, got %q", guidance, loaded)
	}

	// Non-existent key
	_, ok = loadCachedConsultation(dir, "nonexistent")
	if ok {
		t.Error("expected false for non-existent key")
	}
}

func TestAllCachedFrameworks_Cap(t *testing.T) {
	cached := []CachedResource{
		{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}, {Name: "e"},
	}

	result := allCachedFrameworks(cached, 3)
	if len(result) != 3 {
		t.Errorf("expected 3, got %d", len(result))
	}

	result = allCachedFrameworks(cached, 10)
	if len(result) != 5 {
		t.Errorf("expected 5 (all), got %d", len(result))
	}
}

func TestBuildResourceFallbackInstructions(t *testing.T) {
	result := buildResourceFallbackInstructions()
	if !strings.Contains(result, "Documentation Verification") {
		t.Error("expected heading")
	}
	if !strings.Contains(result, "web search") {
		t.Error("expected web search instructions")
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
		Config: RalphConfig{},
	}
	codebaseCtx := &CodebaseContext{
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
	rm := NewResourceManager(nil, []string{"next"})
	// next is detected but not cached on disk
	cached := rm.GetCachedResources()
	if len(cached) != 0 {
		t.Errorf("expected 0 cached resources (nothing on disk), got %d", len(cached))
	}
}

func TestGetCachedResources_WithCachedRepo(t *testing.T) {
	dir := t.TempDir()

	// Create a fake cached repo directory
	repoDir := filepath.Join(dir, "next")
	os.MkdirAll(repoDir, 0755)

	// Create a registry with repo metadata
	reg, _ := LoadResourceRegistry(dir)
	reg.UpdateRepo("next", &CachedRepo{
		URL:    "https://github.com/vercel/next.js",
		Branch: "canary",
		Commit: "abc123",
	})
	reg.Save(dir)

	cfg := &ResourcesConfig{CacheDir: dir}
	rm := NewResourceManager(cfg, []string{"next"})

	cached := rm.GetCachedResources()
	if len(cached) != 1 {
		t.Fatalf("expected 1 cached resource, got %d", len(cached))
	}
	if cached[0].Name != "next" {
		t.Errorf("expected name 'next', got '%s'", cached[0].Name)
	}
	if cached[0].Commit != "abc123" {
		t.Errorf("expected commit 'abc123', got '%s'", cached[0].Commit)
	}
	if cached[0].Path != repoDir {
		t.Errorf("expected path '%s', got '%s'", repoDir, cached[0].Path)
	}
}

func TestGetPrompt_Consult(t *testing.T) {
	prompt := getPrompt("consult", map[string]string{
		"framework":          "next",
		"frameworkPath":      "/cache/next",
		"storyId":            "US-001",
		"storyTitle":         "Add login form",
		"storyDescription":   "As a user...",
		"acceptanceCriteria": "- Form renders\n- Auth works",
		"techStack":          "typescript",
	})

	if !strings.Contains(prompt, "next") {
		t.Error("prompt should contain framework name")
	}
	if !strings.Contains(prompt, "/cache/next") {
		t.Error("prompt should contain framework path")
	}
	if !strings.Contains(prompt, "US-001") {
		t.Error("prompt should contain story ID")
	}
	if !strings.Contains(prompt, "GUIDANCE_START") {
		t.Error("prompt should contain GUIDANCE_START marker")
	}
	if !strings.Contains(prompt, "GUIDANCE_END") {
		t.Error("prompt should contain GUIDANCE_END marker")
	}
	if !strings.Contains(prompt, "Source:") {
		t.Error("prompt should mention source citations")
	}
}

func TestGetPrompt_ConsultFeature(t *testing.T) {
	prompt := getPrompt("consult-feature", map[string]string{
		"framework":     "react",
		"frameworkPath":  "/cache/react",
		"feature":       "auth",
		"techStack":     "typescript",
	})

	if !strings.Contains(prompt, "react") {
		t.Error("prompt should contain framework name")
	}
	if !strings.Contains(prompt, "auth") {
		t.Error("prompt should contain feature name")
	}
	if !strings.Contains(prompt, "GUIDANCE_START") {
		t.Error("prompt should contain GUIDANCE_START marker")
	}
}

func TestGetPrompt_PrdCreateWithResourceGuidance(t *testing.T) {
	prompt := getPrompt("prd-create", map[string]string{
		"feature":          "auth",
		"outputPath":       "/path/to/prd.md",
		"codebaseContext":  "",
		"resourceGuidance": "## Framework Implementation Guidance\n\n### next\n\nUse app router.\n",
	})

	if !strings.Contains(prompt, "Framework Implementation Guidance") {
		t.Error("prompt should contain resource guidance")
	}
}

func TestGetPrompt_RefineWithResourceGuidance(t *testing.T) {
	prompt := getPrompt("refine", map[string]string{
		"feature":          "auth",
		"prdMdContent":     "# Auth",
		"prdJsonContent":   "{}",
		"progress":         "0/1",
		"storyDetails":     "",
		"learnings":        "",
		"diffSummary":      "",
		"codebaseContext":  "",
		"verifyCommands":   "",
		"serviceURLs":      "",
		"knowledgeFile":    "CLAUDE.md",
		"branchName":       "ralph/auth",
		"featureDir":       "/test",
		"resourceGuidance": "## Framework Guidance\n\nTest guidance",
	})

	if !strings.Contains(prompt, "Framework Guidance") {
		t.Error("prompt should contain resource guidance")
	}
}

func TestGetPrompt_PrdFinalizeWithResourceGuidance(t *testing.T) {
	prompt := getPrompt("prd-finalize", map[string]string{
		"feature":          "auth",
		"prdContent":       "# Auth PRD",
		"outputPath":       "/path/to/prd.json",
		"resourceGuidance": "## Framework Guidance\n\nNext.js supports server actions.",
	})

	if !strings.Contains(prompt, "Framework Guidance") {
		t.Error("prompt should contain resource guidance")
	}
}
