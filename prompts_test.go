package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
		"project":            "TestProject",
		"description":        "Test feature",
		"branchName":         "ralph/test",
		"progress":           "1/3 stories complete",
		"storyMap":           "✓ US-000: Setup\n→ US-001: Test Story [CURRENT]",
		"codebaseContext":    "",
		"diffSummary":        "",
		"resourceVerificationInstructions": "",
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
	if !strings.Contains(prompt, "TestProject") {
		t.Error("prompt should contain project name")
	}
	if !strings.Contains(prompt, "ralph/test") {
		t.Error("prompt should contain branch name")
	}
	if !strings.Contains(prompt, "1/3 stories complete") {
		t.Error("prompt should contain progress")
	}
	if !strings.Contains(prompt, "[CURRENT]") {
		t.Error("prompt should contain story map with CURRENT marker")
	}
	if !strings.Contains(prompt, "git log --oneline") {
		t.Error("prompt should contain git history hint")
	}
}

func TestGetPrompt_PrdCreate(t *testing.T) {
	prompt := getPrompt("prd-create", map[string]string{
		"feature":    "auth",
		"outputPath": "/path/to/prd.md",
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
	if !strings.Contains(prompt, "e2e tests") {
		t.Error("prompt should mention e2e tests for UI stories")
	}
	if !strings.Contains(prompt, "Typecheck passes") {
		t.Error("prompt should enforce typecheck criterion")
	}
}

func TestGetPrompt_PrdFinalize(t *testing.T) {
	prompt := getPrompt("prd-finalize", map[string]string{
		"feature":    "auth",
		"prdContent": "# Auth Feature\n\n## User Stories\n...",
		"outputPath": "/path/to/prd.json",
	})

	if !strings.Contains(prompt, "schemaVersion") {
		t.Error("prompt should contain schema version")
	}
	if !strings.Contains(prompt, "e2e tests") {
		t.Error("prompt should mention e2e tests for UI stories")
	}
	if !strings.Contains(prompt, "Typecheck passes") {
		t.Error("prompt should enforce typecheck in checklist")
	}
	if !strings.Contains(prompt, "Tests pass") {
		t.Error("prompt should enforce tests pass in checklist")
	}
}

func TestGetPrompt_Refine(t *testing.T) {
	prompt := getPrompt("refine", map[string]string{
		"feature":        "auth",
		"prdMdContent":   "# Auth Feature\n\nUser stories...",
		"prdJsonContent": "```json\n{}\n```",
		"progress":       "2/5 stories complete",
		"storyDetails":   "",
		"learnings":      "",
		"diffSummary":    "",
		"codebaseContext": "",
		"verifyCommands": "- bun run test",
		"serviceURLs":    "",
		"knowledgeFile":  "CLAUDE.md",
		"branchName":     "ralph/auth",
		"featureDir":     "/project/.ralph/2024-01-15-auth",
	})

	if !strings.Contains(prompt, "auth") {
		t.Error("prompt should contain feature name")
	}
	if !strings.Contains(prompt, "Auth Feature") {
		t.Error("prompt should contain prd.md content")
	}
	if !strings.Contains(prompt, "2/5 stories complete") {
		t.Error("prompt should contain progress")
	}
	if !strings.Contains(prompt, "ralph/auth") {
		t.Error("prompt should contain branch name")
	}
	if !strings.Contains(prompt, "CLAUDE.md") {
		t.Error("prompt should contain knowledge file")
	}
	if !strings.Contains(prompt, "ralph run") {
		t.Error("prompt should mention ralph run for resuming")
	}
}

func TestGetPrompt_VerifyFix(t *testing.T) {
	prompt := getPrompt("verify-fix", map[string]string{
		"project":                          "TestProject",
		"description":                      "Test description",
		"branchName":                       "ralph/test",
		"progress":                         "3/5 stories complete",
		"storyDetails":                     "",
		"verifyResults":                    "FAIL: bun run test — exit code 1",
		"verifyCommands":                   "- bun run test",
		"learnings":                        "",
		"diffSummary":                      "",
		"serviceURLs":                      "",
		"knowledgeFile":                    "CLAUDE.md",
		"featureDir":                       "/project/.ralph/2024-01-15-auth",
		"resourceVerificationInstructions": "",
	})

	if !strings.Contains(prompt, "TestProject") {
		t.Error("prompt should contain project name")
	}
	if !strings.Contains(prompt, "FAIL: bun run test") {
		t.Error("prompt should contain verify results")
	}
	if !strings.Contains(prompt, "CLAUDE.md") {
		t.Error("prompt should contain knowledge file")
	}
}

func TestGetPrompt_ProviderAgnostic(t *testing.T) {
	// Verify prompts don't contain provider-specific references
	prompts := []string{"run", "prd-create", "prd-finalize", "refine", "verify-fix"}
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
			"branchName":         "ralph/test",
			"progress":           "0/1",
			"storyMap":           "→ US-001: Test [CURRENT]",
			"storySummaries":     "Test",
			"prdContent":         "Test",
			"outputPath":         "/test",
			"prdPath":            "/test/prd.json",
			"storyDetails":       "",
			"serviceURLs":        "",
			"resourceVerificationInstructions": "",
			"diffSummary":        "",
			"verifySummary":      "",
			"verifyResults":      "",
			"prdMdContent":       "Test",
			"prdJsonContent":     "Test",
			"codebaseContext":    "",
			"featureDir":         "/test",
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
			MaxRetries: 3,
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

	def := &PRDDefinition{
		Project:     "MyApp",
		Description: "Authentication feature",
		BranchName:  "ralph/auth",
		UserStories: []StoryDefinition{
			{ID: "US-001", Title: "Login form", Tags: []string{"ui"}, AcceptanceCriteria: []string{"Form works"}},
			{ID: "US-002", Title: "Session handling"},
		},
	}

	state := NewRunState()
	state.Retries = map[string]int{"US-001": 1}
	state.LastFailure = map[string]string{"US-001": "Previous attempt failed"}
	state.Learnings = []string{"Use bcrypt for passwords"}

	story := &def.UserStories[0]

	prompt := generateRunPrompt(cfg, featureDir, def, state, story, "", "")

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
	if !strings.Contains(prompt, "MyApp") {
		t.Error("prompt should contain project name")
	}
	if !strings.Contains(prompt, "Authentication feature") {
		t.Error("prompt should contain feature description")
	}
	if !strings.Contains(prompt, "ralph/auth") {
		t.Error("prompt should contain branch name")
	}
	if !strings.Contains(prompt, "0/2 stories complete") {
		t.Error("prompt should contain progress")
	}
}

func TestGenerateRunPrompt_StoryMap(t *testing.T) {
	cfg := &ResolvedConfig{
		Config: RalphConfig{
			Provider: ProviderConfig{KnowledgeFile: "AGENTS.md"},
			Verify:   VerifyConfig{Default: []string{"echo ok"}},
		},
	}

	featureDir := &FeatureDir{Feature: "test", Path: "/project/.ralph/test"}

	def := &PRDDefinition{
		Project:     "Test",
		Description: "Test feature",
		BranchName:  "ralph/test",
		UserStories: []StoryDefinition{
			{ID: "US-001", Title: "Database setup"},
			{ID: "US-002", Title: "API endpoint", AcceptanceCriteria: []string{"Returns 200"}},
			{ID: "US-003", Title: "UI component"},
			{ID: "US-004", Title: "Broken feature"},
		},
	}

	state := NewRunState()
	state.MarkPassed("US-001")
	state.MarkSkipped("US-004", "Missing dependency")

	story := &def.UserStories[1] // US-002 is current

	prompt := generateRunPrompt(cfg, featureDir, def, state, story, "", "")

	// Completed story
	if !strings.Contains(prompt, "✓ US-001: Database setup") {
		t.Error("prompt should show completed story with checkmark")
	}

	// Current story
	if !strings.Contains(prompt, "→ US-002: API endpoint [CURRENT]") {
		t.Error("prompt should mark current story")
	}

	// Pending story
	if !strings.Contains(prompt, "○ US-003: UI component") {
		t.Error("prompt should show pending story")
	}

	// Skipped story
	if !strings.Contains(prompt, "✗ US-004: Broken feature (skipped: Missing dependency)") {
		t.Error("prompt should show skipped story with reason")
	}

	// Progress
	if !strings.Contains(prompt, "1/4 stories complete (1 skipped)") {
		t.Error("prompt should show progress with skipped count")
	}
}

func TestBuildProgress(t *testing.T) {
	tests := []struct {
		name   string
		def    *PRDDefinition
		state  *RunState
		expect string
	}{
		{
			"no stories",
			&PRDDefinition{UserStories: []StoryDefinition{}},
			NewRunState(),
			"0/0 stories complete",
		},
		{
			"some complete",
			&PRDDefinition{UserStories: []StoryDefinition{
				{ID: "US-001"}, {ID: "US-002"}, {ID: "US-003"},
			}},
			func() *RunState {
				s := NewRunState()
				s.MarkPassed("US-001")
				s.MarkPassed("US-003")
				return s
			}(),
			"2/3 stories complete",
		},
		{
			"with skipped",
			&PRDDefinition{UserStories: []StoryDefinition{
				{ID: "US-001"}, {ID: "US-002"}, {ID: "US-003"},
			}},
			func() *RunState {
				s := NewRunState()
				s.MarkPassed("US-001")
				s.MarkSkipped("US-002", "too hard")
				return s
			}(),
			"1/3 stories complete (1 skipped)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildProgress(tt.def, tt.state)
			if result != tt.expect {
				t.Errorf("expected %q, got %q", tt.expect, result)
			}
		})
	}
}

func TestBuildStoryMap(t *testing.T) {
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-001", Title: "Done"},
			{ID: "US-002", Title: "Current"},
			{ID: "US-003", Title: "Pending"},
			{ID: "US-004", Title: "Stuck"},
		},
	}

	state := NewRunState()
	state.MarkPassed("US-001")
	state.MarkSkipped("US-004", "Need API")

	current := &StoryDefinition{ID: "US-002"}
	result := buildStoryMap(def, state, current)

	if !strings.Contains(result, "✓ US-001: Done") {
		t.Error("should show completed story")
	}
	if !strings.Contains(result, "→ US-002: Current [CURRENT]") {
		t.Error("should mark current story")
	}
	if !strings.Contains(result, "○ US-003: Pending") {
		t.Error("should show pending story")
	}
	if !strings.Contains(result, "✗ US-004: Stuck (skipped: Need API)") {
		t.Error("should show skipped story with reason")
	}
}

