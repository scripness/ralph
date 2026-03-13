package main

import (
	"strings"
	"testing"
)

// newTestResourceManager creates a ResourceManager with manually populated detected resources.
// Bypasses network resolution for testing.
func newTestResourceManager(cacheDir string, resources map[string]*Resource) *ResourceManager {
	if cacheDir == "" {
		cacheDir = DefaultScripResourcesCacheDir()
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
