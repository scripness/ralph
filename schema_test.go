package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidatePRD_Valid(t *testing.T) {
	prd := &PRD{
		SchemaVersion: 2,
		Project:       "TestProject",
		BranchName:    "ralph/test",
		Description:   "Test description",
		Run: Run{
			Learnings: []string{},
		},
		UserStories: []UserStory{
			{
				ID:                 "US-001",
				Title:              "Test story",
				Description:        "As a user...",
				AcceptanceCriteria: []string{"Criterion 1"},
				Priority:           1,
				Passes:             false,
				Retries:            0,
			},
		},
	}

	err := ValidatePRD(prd)
	if err != nil {
		t.Errorf("expected valid PRD, got error: %v", err)
	}
}

func TestValidatePRD_WrongSchemaVersion(t *testing.T) {
	prd := &PRD{
		SchemaVersion: 1,
		Project:       "Test",
		BranchName:    "ralph/test",
		UserStories: []UserStory{
			{ID: "US-001", Title: "Test", AcceptanceCriteria: []string{"x"}},
		},
	}

	err := ValidatePRD(prd)
	if err == nil {
		t.Error("expected error for wrong schema version")
	}
}

func TestValidatePRD_MissingProject(t *testing.T) {
	prd := &PRD{
		SchemaVersion: 2,
		BranchName:    "ralph/test",
		UserStories: []UserStory{
			{ID: "US-001", Title: "Test", AcceptanceCriteria: []string{"x"}},
		},
	}

	err := ValidatePRD(prd)
	if err == nil {
		t.Error("expected error for missing project")
	}
}

func TestValidatePRD_MissingBranchName(t *testing.T) {
	prd := &PRD{
		SchemaVersion: 2,
		Project:       "Test",
		UserStories: []UserStory{
			{ID: "US-001", Title: "Test", AcceptanceCriteria: []string{"x"}},
		},
	}

	err := ValidatePRD(prd)
	if err == nil {
		t.Error("expected error for missing branchName")
	}
}

func TestValidatePRD_EmptyUserStories(t *testing.T) {
	prd := &PRD{
		SchemaVersion: 2,
		Project:       "Test",
		BranchName:    "ralph/test",
		UserStories:   []UserStory{},
	}

	err := ValidatePRD(prd)
	if err == nil {
		t.Error("expected error for empty userStories")
	}
}

func TestValidatePRD_StoryMissingID(t *testing.T) {
	prd := &PRD{
		SchemaVersion: 2,
		Project:       "Test",
		BranchName:    "ralph/test",
		UserStories: []UserStory{
			{Title: "Test", AcceptanceCriteria: []string{"x"}},
		},
	}

	err := ValidatePRD(prd)
	if err == nil {
		t.Error("expected error for story missing ID")
	}
}

func TestValidatePRD_StoryMissingTitle(t *testing.T) {
	prd := &PRD{
		SchemaVersion: 2,
		Project:       "Test",
		BranchName:    "ralph/test",
		UserStories: []UserStory{
			{ID: "US-001", AcceptanceCriteria: []string{"x"}},
		},
	}

	err := ValidatePRD(prd)
	if err == nil {
		t.Error("expected error for story missing title")
	}
}

func TestValidatePRD_StoryMissingAcceptanceCriteria(t *testing.T) {
	prd := &PRD{
		SchemaVersion: 2,
		Project:       "Test",
		BranchName:    "ralph/test",
		UserStories: []UserStory{
			{ID: "US-001", Title: "Test"},
		},
	}

	err := ValidatePRD(prd)
	if err == nil {
		t.Error("expected error for story missing acceptanceCriteria")
	}
}

func TestGetNextStory_PicksHighestPriority(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-003", Priority: 3, Passes: false},
			{ID: "US-001", Priority: 1, Passes: false},
			{ID: "US-002", Priority: 2, Passes: false},
		},
	}

	next := GetNextStory(prd)
	if next == nil {
		t.Fatal("expected next story, got nil")
	}
	if next.ID != "US-001" {
		t.Errorf("expected US-001, got %s", next.ID)
	}
}

func TestGetNextStory_SkipsPassingStories(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Priority: 1, Passes: true},
			{ID: "US-002", Priority: 2, Passes: false},
			{ID: "US-003", Priority: 3, Passes: false},
		},
	}

	next := GetNextStory(prd)
	if next == nil {
		t.Fatal("expected next story, got nil")
	}
	if next.ID != "US-002" {
		t.Errorf("expected US-002, got %s", next.ID)
	}
}