func TestBuildLearnings_Empty(t *testing.T) {
	result := buildLearnings(nil, "## Learnings")
	if result != "" {
		t.Errorf("expected empty string for nil learnings, got %q", result)
	}
}

func TestBuildLearnings_WithItems(t *testing.T) {
	learnings := []string{"first", "second", "third"}
	result := buildLearnings(learnings, "## Learnings from Previous Work")
	if !strings.Contains(result, "## Learnings from Previous Work") {
		t.Error("expected heading in output")
	}
	if !strings.Contains(result, "- first") {
		t.Error("expected first learning in output")
	}
	if strings.Contains(result, "showing") {
		t.Error("should not show truncation notice for small list")
	}
}

func TestBuildLearnings_Capped(t *testing.T) {
	// Create more than maxLearningsInPrompt learnings
	learnings := make([]string, maxLearningsInPrompt+10)
	for i := range learnings {
		learnings[i] = fmt.Sprintf("learning %d", i)
	}

	result := buildLearnings(learnings, "## Learnings")
	if !strings.Contains(result, "showing") {
		t.Error("expected truncation notice for large list")
	}
	// Should contain the last one but not the first
	if !strings.Contains(result, fmt.Sprintf("learning %d", maxLearningsInPrompt+9)) {
		t.Error("expected most recent learning present")
	}
	if strings.Contains(result, "- learning 0\n") {
		t.Error("expected oldest learning to be truncated")
	}
}

