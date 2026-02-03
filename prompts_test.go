package main

import (
	"strings"
	"testing"
)

func TestGetPrompt_Run(t *testing.T) {
	prompt := getPrompt("run", map[string]string{
		"storyId":            "US-001",
		"storyTitle":         "Test Story",
		"storyDescription":   "As a user...",
		"acceptanceCriteria": "- Criterion 1\n- Criterion 2",
		"tags":               "",
		"retryInfo":          "",
		"verifyCommands":     "- bun run test",
		"learnings":          "",
		"knowledgeFile":      "AGENTS.md",
	})

	if !strings.Contains(prompt, "US-001") {
		t.Error("prompt should contain story ID")
	}
	if !strings.Contains(prompt, "Test Story") {
		t.Error("prompt should contain story title")
	}
	if !strings.Contains(prompt, "<ralph>DONE</ralph>") {
		t.Error("prompt should contain DONE marker")
	}
	if !strings.Contains(prompt, "AGENTS.md") {
		t.Error("prompt should contain knowledge file name")
	}
	if !strings.Contains(prompt, "CLI handles") {
		t.Error("prompt should contain responsibility boundaries")
	}
}

func TestGetPrompt_Verify(t *testing.T) {
	prompt := getPrompt("verify", map[string]string{
		"project":        "TestProject",
		"description":    "Test description",
		"storySummaries": "- US-001: Complete",
		"verifyCommands": "- bun run test",
		"learnings":      "",
		"knowledgeFile":  "CLAUDE.md",
	})

	if !strings.Contains(prompt, "TestProject") {
		t.Error("prompt should contain project name")
	}
	if !strings.Contains(prompt, "<ralph>VERIFIED</ralph>") {
		t.Error("prompt should contain VERIFIED marker")
	}
	if !strings.Contains(prompt, "<ralph>RESET:") {
		t.Error("prompt should contain RESET marker example")
	}
	if !strings.Contains(prompt, "CLAUDE.md") {
		t.Error("prompt should contain knowledge file name")
	}
}

func TestGetPrompt_PrdCreate(t *testing.T) {
	prompt := getPrompt("prd-create", map[string]string{
		"feature":     "auth",
		"outputPath":  "/path/to/prd.md",
		"projectRoot": "/project",
	})

	if !strings.Contains(prompt, "auth") {
		t.Error("prompt should contain feature name")
	}
	if !strings.Contains(prompt, "Clarifying Questions") {
		t.Error("prompt should contain clarifying questions section")
	}
	if !strings.Contains(prompt, "Do NOT start implementing") {
		t.Error("prompt should prohibit implementation")
	}
	if !strings.Contains(prompt, "US-XXX") {
		t.Error("prompt should contain story format example")
	}
}

func TestGetPrompt_PrdFinalize(t *testing.T) {
	prompt := getPrompt("prd-finalize", map[string]string{
		"feature":     "auth",
		"prdContent":  "# Auth Feature\n\n## User Stories\n...",
		"outputPath":  "/path/to/prd.json",
		"projectRoot": "/project",
	})

	if !strings.Contains(prompt, "schemaVersion") {
		t.Error("prompt should contain schema version")
	}
	if !strings.Contains(prompt, "browserSteps") {
		t.Error("prompt should document browserSteps")
	}
	if !strings.Contains(prompt, "navigate") {
		t.Error("prompt should list available actions")
	}
}

func TestGetPrompt_PrdRefine(t *testing.T) {
	prompt := getPrompt("prd-refine", map[string]string{
		"feature":     "auth",
		"prdContent":  "# Existing PRD content",
		"outputPath":  "/path/to/prd.md",
		"projectRoot": "/project",
	})

	if !strings.Contains(prompt, "Existing PRD content") {
		t.Error("prompt should contain existing PRD content")
	}
	if !strings.Contains(prompt, "Story Sizing") {
		t.Error("prompt should contain story sizing guidance")
	}
	if !strings.Contains(prompt, "Dependency Order") {
		t.Error("prompt should contain dependency guidance")
	}
}