func TestGetNextStory_SkipsBlockedStories(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Priority: 1, Passes: false, Blocked: true},
			{ID: "US-002", Priority: 2, Passes: false, Blocked: false},
			{ID: "US-003", Priority: 3, Passes: false, Blocked: false},
		},
	}

	next := GetNextStory(prd)
	if next == nil {
		t.Fatal("expected next story, got nil")
	}
	if next.ID != "US-002" {
		t.Errorf("expected US-002, got %s", next.ID)
	}
}

func TestGetNextStory_ReturnsNilWhenAllPass(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Priority: 1, Passes: true},
			{ID: "US-002", Priority: 2, Passes: true},
		},
	}

	next := GetNextStory(prd)
	if next != nil {
		t.Errorf("expected nil, got %s", next.ID)
	}
}

func TestAllStoriesComplete_True(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{Passes: true},
			{Passes: true},
		},
	}

	if !AllStoriesComplete(prd) {
		t.Error("expected all stories complete")
	}
}

func TestAllStoriesComplete_False(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{Passes: true},
			{Passes: false},
		},
	}

	if AllStoriesComplete(prd) {
		t.Error("expected not all stories complete")
	}
}

func TestAllStoriesComplete_BlockedCountsAsComplete(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{Passes: true},
			{Passes: false, Blocked: true},
		},
	}

	// Blocked stories don't prevent completion
	if !AllStoriesComplete(prd) {
		t.Error("expected all stories complete (blocked counts as done)")
	}
}

func TestLoadPRD_ValidFile(t *testing.T) {
	dir := t.TempDir()
	prdPath := filepath.Join(dir, "prd.json")

	content := `{
		"schemaVersion": 2,
		"project": "Test",
		"branchName": "ralph/test",
		"description": "Test",
		"run": {"startedAt": null, "currentStoryId": null, "learnings": []},
		"userStories": [
			{"id": "US-001", "title": "Test", "description": "...", "acceptanceCriteria": ["x"], "priority": 1, "passes": false, "retries": 0, "blocked": false, "lastResult": null, "notes": ""}
		]
	}`
	os.WriteFile(prdPath, []byte(content), 0644)

	prd, err := LoadPRD(prdPath)
	if err != nil {
		t.Fatalf("failed to load PRD: %v", err)
	}
	if prd.Project != "Test" {
		t.Errorf("expected project 'Test', got '%s'", prd.Project)
	}
}

func TestLoadPRD_FileNotFound(t *testing.T) {
	_, err := LoadPRD("/nonexistent/prd.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadPRD_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	prdPath := filepath.Join(dir, "prd.json")
	os.WriteFile(prdPath, []byte("not json"), 0644)

	_, err := LoadPRD(prdPath)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestIsUIStory(t *testing.T) {
	uiStory := &UserStory{Tags: []string{"ui"}}
	if !IsUIStory(uiStory) {
		t.Error("expected story with 'ui' tag to be UI story")
	}

	nonUIStory := &UserStory{Tags: []string{"backend"}}
	if IsUIStory(nonUIStory) {
		t.Error("expected story without 'ui' tag to not be UI story")
	}

	emptyTags := &UserStory{Tags: []string{}}
	if IsUIStory(emptyTags) {
		t.Error("expected story with empty tags to not be UI story")
	}
}

func TestMarkStoryFailed(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Retries: 0, Blocked: false},
		},
	}

	prd.MarkStoryFailed("US-001", "test failure", 3)

	story := GetStoryByID(prd, "US-001")
	if story.Retries != 1 {
		t.Errorf("expected retries=1, got %d", story.Retries)
	}
	if story.Notes != "test failure" {
		t.Errorf("expected notes='test failure', got '%s'", story.Notes)
	}
	if story.Blocked {
		t.Error("expected not blocked after first failure")
	}
}

func TestMarkStoryFailed_Blocked(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Retries: 2, Blocked: false},
		},
	}

	prd.MarkStoryFailed("US-001", "third failure", 3)

	story := GetStoryByID(prd, "US-001")
	if story.Retries != 3 {
		t.Errorf("expected retries=3, got %d", story.Retries)
	}
	if !story.Blocked {
		t.Error("expected blocked after reaching maxRetries")
	}
}