func TestGetPrompt_RunWithResourcesCache(t *testing.T) {
	resourceInstr := "## Documentation Verification\n\nThe following framework source code is cached locally:\nnext, react\n\n**Available at:** ~/.ralph/resources/<framework>/\n"
	prompt := getPrompt("run", map[string]string{
		"storyId":                           "US-001",
		"storyTitle":                        "Test Story",
		"storyDescription":                  "As a user...",
		"acceptanceCriteria":                "- Criterion 1",
		"tags":                              "",
		"retryInfo":                         "",
		"verifyCommands":                    "- bun run test",
		"learnings":                         "",
		"knowledgeFile":                     "AGENTS.md",
		"project":                           "TestProject",
		"description":                       "Test feature",
		"branchName":                        "ralph/test",
		"progress":                          "0/1",
		"storyMap":                          "→ US-001: Test [CURRENT]",
		"codebaseContext":                   "",
		"diffSummary":                       "",
		"resourceVerificationInstructions":resourceInstr,
	})

	if !strings.Contains(prompt, "Documentation Verification") {
		t.Error("prompt should contain documentation verification section")
	}
	if !strings.Contains(prompt, "source code is cached") {
		t.Error("prompt should contain resource cache instructions")
	}
}

func TestGetPrompt_RunWithoutResourcesCache(t *testing.T) {
	// When no resources cached, use web search fallback
	webSearchInstr := "## Documentation Verification\n\nBefore committing, verify your implementation against current official documentation using web search:\n"
	prompt := getPrompt("run", map[string]string{
		"storyId":                           "US-001",
		"storyTitle":                        "Test Story",
		"storyDescription":                  "As a user...",
		"acceptanceCriteria":                "- Criterion 1",
		"tags":                              "",
		"retryInfo":                         "",
		"verifyCommands":                    "- bun run test",
		"learnings":                         "",
		"knowledgeFile":                     "AGENTS.md",
		"project":                           "TestProject",
		"description":                       "Test feature",
		"branchName":                        "ralph/test",
		"progress":                          "0/1",
		"storyMap":                          "→ US-001: Test [CURRENT]",
		"codebaseContext":                   "",
		"diffSummary":                       "",
		"resourceVerificationInstructions":webSearchInstr,
	})

	if !strings.Contains(prompt, "Documentation Verification") {
		t.Error("prompt should contain documentation verification section")
	}
	if !strings.Contains(prompt, "web search") {
		t.Error("prompt should contain web search fallback instructions")
	}
}