func TestGetPrompt_ProviderAgnostic(t *testing.T) {
	// Verify prompts don't contain provider-specific references
	prompts := []string{"run", "verify", "prd-create", "prd-finalize", "prd-refine"}
	forbiddenTerms := []string{
		"$AMP_CURRENT_THREAD_ID",
		"read_thread",
		"dev-browser skill",
		"oracle",
		"MCP",
		"skill",
	}

	for _, name := range prompts {
		prompt := getPrompt(name, map[string]string{
			"feature":            "test",
			"storyId":            "US-001",
			"storyTitle":         "Test",
			"storyDescription":   "Test",
			"acceptanceCriteria": "Test",
			"tags":               "",
			"retryInfo":          "",
			"verifyCommands":     "Test",
			"learnings":          "",
			"knowledgeFile":      "AGENTS.md",
			"project":            "Test",
			"description":        "Test",
			"storySummaries":     "Test",
			"prdContent":         "Test",
			"outputPath":         "/test",
			"projectRoot":        "/test",
		})

		for _, term := range forbiddenTerms {
			if strings.Contains(prompt, term) {
				t.Errorf("prompt '%s' contains provider-specific term: %s", name, term)
			}
		}
	}
}

func TestGetPrompt_NotFound(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for non-existent prompt")
		}
	}()

	getPrompt("nonexistent", nil)
}

func TestGenerateRunPrompt(t *testing.T) {
	cfg := &ResolvedConfig{
		ProjectRoot: "/project",
		Config: RalphConfig{
			Provider: ProviderConfig{
				Command:       "amp",
				KnowledgeFile: "AGENTS.md",
			},
			Verify: VerifyConfig{
				Default: []string{"bun run test"},
				UI:      []string{"bun run test:e2e"},
			},
		},
	}

	featureDir := &FeatureDir{
		Feature: "auth",
		Path:    "/project/.ralph/2024-01-15-auth",
	}

	prd := &PRD{
		Run: Run{
			Learnings: []string{"Use bcrypt for passwords"},
		},
	}

	story := &UserStory{
		ID:                 "US-001",
		Title:              "Login form",
		Description:        "As a user, I want to login",
		AcceptanceCriteria: []string{"Form validates", "Token returned"},
		Tags:               []string{"ui"},
		Retries:            1,
		Notes:              "Previous attempt failed",
	}

	prompt := generateRunPrompt(cfg, featureDir, prd, story)

	if !strings.Contains(prompt, "US-001") {
		t.Error("prompt should contain story ID")
	}
	if !strings.Contains(prompt, "Login form") {
		t.Error("prompt should contain story title")
	}
	if !strings.Contains(prompt, "bcrypt") {
		t.Error("prompt should contain learnings")
	}
	if !strings.Contains(prompt, "Previous Attempts") {
		t.Error("prompt should contain retry info")
	}
	if !strings.Contains(prompt, "AGENTS.md") {
		t.Error("prompt should contain knowledge file")
	}
}

func TestGenerateVerifyPrompt(t *testing.T) {
	cfg := &ResolvedConfig{
		ProjectRoot: "/project",
		Config: RalphConfig{
			Provider: ProviderConfig{
				KnowledgeFile: "CLAUDE.md",
			},
			Verify: VerifyConfig{
				Default: []string{"bun run test"},
			},
		},
	}

	featureDir := &FeatureDir{
		Feature: "auth",
		Path:    "/project/.ralph/2024-01-15-auth",
	}

	prd := &PRD{
		Project:     "MyProject",
		Description: "Authentication feature",
		Run: Run{
			Learnings: []string{"Learned something"},
		},
		UserStories: []UserStory{
			{ID: "US-001", Title: "Login", Passes: true},
			{ID: "US-002", Title: "Logout", Passes: true},
		},
	}

	prompt := generateVerifyPrompt(cfg, featureDir, prd)

	if !strings.Contains(prompt, "MyProject") {
		t.Error("prompt should contain project name")
	}
	if !strings.Contains(prompt, "US-001") {
		t.Error("prompt should contain story IDs")
	}
	if !strings.Contains(prompt, "CLAUDE.md") {
		t.Error("prompt should contain knowledge file")
	}
}