func TestResetStory(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Passes: true, Retries: 0, LastResult: &LastResult{Commit: "abc123"}},
		},
	}

	prd.ResetStory("US-001", "needs rework", 3)

	story := GetStoryByID(prd, "US-001")
	if story.Passes {
		t.Error("expected passes=false after reset")
	}
	if story.LastResult != nil {
		t.Error("expected lastResult=nil after reset")
	}
	if story.Retries != 1 {
		t.Errorf("expected retries=1, got %d", story.Retries)
	}
	if story.Notes != "needs rework" {
		t.Errorf("expected notes='needs rework', got '%s'", story.Notes)
	}
}

func TestMarkStoryBlocked(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-007", Blocked: false, Notes: ""},
		},
	}

	prd.MarkStoryBlocked("US-007", "Depends on US-003 which isn't complete")

	story := GetStoryByID(prd, "US-007")
	if !story.Blocked {
		t.Error("expected blocked=true")
	}
	if story.Notes != "Depends on US-003 which isn't complete" {
		t.Errorf("expected notes set, got '%s'", story.Notes)
	}
}

func TestMarkStoryBlocked_NonExistent(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Blocked: false},
		},
	}

	// Should not panic when blocking non-existent story
	prd.MarkStoryBlocked("US-999", "reason")

	story := GetStoryByID(prd, "US-001")
	if story.Blocked {
		t.Error("expected US-001 to remain unblocked")
	}
}

func TestAddLearning_Deduplication(t *testing.T) {
	prd := &PRD{Run: Run{Learnings: []string{}}}

	prd.AddLearning("first learning")
	prd.AddLearning("second learning")
	prd.AddLearning("first learning") // duplicate

	if len(prd.Run.Learnings) != 2 {
		t.Errorf("expected 2 learnings (deduplicated), got %d: %v", len(prd.Run.Learnings), prd.Run.Learnings)
	}
	if prd.Run.Learnings[0] != "first learning" {
		t.Errorf("expected first='first learning', got '%s'", prd.Run.Learnings[0])
	}
	if prd.Run.Learnings[1] != "second learning" {
		t.Errorf("expected second='second learning', got '%s'", prd.Run.Learnings[1])
	}
}

func TestAddLearning_UniqueAdded(t *testing.T) {
	prd := &PRD{Run: Run{Learnings: []string{}}}

	prd.AddLearning("learning A")
	prd.AddLearning("learning B")
	prd.AddLearning("learning C")

	if len(prd.Run.Learnings) != 3 {
		t.Errorf("expected 3 unique learnings, got %d", len(prd.Run.Learnings))
	}
}

func TestAddLearning_NilLearnings(t *testing.T) {
	prd := &PRD{Run: Run{Learnings: nil}}

	prd.AddLearning("first")
	prd.AddLearning("first") // duplicate

	if len(prd.Run.Learnings) != 1 {
		t.Errorf("expected 1 learning, got %d", len(prd.Run.Learnings))
	}
}

func TestAddLearning_NormalizedDedup(t *testing.T) {
	prd := &PRD{Run: Run{Learnings: []string{}}}

	prd.AddLearning("Must restart dev server after schema changes")
	prd.AddLearning("must restart dev server after schema changes")  // case diff
	prd.AddLearning("Must restart dev server after schema changes.") // trailing period

	if len(prd.Run.Learnings) != 1 {
		t.Errorf("expected 1 learning (normalized dedup), got %d: %v", len(prd.Run.Learnings), prd.Run.Learnings)
	}
}