func TestGenerateRefinePrompt(t *testing.T) {
	// Create temp dir with prd.md and prd.json
	dir := t.TempDir()
	featureDir := &FeatureDir{
		Feature:    "auth",
		Path:       dir,
		HasPrdMd:   true,
		HasPrdJson: true,
	}

	prdMdContent := "# Auth Feature\n\nBuild login and logout."
	if err := os.WriteFile(featureDir.PrdMdPath(), []byte(prdMdContent), 0644); err != nil {
		t.Fatal(err)
	}

	def := &PRDDefinition{
		SchemaVersion: 3,
		Project:       "MyApp",
		BranchName:    "ralph/auth",
		Description:   "Auth feature",
		UserStories: []StoryDefinition{
			{ID: "US-001", Title: "Login", AcceptanceCriteria: []string{"Works"}},
			{ID: "US-002", Title: "Logout", AcceptanceCriteria: []string{"Works"}},
			{ID: "US-003", Title: "Register", AcceptanceCriteria: []string{"Works"}},
		},
	}

	state := NewRunState()
	state.MarkPassed("US-001")
	state.MarkSkipped("US-002", "Depends on sessions")
	state.Retries = map[string]int{"US-002": 3, "US-003": 2}
	state.Learnings = []string{"Use bcrypt for passwords"}

	prdJSON, _ := json.Marshal(def)
	if err := os.WriteFile(featureDir.PrdJsonPath(), prdJSON, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &ResolvedConfig{
		ProjectRoot: dir,
		Config: RalphConfig{
			Provider: ProviderConfig{
				Command:       "claude",
				KnowledgeFile: "CLAUDE.md",
			},
			Verify: VerifyConfig{
				Default: []string{"bun run typecheck", "bun run test"},
			},
			Services: []ServiceConfig{
				{Name: "dev", Ready: "http://localhost:3000"},
			},
		},
	}

	prompt := generateRefinePrompt(cfg, featureDir, def, state)

	// Check prd.md content is included
	if !strings.Contains(prompt, "Auth Feature") {
		t.Error("prompt should contain prd.md content")
	}
	if !strings.Contains(prompt, "Build login and logout") {
		t.Error("prompt should contain prd.md body")
	}

	// Check prd.json content is included
	if !strings.Contains(prompt, "schemaVersion") {
		t.Error("prompt should contain prd.json content")
	}

	// Check progress
	if !strings.Contains(prompt, "1/3 stories complete (1 skipped)") {
		t.Error("prompt should contain progress")
	}

	// Check story details
	if !strings.Contains(prompt, "US-001: Login** — PASSED") {
		t.Error("prompt should show US-001 as PASSED")
	}
	if !strings.Contains(prompt, "US-002: Logout** — SKIPPED") {
		t.Error("prompt should show US-002 as SKIPPED")
	}
	if !strings.Contains(prompt, "Depends on sessions") {
		t.Error("prompt should include story notes")
	}

	// Check learnings
	if !strings.Contains(prompt, "bcrypt") {
		t.Error("prompt should include learnings")
	}

	// Check environment info
	if !strings.Contains(prompt, "ralph/auth") {
		t.Error("prompt should contain branch name")
	}
	if !strings.Contains(prompt, "CLAUDE.md") {
		t.Error("prompt should contain knowledge file")
	}

	// Check verify commands
	if !strings.Contains(prompt, "bun run typecheck") {
		t.Error("prompt should contain verify commands")
	}

	// Check service URLs
	if !strings.Contains(prompt, "localhost:3000") {
		t.Error("prompt should contain service URLs")
	}

	// Check guidance
	if !strings.Contains(prompt, "ralph run auth") {
		t.Error("prompt should mention ralph run for resuming")
	}
}

func TestBuildRefinementStoryDetails(t *testing.T) {
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-001", Title: "Login"},
			{ID: "US-002", Title: "Logout"},
			{ID: "US-003", Title: "Register"},
		},
	}

	state := NewRunState()
	state.MarkPassed("US-001")
	state.MarkSkipped("US-002", "Cannot implement")
	state.Retries = map[string]int{"US-002": 3}

	result := buildRefinementStoryDetails(def, state)

	if !strings.Contains(result, "Story Execution Status") {
		t.Error("expected heading")
	}
	if !strings.Contains(result, "US-001: Login** — PASSED") {
		t.Error("expected PASSED status for US-001")
	}
	if !strings.Contains(result, "US-002: Logout** — SKIPPED (3 retries)") {
		t.Error("expected SKIPPED with retries for US-002")
	}
	if !strings.Contains(result, "Notes: Cannot implement") {
		t.Error("expected notes for US-002")
	}
	if !strings.Contains(result, "US-003: Register** — PENDING") {
		t.Error("expected PENDING for US-003")
	}
}

