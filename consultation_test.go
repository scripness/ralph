package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestResourceManager creates a ResourceManager with manually populated detected resources.
// Bypasses network resolution for testing.
func newTestResourceManager(cacheDir string, resources map[string]*Resource) *ResourceManager {
	if cacheDir == "" {
		cacheDir = DefaultResourcesCacheDir()
	}
	detected := resources
	if detected == nil {
		detected = make(map[string]*Resource)
	}
	return &ResourceManager{
		cacheDir: cacheDir,
		detected: detected,
	}
}

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

func TestRelevantFrameworks_NameBasedMatching(t *testing.T) {
	// Auto-resolved dep without frameworkKeywords entry.
	// Scoped package "@scope/name" produces 2 variants: ["scope", "name"],
	// so if both appear in story text, score reaches 2.
	cached := []CachedResource{
		{Name: "@sentry/node", Path: "/cache/@sentry/node@8.0.0", Version: "8.0.0"},
	}

	// Story mentioning both "sentry" and "node" → 2 variant hits → qualifies
	story := &StoryDefinition{
		ID:          "US-001",
		Title:       "Add sentry error tracking to node API",
		Description: "Integrate error monitoring",
	}
	result := relevantFrameworks(story, cached, 3)
	found := false
	for _, r := range result {
		if r.Name == "@sentry/node" {
			found = true
		}
	}
	if !found {
		t.Error("expected name-based matching to find '@sentry/node' (both variants 'sentry' and 'node' appear)")
	}

	// Single-word dep without keywords: only 1 variant, so score=1, NOT enough
	cached2 := []CachedResource{
		{Name: "pino", Path: "/cache/pino@8.0.0", Version: "8.0.0"},
	}
	story2 := &StoryDefinition{
		ID:          "US-002",
		Title:       "Add pino logging",
		Description: "Use pino for structured logging",
	}
	result2 := relevantFrameworks(story2, cached2, 3)
	if len(result2) != 0 {
		t.Error("single-word dep without keywords should not qualify on name alone (score=1)")
	}

	// Single-word dep WITH tag match: tag gives 2 points, name adds 1 → qualifies
	cached3 := []CachedResource{
		{Name: "pino", Path: "/cache/pino@8.0.0", Version: "8.0.0"},
	}
	story3 := &StoryDefinition{
		ID:    "US-003",
		Title: "Add pino logging",
		Tags:  []string{"api"}, // "api" tag maps to known frameworks, not pino
	}
	result3 := relevantFrameworks(story3, cached3, 3)
	// pino is not in frameworkTagMap["api"], so no tag points either
	if len(result3) != 0 {
		t.Error("pino should not match api tag")
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
			{Name: "next", Version: "15.0.0", Path: "/cache/next@15.0.0"},
			{Name: "prisma", Path: "/cache/prisma@5.22.0"},
		},
	}

	output := FormatGuidance(result)

	if !strings.Contains(output, "Additional Framework References") {
		t.Error("expected fallback heading")
	}
	if !strings.Contains(output, "**next v15.0.0**") {
		t.Error("expected next with version in fallback")
	}
	if !strings.Contains(output, "/cache/prisma@5.22.0") {
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

func TestFrameworkKeywords_NonEmpty(t *testing.T) {
	for name, keywords := range frameworkKeywords {
		if len(keywords) == 0 {
			t.Errorf("frameworkKeywords[%q] has empty keyword list", name)
		}
	}
}

func TestFrameworkKeywords_NoEmptyStrings(t *testing.T) {
	for name, keywords := range frameworkKeywords {
		for i, kw := range keywords {
			if kw == "" {
				t.Errorf("frameworkKeywords[%q][%d] is empty string", name, i)
			}
		}
	}
}

func TestFrameworkKeywords_NoDuplicates(t *testing.T) {
	for name, keywords := range frameworkKeywords {
		seen := make(map[string]bool)
		for _, kw := range keywords {
			lower := strings.ToLower(kw)
			if seen[lower] {
				t.Errorf("frameworkKeywords[%q] has duplicate keyword %q", name, kw)
			}
			seen[lower] = true
		}
	}
}

func TestFrameworkTagMap_ValidFrameworks(t *testing.T) {
	for tag, frameworks := range frameworkTagMap {
		for _, fw := range frameworks {
			if _, ok := frameworkKeywords[fw]; !ok {
				t.Errorf("frameworkTagMap[%q] references %q which is not in frameworkKeywords", tag, fw)
			}
		}
	}
}

func TestGetPrompt_Consult(t *testing.T) {
	prompt := getPrompt("consult", map[string]string{
		"framework":          "next v15.0.0",
		"frameworkPath":      "/cache/next@15.0.0",
		"storyId":            "US-001",
		"storyTitle":         "Add login form",
		"storyDescription":   "As a user...",
		"acceptanceCriteria": "- Form renders\n- Auth works",
		"techStack":          "typescript",
	})

	if !strings.Contains(prompt, "next v15.0.0") {
		t.Error("prompt should contain framework name with version")
	}
	if !strings.Contains(prompt, "/cache/next@15.0.0") {
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
		"framework":     "react v18.2.0",
		"frameworkPath":  "/cache/react@18.2.0",
		"feature":       "auth",
		"techStack":     "typescript",
	})

	if !strings.Contains(prompt, "react v18.2.0") {
		t.Error("prompt should contain framework name with version")
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

// --- runConsultSubagent subprocess tests ---

func TestRunConsultSubagent_Success(t *testing.T) {
	dir := t.TempDir()
	cfg := &ResolvedConfig{
		ProjectRoot: dir,
		Config: RalphConfig{
			Provider: ProviderConfig{
				Command:    "sh",
				Args:       []string{"-c", `echo '<ralph>GUIDANCE_START</ralph>'; echo 'Use hooks for state management.'; echo ''; echo 'Source: src/hooks.ts'; echo '<ralph>GUIDANCE_END</ralph>'`},
				PromptMode: "arg",
			},
		},
	}
	result, err := runConsultSubagent(cfg, "test prompt", 10*time.Second)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(result, "Use hooks") {
		t.Error("expected guidance content in result")
	}
	if !strings.Contains(result, "Source: src/hooks.ts") {
		t.Error("expected source citation in result")
	}
}

func TestRunConsultSubagent_MissingMarkers(t *testing.T) {
	dir := t.TempDir()
	cfg := &ResolvedConfig{
		ProjectRoot: dir,
		Config: RalphConfig{
			Provider: ProviderConfig{
				Command:    "sh",
				Args:       []string{"-c", `echo 'Some output without markers'`},
				PromptMode: "arg",
			},
		},
	}
	_, err := runConsultSubagent(cfg, "test prompt", 10*time.Second)
	if err == nil {
		t.Fatal("expected error for missing markers")
	}
	if !strings.Contains(err.Error(), "no guidance") {
		t.Errorf("expected 'no guidance' in error, got: %v", err)
	}
}

func TestRunConsultSubagent_MissingSourceCitation(t *testing.T) {
	dir := t.TempDir()
	cfg := &ResolvedConfig{
		ProjectRoot: dir,
		Config: RalphConfig{
			Provider: ProviderConfig{
				Command:    "sh",
				Args:       []string{"-c", `echo '<ralph>GUIDANCE_START</ralph>'; echo 'Use hooks.'; echo '<ralph>GUIDANCE_END</ralph>'`},
				PromptMode: "arg",
			},
		},
	}
	_, err := runConsultSubagent(cfg, "test prompt", 10*time.Second)
	if err == nil {
		t.Fatal("expected error for missing source citation")
	}
	if !strings.Contains(err.Error(), "source citations") {
		t.Errorf("expected 'source citations' in error, got: %v", err)
	}
}

func TestRunConsultSubagent_Timeout(t *testing.T) {
	dir := t.TempDir()
	cfg := &ResolvedConfig{
		ProjectRoot: dir,
		Config: RalphConfig{
			Provider: ProviderConfig{
				Command:    "sh",
				Args:       []string{"-c", "sleep 30"},
				PromptMode: "arg",
			},
		},
	}
	_, err := runConsultSubagent(cfg, "test prompt", 1*time.Second)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected 'timed out' in error, got: %v", err)
	}
}

func TestRunConsultSubagent_StderrMarkers(t *testing.T) {
	dir := t.TempDir()
	// Output markers on stderr instead of stdout
	cfg := &ResolvedConfig{
		ProjectRoot: dir,
		Config: RalphConfig{
			Provider: ProviderConfig{
				Command:    "sh",
				Args:       []string{"-c", `echo '<ralph>GUIDANCE_START</ralph>' >&2; echo 'Guidance via stderr.' >&2; echo 'Source: lib/core.ts' >&2; echo '<ralph>GUIDANCE_END</ralph>' >&2`},
				PromptMode: "arg",
			},
		},
	}
	result, err := runConsultSubagent(cfg, "test prompt", 10*time.Second)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(result, "Guidance via stderr") {
		t.Error("expected guidance from stderr")
	}
}

// --- setupConsultTestResources helper ---

// setupConsultTestResources creates a temp dir with a fake cached resource and registry.
func setupConsultTestResources(t *testing.T, name string) (string, *ResourceManager) {
	t.Helper()
	dir := t.TempDir()

	key := name + "@1.0.0"

	// Create fake cached repo directory using versioned key
	repoDir := filepath.Join(dir, key)
	os.MkdirAll(repoDir, 0755)

	// Create registry with repo metadata
	reg, _ := LoadResourceRegistry(dir)
	reg.UpdateRepo(key, &CachedRepo{
		URL:     "https://github.com/example/" + name,
		Tag:     "v1.0.0",
		Version: "1.0.0",
		Commit:  "abc123",
	})
	reg.Save(dir)

	rm := newTestResourceManager(dir, map[string]*Resource{
		key: {Name: name, URL: "https://github.com/example/" + name, Branch: "v1.0.0", Version: "1.0.0"},
	})

	return dir, rm
}

func TestConsultResources_Integration(t *testing.T) {
	_, rm := setupConsultTestResources(t, "react")
	featurePath := t.TempDir()

	cfg := &ResolvedConfig{
		ProjectRoot: t.TempDir(),
		Config: RalphConfig{
			Provider: ProviderConfig{
				Command:    "sh",
				Args:       []string{"-c", `echo '<ralph>GUIDANCE_START</ralph>'; echo 'Use hooks for state.'; echo ''; echo 'Source: packages/react/src/hooks.ts'; echo '<ralph>GUIDANCE_END</ralph>'`},
				PromptMode: "arg",
			},
		},
	}

	// Story with "ui" tag → relevantFrameworks should match "react"
	story := &StoryDefinition{
		ID:          "US-001",
		Title:       "Add login form",
		Description: "Create a react component for the login form",
		Tags:        []string{"ui"},
	}

	result := ConsultResources(context.Background(), cfg, story, rm, nil, featurePath)

	if len(result.Consultations) != 1 {
		t.Fatalf("expected 1 consultation, got %d", len(result.Consultations))
	}
	if result.Consultations[0].Framework != "react" {
		t.Errorf("expected framework 'react', got '%s'", result.Consultations[0].Framework)
	}
	if !strings.Contains(result.Consultations[0].Guidance, "Use hooks") {
		t.Error("expected guidance content")
	}
	if len(result.FallbackPaths) != 0 {
		t.Errorf("expected 0 fallbacks, got %d", len(result.FallbackPaths))
	}

	// Verify cache was written
	cacheKey := consultCacheKey("US-001", "react", "abc123", story.Description)
	if _, ok := loadCachedConsultation(featurePath, cacheKey); !ok {
		t.Error("expected consultation to be cached")
	}
}

func TestConsultResources_CacheHit(t *testing.T) {
	_, rm := setupConsultTestResources(t, "react")
	featurePath := t.TempDir()

	story := &StoryDefinition{
		ID:          "US-001",
		Title:       "Add login form",
		Description: "Create a react component for the login form",
		Tags:        []string{"ui"},
	}

	// Pre-populate cache with the exact key ConsultResources will compute
	cacheKey := consultCacheKey(story.ID, "react", "abc123", story.Description)
	cachedGuidance := "Cached: Use hooks.\n\nSource: cached/path.ts"
	saveCachedConsultation(featurePath, cacheKey, cachedGuidance)

	// Provider that would fail if called (use "false" command which exits with 1)
	cfg := &ResolvedConfig{
		ProjectRoot: t.TempDir(),
		Config: RalphConfig{
			Provider: ProviderConfig{
				Command:    "false",
				PromptMode: "arg",
			},
		},
	}

	result := ConsultResources(context.Background(), cfg, story, rm, nil, featurePath)

	if len(result.Consultations) != 1 {
		t.Fatalf("expected 1 consultation from cache, got %d", len(result.Consultations))
	}
	if result.Consultations[0].Guidance != cachedGuidance {
		t.Errorf("expected cached guidance, got: %s", result.Consultations[0].Guidance)
	}
	if len(result.FallbackPaths) != 0 {
		t.Errorf("expected 0 fallbacks, got %d", len(result.FallbackPaths))
	}
}

func TestConsultResources_NoRelevantFrameworks(t *testing.T) {
	_, rm := setupConsultTestResources(t, "react")
	featurePath := t.TempDir()

	cfg := &ResolvedConfig{
		ProjectRoot: t.TempDir(),
		Config: RalphConfig{
			Provider: ProviderConfig{Command: "echo", PromptMode: "arg"},
		},
	}

	// Story with no matching tags and no matching keywords
	story := &StoryDefinition{
		ID:          "US-001",
		Title:       "Configure CI pipeline",
		Description: "Set up GitHub Actions",
		Tags:        []string{},
	}

	result := ConsultResources(context.Background(), cfg, story, rm, nil, featurePath)

	if len(result.Consultations) != 0 {
		t.Errorf("expected 0 consultations for irrelevant story, got %d", len(result.Consultations))
	}
	if len(result.FallbackPaths) != 0 {
		t.Errorf("expected 0 fallbacks, got %d", len(result.FallbackPaths))
	}
}

func TestConsultResources_SubagentFailureFallback(t *testing.T) {
	_, rm := setupConsultTestResources(t, "react")
	featurePath := t.TempDir()

	// Provider that outputs nothing (no markers → consultation fails → fallback)
	cfg := &ResolvedConfig{
		ProjectRoot: t.TempDir(),
		Config: RalphConfig{
			Provider: ProviderConfig{
				Command:    "sh",
				Args:       []string{"-c", "echo 'no markers here'"},
				PromptMode: "arg",
			},
		},
	}

	story := &StoryDefinition{
		ID:    "US-001",
		Title: "Add login form",
		Tags:  []string{"ui"},
	}

	result := ConsultResources(context.Background(), cfg, story, rm, nil, featurePath)

	if len(result.Consultations) != 0 {
		t.Errorf("expected 0 consultations (should fail), got %d", len(result.Consultations))
	}
	if len(result.FallbackPaths) != 1 {
		t.Fatalf("expected 1 fallback, got %d", len(result.FallbackPaths))
	}
	if result.FallbackPaths[0].Name != "react" {
		t.Errorf("expected fallback for 'react', got '%s'", result.FallbackPaths[0].Name)
	}
}

// --- ConsultResourcesForFeature integration tests ---

func TestConsultResourcesForFeature_Integration(t *testing.T) {
	_, rm := setupConsultTestResources(t, "react")
	featurePath := t.TempDir()

	cfg := &ResolvedConfig{
		ProjectRoot: t.TempDir(),
		Config: RalphConfig{
			Provider: ProviderConfig{
				Command:    "sh",
				Args:       []string{"-c", `echo '<ralph>GUIDANCE_START</ralph>'; echo 'Feature-level react guidance.'; echo ''; echo 'Source: packages/react/src/index.ts'; echo '<ralph>GUIDANCE_END</ralph>'`},
				PromptMode: "arg",
			},
		},
	}

	result := ConsultResourcesForFeature(context.Background(), cfg, "auth", rm, nil, featurePath)

	if len(result.Consultations) != 1 {
		t.Fatalf("expected 1 consultation, got %d", len(result.Consultations))
	}
	if result.Consultations[0].Framework != "react" {
		t.Errorf("expected framework 'react', got '%s'", result.Consultations[0].Framework)
	}
	if !strings.Contains(result.Consultations[0].Guidance, "Feature-level react guidance") {
		t.Error("expected feature-level guidance content")
	}

	// Verify cache was written
	cacheKey := featureConsultCacheKey("auth", "react", "abc123")
	if _, ok := loadCachedConsultation(featurePath, cacheKey); !ok {
		t.Error("expected feature consultation to be cached")
	}
}

func TestConsultResourcesForFeature_CacheHit(t *testing.T) {
	_, rm := setupConsultTestResources(t, "react")
	featurePath := t.TempDir()

	// Pre-populate cache
	cacheKey := featureConsultCacheKey("auth", "react", "abc123")
	cachedGuidance := "Cached feature guidance.\n\nSource: cached/index.ts"
	saveCachedConsultation(featurePath, cacheKey, cachedGuidance)

	cfg := &ResolvedConfig{
		ProjectRoot: t.TempDir(),
		Config: RalphConfig{
			Provider: ProviderConfig{
				Command:    "false",
				PromptMode: "arg",
			},
		},
	}

	result := ConsultResourcesForFeature(context.Background(), cfg, "auth", rm, nil, featurePath)

	if len(result.Consultations) != 1 {
		t.Fatalf("expected 1 consultation from cache, got %d", len(result.Consultations))
	}
	if result.Consultations[0].Guidance != cachedGuidance {
		t.Errorf("expected cached guidance, got: %s", result.Consultations[0].Guidance)
	}
}

func TestDependencyNameVariants(t *testing.T) {
	tests := []struct {
		name     string
		expected []string
	}{
		{"pino", []string{"pino"}},
		{"@prisma/client", []string{"prisma", "client"}},
		{"react", []string{"react"}},
		{"@types/node", []string{"types", "node"}},
	}
	for _, tt := range tests {
		result := dependencyNameVariants(tt.name)
		if len(result) != len(tt.expected) {
			t.Errorf("dependencyNameVariants(%q) = %v, expected %v", tt.name, result, tt.expected)
			continue
		}
		for i, v := range result {
			if v != tt.expected[i] {
				t.Errorf("dependencyNameVariants(%q)[%d] = %q, expected %q", tt.name, i, v, tt.expected[i])
			}
		}
	}
}