func TestNormalizeLearning(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello world"},
		{"trailing period.", "trailing period"},
		{"  spaces  ", "spaces"},
		{"Multiple!!!!", "multiple"},
		{"keep:colons", "keep:colons"},
	}
	for _, tt := range tests {
		got := normalizeLearning(tt.input)
		if got != tt.want {
			t.Errorf("normalizeLearning(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBrowserSteps_InUserStory(t *testing.T) {
	prd := &PRD{
		SchemaVersion: 2,
		Project:       "Test",
		BranchName:    "ralph/test",
		UserStories: []UserStory{
			{
				ID:                 "US-001",
				Title:              "Login form",
				AcceptanceCriteria: []string{"Login works"},
				Tags:               []string{"ui"},
				BrowserSteps: []BrowserStep{
					{Action: "navigate", URL: "/login"},
					{Action: "type", Selector: "#email", Value: "test@example.com"},
					{Action: "click", Selector: "button[type=submit]"},
					{Action: "waitFor", Selector: ".dashboard"},
					{Action: "assertText", Selector: "h1", Contains: "Welcome"},
				},
			},
		},
	}

	err := ValidatePRD(prd)
	if err != nil {
		t.Errorf("expected valid PRD with browserSteps, got error: %v", err)
	}

	story := GetStoryByID(prd, "US-001")
	if len(story.BrowserSteps) != 5 {
		t.Errorf("expected 5 browser steps, got %d", len(story.BrowserSteps))
	}
	if story.BrowserSteps[0].Action != "navigate" {
		t.Errorf("expected first step action='navigate', got '%s'", story.BrowserSteps[0].Action)
	}
	if story.BrowserSteps[1].Selector != "#email" {
		t.Errorf("expected second step selector='#email', got '%s'", story.BrowserSteps[1].Selector)
	}
}

func TestBrowserStep_Timeout(t *testing.T) {
	step := BrowserStep{
		Action:   "waitFor",
		Selector: ".slow-element",
		Timeout:  30,
	}

	if step.Timeout != 30 {
		t.Errorf("expected timeout=30, got %d", step.Timeout)
	}
}

func TestWarnPRDQuality_MissingTypecheck(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", AcceptanceCriteria: []string{"Feature works correctly"}},
			{ID: "US-002", AcceptanceCriteria: []string{"Typecheck passes", "UI renders"}},
		},
	}

	warnings := WarnPRDQuality(prd)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
	if warnings[0] != "US-001: missing 'Typecheck passes' criterion" {
		t.Errorf("unexpected warning: %s", warnings[0])
	}
}

func TestWarnPRDQuality_AllGood(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", AcceptanceCriteria: []string{"Typecheck passes", "Tests pass"}},
			{ID: "US-002", AcceptanceCriteria: []string{"Typecheck passes", "UI renders"}},
		},
	}

	warnings := WarnPRDQuality(prd)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestWarnPRDQuality_CaseInsensitive(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", AcceptanceCriteria: []string{"typecheck passes"}},
			{ID: "US-002", AcceptanceCriteria: []string{"All TYPECHECK errors resolved"}},
		},
	}

	warnings := WarnPRDQuality(prd)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for case-insensitive match, got %v", warnings)
	}
}

func TestSetCurrentStory(t *testing.T) {
	prd := &PRD{Run: Run{}}

	prd.SetCurrentStory("US-001")

	if prd.Run.CurrentStoryID == nil || *prd.Run.CurrentStoryID != "US-001" {
		t.Errorf("expected currentStoryId='US-001', got %v", prd.Run.CurrentStoryID)
	}
	if prd.Run.StartedAt == nil {
		t.Error("expected startedAt to be set")
	}
}

func TestSetCurrentStory_PreservesStartedAt(t *testing.T) {
	ts := "2024-01-15T10:00:00Z"
	prd := &PRD{Run: Run{StartedAt: &ts}}

	prd.SetCurrentStory("US-002")

	if prd.Run.StartedAt == nil || *prd.Run.StartedAt != ts {
		t.Errorf("expected startedAt preserved as '%s', got %v", ts, prd.Run.StartedAt)
	}
}

func TestClearCurrentStory(t *testing.T) {
	id := "US-001"
	prd := &PRD{Run: Run{CurrentStoryID: &id}}

	prd.ClearCurrentStory()

	if prd.Run.CurrentStoryID != nil {
		t.Errorf("expected currentStoryId=nil, got %v", prd.Run.CurrentStoryID)
	}
}

func TestMarkStoryPassed(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Passes: false},
		},
	}

	prd.MarkStoryPassed("US-001", "abc123", "Implemented login")

	story := GetStoryByID(prd, "US-001")
	if !story.Passes {
		t.Error("expected passes=true")
	}
	if story.LastResult == nil {
		t.Fatal("expected lastResult to be set")
	}
	if story.LastResult.Commit != "abc123" {
		t.Errorf("expected commit='abc123', got '%s'", story.LastResult.Commit)
	}
	if story.LastResult.Summary != "Implemented login" {
		t.Errorf("expected summary='Implemented login', got '%s'", story.LastResult.Summary)
	}
	if story.LastResult.CompletedAt == "" {
		t.Error("expected completedAt to be set")
	}
}

func TestMarkStoryPassed_NotFound(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Passes: false},
		},
	}

	// Should not panic
	prd.MarkStoryPassed("US-999", "abc", "summary")

	story := GetStoryByID(prd, "US-001")
	if story.Passes {
		t.Error("expected US-001 to remain unpassed")
	}
}