func TestBuildRefinementStoryDetails_Empty(t *testing.T) {
	def := &PRDDefinition{UserStories: []StoryDefinition{}}
	state := NewRunState()
	result := buildRefinementStoryDetails(def, state)
	if result != "" {
		t.Errorf("expected empty string for no stories, got %q", result)
	}
}

func TestBuildCriteriaChecklist(t *testing.T) {
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-001", Title: "Login", AcceptanceCriteria: []string{"Form renders", "Auth works"}},
			{ID: "US-002", Title: "Logout", AcceptanceCriteria: []string{"Session cleared"}},
			{ID: "US-003", Title: "Register", AcceptanceCriteria: []string{"Account created"}},
		},
	}

	state := NewRunState()
	state.MarkPassed("US-001")
	state.MarkSkipped("US-003", "blocked")

	result := buildCriteriaChecklist(def, state)

	if !strings.Contains(result, "### US-001: Login (PASSED)") {
		t.Error("expected US-001 with PASSED status")
	}
	if !strings.Contains(result, "### US-002: Logout (PENDING)") {
		t.Error("expected US-002 with PENDING status")
	}
	if !strings.Contains(result, "### US-003: Register (SKIPPED)") {
		t.Error("expected US-003 with SKIPPED status")
	}
	if !strings.Contains(result, "- [ ] Form renders") {
		t.Error("expected criterion checkbox")
	}
	if !strings.Contains(result, "- [ ] Session cleared") {
		t.Error("expected criterion checkbox")
	}
}