func TestHasBlockedStories(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Blocked: false},
			{ID: "US-002", Blocked: true},
		},
	}

	if !HasBlockedStories(prd) {
		t.Error("expected HasBlockedStories=true")
	}

	prdNoBlocked := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Blocked: false},
		},
	}
	if HasBlockedStories(prdNoBlocked) {
		t.Error("expected HasBlockedStories=false")
	}
}

func TestGetBlockedStories(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Blocked: false},
			{ID: "US-002", Blocked: true},
			{ID: "US-003", Blocked: true},
		},
	}

	blocked := GetBlockedStories(prd)
	if len(blocked) != 2 {
		t.Fatalf("expected 2 blocked stories, got %d", len(blocked))
	}
	if blocked[0].ID != "US-002" || blocked[1].ID != "US-003" {
		t.Errorf("unexpected blocked stories: %v", blocked)
	}
}

func TestCountComplete(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Passes: true},
			{ID: "US-002", Passes: false},
			{ID: "US-003", Passes: true},
		},
	}

	if count := CountComplete(prd); count != 2 {
		t.Errorf("expected 2 complete, got %d", count)
	}
}

func TestCountBlocked(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Blocked: false},
			{ID: "US-002", Blocked: true},
			{ID: "US-003", Blocked: true},
		},
	}

	if count := CountBlocked(prd); count != 2 {
		t.Errorf("expected 2 blocked, got %d", count)
	}
}

func TestGetStoryByID(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Title: "First"},
			{ID: "US-002", Title: "Second"},
		},
	}

	story := GetStoryByID(prd, "US-002")
	if story == nil {
		t.Fatal("expected story, got nil")
	}
	if story.Title != "Second" {
		t.Errorf("expected title='Second', got '%s'", story.Title)
	}

	notFound := GetStoryByID(prd, "US-999")
	if notFound != nil {
		t.Errorf("expected nil for non-existent ID, got %v", notFound)
	}
}

func TestSavePRD_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	prdPath := filepath.Join(dir, "prd.json")

	original := &PRD{
		SchemaVersion: 2,
		Project:       "RoundTrip",
		BranchName:    "ralph/test",
		Description:   "Test round trip",
		Run:           Run{Learnings: []string{"learning1"}},
		UserStories: []UserStory{
			{
				ID:                 "US-001",
				Title:              "Test",
				Description:        "Desc",
				AcceptanceCriteria: []string{"works"},
				Priority:           1,
			},
		},
	}

	if err := SavePRD(prdPath, original); err != nil {
		t.Fatalf("SavePRD failed: %v", err)
	}

	loaded, err := LoadPRD(prdPath)
	if err != nil {
		t.Fatalf("LoadPRD failed: %v", err)
	}

	if loaded.Project != "RoundTrip" {
		t.Errorf("expected project='RoundTrip', got '%s'", loaded.Project)
	}
	if len(loaded.Run.Learnings) != 1 || loaded.Run.Learnings[0] != "learning1" {
		t.Errorf("expected learnings preserved, got %v", loaded.Run.Learnings)
	}
	if len(loaded.UserStories) != 1 || loaded.UserStories[0].ID != "US-001" {
		t.Errorf("expected stories preserved, got %v", loaded.UserStories)
	}
}

func TestLoadPRD_WithBrowserSteps(t *testing.T) {
	dir := t.TempDir()
	prdPath := filepath.Join(dir, "prd.json")

	content := `{
		"schemaVersion": 2,
		"project": "Test",
		"branchName": "ralph/test",
		"description": "Test",
		"run": {"startedAt": null, "currentStoryId": null, "learnings": []},
		"userStories": [
			{
				"id": "US-001",
				"title": "Test",
				"description": "...",
				"acceptanceCriteria": ["x"],
				"tags": ["ui"],
				"priority": 1,
				"passes": false,
				"retries": 0,
				"blocked": false,
				"lastResult": null,
				"notes": "",
				"browserSteps": [
					{"action": "navigate", "url": "/test"},
					{"action": "assertVisible", "selector": ".content"}
				]
			}
		]
	}`
	os.WriteFile(prdPath, []byte(content), 0644)

	prd, err := LoadPRD(prdPath)
	if err != nil {
		t.Fatalf("failed to load PRD: %v", err)
	}

	story := GetStoryByID(prd, "US-001")
	if len(story.BrowserSteps) != 2 {
		t.Errorf("expected 2 browser steps, got %d", len(story.BrowserSteps))
	}
	if story.BrowserSteps[0].URL != "/test" {
		t.Errorf("expected URL='/test', got '%s'", story.BrowserSteps[0].URL)
	}
}