func TestBuildCriteriaChecklist_Empty(t *testing.T) {
	def := &PRDDefinition{UserStories: []StoryDefinition{}}
	state := NewRunState()
	result := buildCriteriaChecklist(def, state)
	if result != "" {
		t.Errorf("expected empty string for no stories, got %q", result)
	}
}

func TestGenerateRunPrompt_CrossFeatureLearnings(t *testing.T) {
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")

	// Create another feature with learnings
	otherFeature := filepath.Join(ralphDir, "2024-01-10-billing")
	os.MkdirAll(otherFeature, 0755)
	otherState := NewRunState()
	otherState.MarkPassed("US-010")
	otherState.Learnings = []string{"Stripe needs idempotency keys"}
	otherStateJSON, _ := json.Marshal(otherState)
	os.WriteFile(filepath.Join(otherFeature, "run-state.json"), otherStateJSON, 0644)

	// Current feature directory
	currentFeature := filepath.Join(ralphDir, "2024-01-20-auth")
	os.MkdirAll(currentFeature, 0755)

	cfg := &ResolvedConfig{
		ProjectRoot: dir,
		Config: RalphConfig{
			MaxRetries: 3,
			Provider: ProviderConfig{
				Command:       "claude",
				KnowledgeFile: "CLAUDE.md",
			},
			Verify: VerifyConfig{
				Default: []string{"echo ok"},
			},
		},
	}

	featureDir := &FeatureDir{
		Feature: "auth",
		Path:    currentFeature,
	}

	def := &PRDDefinition{
		Project:     "TestApp",
		Description: "Auth",
		BranchName:  "ralph/auth",
		UserStories: []StoryDefinition{
			{ID: "US-001", Title: "Login", AcceptanceCriteria: []string{"Works"}},
		},
	}

	state := NewRunState()
	story := &def.UserStories[0]

	prompt := generateRunPrompt(cfg, featureDir, def, state, story, "", "")

	if !strings.Contains(prompt, "Learnings from Previous Features") {
		t.Error("prompt should contain cross-feature learnings heading")
	}
	if !strings.Contains(prompt, "Stripe needs idempotency keys") {
		t.Error("prompt should contain cross-feature learning content")
	}
}

func TestGenerateVerifyAnalyzePrompt(t *testing.T) {
	cfg := &ResolvedConfig{
		ProjectRoot: "/project",
		Config: RalphConfig{
			Provider: ProviderConfig{
				Command:       "claude",
				KnowledgeFile: "CLAUDE.md",
			},
			Verify: VerifyConfig{
				Default: []string{"go vet ./...", "go test ./..."},
			},
		},
	}

	featureDir := &FeatureDir{
		Feature: "auth",
		Path:    "/project/.ralph/2024-01-15-auth",
	}

	def := &PRDDefinition{
		Project:     "MyApp",
		Description: "Auth feature",
		BranchName:  "ralph/auth",
		UserStories: []StoryDefinition{
			{ID: "US-001", Title: "Login", AcceptanceCriteria: []string{"Form renders"}},
			{ID: "US-002", Title: "Logout", AcceptanceCriteria: []string{"Session cleared"}},
		},
	}

	state := NewRunState()
	state.MarkPassed("US-001")

	report := &VerifyReport{}
	report.AddPass("go vet ./...")
	report.AddFail("go test ./...", "exit code 1")

	prompt := generateVerifyAnalyzePrompt(cfg, featureDir, def, state, report)

	if !strings.Contains(prompt, "MyApp") {
		t.Error("prompt should contain project name")
	}
	if !strings.Contains(prompt, "Auth feature") {
		t.Error("prompt should contain feature description")
	}
	if !strings.Contains(prompt, "ralph/auth") {
		t.Error("prompt should contain branch name")
	}
	if !strings.Contains(prompt, "US-001: Login (PASSED)") {
		t.Error("prompt should contain criteria checklist")
	}
	if !strings.Contains(prompt, "VERIFY_PASS") {
		t.Error("prompt should contain VERIFY_PASS marker instructions")
	}
	if !strings.Contains(prompt, "VERIFY_FAIL") {
		t.Error("prompt should contain VERIFY_FAIL marker instructions")
	}
}
