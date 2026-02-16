package main

import (
	"os"
	"path/filepath"
	"testing"
)

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

func TestResetStoryForPreVerify(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Passes: true, Retries: 2, LastResult: &LastResult{Commit: "abc123"}},
		},
	}

	prd.ResetStoryForPreVerify("US-001", "pre-verify: acceptance criteria changed")

	story := GetStoryByID(prd, "US-001")
	if story.Passes {
		t.Error("expected passes=false after pre-verify reset")
	}
	if story.LastResult != nil {
		t.Error("expected lastResult=nil after pre-verify reset")
	}
	// Key difference from ResetStory: retries should NOT be incremented
	if story.Retries != 2 {
		t.Errorf("expected retries=2 (unchanged), got %d", story.Retries)
	}
	if story.Notes != "pre-verify: acceptance criteria changed" {
		t.Errorf("expected notes set, got '%s'", story.Notes)
	}
	if story.Blocked {
		t.Error("expected not blocked (pre-verify reset doesn't block)")
	}
}

func TestResetStoryForPreVerify_NotFound(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Passes: true},
		},
	}

	// Should not panic
	prd.ResetStoryForPreVerify("US-999", "reason")

	story := GetStoryByID(prd, "US-001")
	if !story.Passes {
		t.Error("expected US-001 to remain passed")
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
		SchemaVersion: 3,
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

func TestGetPendingStories(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Passes: true},
			{ID: "US-002", Passes: false, Blocked: false},
			{ID: "US-003", Passes: false, Blocked: true},
			{ID: "US-004", Passes: false, Blocked: false},
		},
	}

	pending := GetPendingStories(prd)
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending, got %d", len(pending))
	}
	if pending[0].ID != "US-002" || pending[1].ID != "US-004" {
		t.Errorf("expected US-002 and US-004, got %s and %s", pending[0].ID, pending[1].ID)
	}
}

func TestGetPendingStories_AllComplete(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Passes: true},
			{ID: "US-002", Passes: false, Blocked: true},
		},
	}

	pending := GetPendingStories(prd)
	if len(pending) != 0 {
		t.Errorf("expected 0 pending, got %d", len(pending))
	}
}

func TestGetNextStory_PrefersCurrentStoryID(t *testing.T) {
	currentID := "US-003"
	prd := &PRD{
		Run: Run{CurrentStoryID: &currentID},
		UserStories: []UserStory{
			{ID: "US-001", Priority: 1, Passes: false},
			{ID: "US-002", Priority: 2, Passes: false},
			{ID: "US-003", Priority: 3, Passes: false}, // lower priority but current
		},
	}

	next := GetNextStory(prd)
	if next == nil {
		t.Fatal("expected next story, got nil")
	}
	if next.ID != "US-003" {
		t.Errorf("expected US-003 (currentStoryID), got %s", next.ID)
	}
}

func TestGetNextStory_CurrentStoryID_PassedFallsThrough(t *testing.T) {
	currentID := "US-001"
	prd := &PRD{
		Run: Run{CurrentStoryID: &currentID},
		UserStories: []UserStory{
			{ID: "US-001", Priority: 1, Passes: true}, // current but already passed
			{ID: "US-002", Priority: 2, Passes: false},
		},
	}

	next := GetNextStory(prd)
	if next == nil {
		t.Fatal("expected next story, got nil")
	}
	if next.ID != "US-002" {
		t.Errorf("expected US-002 (fallthrough), got %s", next.ID)
	}
}

func TestGetNextStory_CurrentStoryID_BlockedFallsThrough(t *testing.T) {
	currentID := "US-001"
	prd := &PRD{
		Run: Run{CurrentStoryID: &currentID},
		UserStories: []UserStory{
			{ID: "US-001", Priority: 1, Passes: false, Blocked: true},
			{ID: "US-002", Priority: 2, Passes: false},
		},
	}

	next := GetNextStory(prd)
	if next == nil {
		t.Fatal("expected next story, got nil")
	}
	if next.ID != "US-002" {
		t.Errorf("expected US-002 (fallthrough from blocked), got %s", next.ID)
	}
}

func TestGetNextStory_CurrentStoryID_NonExistentFallsThrough(t *testing.T) {
	currentID := "US-999"
	prd := &PRD{
		Run: Run{CurrentStoryID: &currentID},
		UserStories: []UserStory{
			{ID: "US-001", Priority: 1, Passes: false},
			{ID: "US-002", Priority: 2, Passes: false},
		},
	}

	next := GetNextStory(prd)
	if next == nil {
		t.Fatal("expected next story, got nil")
	}
	if next.ID != "US-001" {
		t.Errorf("expected US-001 (fallthrough from nonexistent), got %s", next.ID)
	}
}

func TestMarkStoryPassed_ClearsBlocked(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Passes: false, Blocked: true},
		},
	}

	prd.MarkStoryPassed("US-001", "abc123", "Implemented")

	story := GetStoryByID(prd, "US-001")
	if !story.Passes {
		t.Error("expected passes=true")
	}
	if story.Blocked {
		t.Error("expected blocked=false after marking passed")
	}
}

func TestMarkStoryFailed_ClearsPasses(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Passes: true, LastResult: &LastResult{Commit: "abc"}},
		},
	}

	prd.MarkStoryFailed("US-001", "test failure", 3)

	story := GetStoryByID(prd, "US-001")
	if story.Passes {
		t.Error("expected passes=false after marking failed")
	}
	if story.LastResult != nil {
		t.Error("expected lastResult=nil after marking failed")
	}
}

func TestMarkStoryBlocked_ClearsPasses(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Passes: true, LastResult: &LastResult{Commit: "abc"}},
		},
	}

	prd.MarkStoryBlocked("US-001", "impossible to implement")

	story := GetStoryByID(prd, "US-001")
	if story.Passes {
		t.Error("expected passes=false after marking blocked")
	}
	if story.LastResult != nil {
		t.Error("expected lastResult=nil after marking blocked")
	}
	if !story.Blocked {
		t.Error("expected blocked=true")
	}
}

// --- v3 / WorkingPRD tests ---

func writeV3PRD(t *testing.T, dir string) string {
	t.Helper()
	prdPath := filepath.Join(dir, "prd.json")
	def := &PRDDefinition{
		SchemaVersion: 3,
		Project:       "TestProject",
		BranchName:    "ralph/test",
		Description:   "Test feature",
		UserStories: []StoryDefinition{
			{
				ID:                 "US-001",
				Title:              "First story",
				Description:        "As a user...",
				AcceptanceCriteria: []string{"Criterion 1"},
				Priority:           1,
			},
			{
				ID:                 "US-002",
				Title:              "Second story",
				Description:        "As a user...",
				AcceptanceCriteria: []string{"Criterion 2"},
				Tags:               []string{"ui"},
				Priority:           2,
			},
		},
	}
	if err := AtomicWriteJSON(prdPath, def); err != nil {
		t.Fatalf("failed to write v3 PRD: %v", err)
	}
	return prdPath
}

func TestLoadPRDDefinition_Valid(t *testing.T) {
	dir := t.TempDir()
	prdPath := writeV3PRD(t, dir)

	def, err := LoadPRDDefinition(prdPath)
	if err != nil {
		t.Fatalf("LoadPRDDefinition failed: %v", err)
	}
	if def.SchemaVersion != 3 {
		t.Errorf("expected schemaVersion=3, got %d", def.SchemaVersion)
	}
	if def.Project != "TestProject" {
		t.Errorf("expected project='TestProject', got '%s'", def.Project)
	}
	if len(def.UserStories) != 2 {
		t.Errorf("expected 2 stories, got %d", len(def.UserStories))
	}
}

func TestLoadPRDDefinition_NotFound(t *testing.T) {
	_, err := LoadPRDDefinition("/nonexistent/prd.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadPRDDefinition_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	prdPath := filepath.Join(dir, "prd.json")
	os.WriteFile(prdPath, []byte("not json"), 0644)

	_, err := LoadPRDDefinition(prdPath)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestValidatePRDDefinition_WrongVersion(t *testing.T) {
	def := &PRDDefinition{
		SchemaVersion: 2,
		Project:       "Test",
		BranchName:    "ralph/test",
		UserStories:   []StoryDefinition{{ID: "US-001", Title: "Test", AcceptanceCriteria: []string{"x"}}},
	}
	if err := ValidatePRDDefinition(def); err == nil {
		t.Error("expected error for wrong schemaVersion")
	}
}

func TestValidatePRDDefinition_MissingProject(t *testing.T) {
	def := &PRDDefinition{
		SchemaVersion: 3,
		BranchName:    "ralph/test",
		UserStories:   []StoryDefinition{{ID: "US-001", Title: "Test", AcceptanceCriteria: []string{"x"}}},
	}
	if err := ValidatePRDDefinition(def); err == nil {
		t.Error("expected error for missing project")
	}
}

func TestValidatePRDDefinition_EmptyStories(t *testing.T) {
	def := &PRDDefinition{
		SchemaVersion: 3,
		Project:       "Test",
		BranchName:    "ralph/test",
		UserStories:   []StoryDefinition{},
	}
	if err := ValidatePRDDefinition(def); err == nil {
		t.Error("expected error for empty stories")
	}
}

func TestLoadRunState_NotFound(t *testing.T) {
	state, err := LoadRunState("/nonexistent/run-state.json")
	if err != nil {
		t.Fatalf("expected no error for missing state file, got: %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if len(state.Stories) != 0 {
		t.Errorf("expected empty stories map, got %d entries", len(state.Stories))
	}
}

func TestLoadRunState_Valid(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "run-state.json")

	state := &RunState{
		Learnings: []string{"learned something"},
		Stories: map[string]*StoryState{
			"US-001": {Passes: true, Retries: 1},
		},
	}
	if err := SaveRunState(statePath, state); err != nil {
		t.Fatalf("SaveRunState failed: %v", err)
	}

	loaded, err := LoadRunState(statePath)
	if err != nil {
		t.Fatalf("LoadRunState failed: %v", err)
	}
	if len(loaded.Learnings) != 1 || loaded.Learnings[0] != "learned something" {
		t.Errorf("expected learnings preserved, got %v", loaded.Learnings)
	}
	if ss, ok := loaded.Stories["US-001"]; !ok || !ss.Passes {
		t.Error("expected US-001 passes=true")
	}
}

func TestLoadRunState_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "run-state.json")
	os.WriteFile(statePath, []byte("not json"), 0644)

	_, err := LoadRunState(statePath)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSaveRunState_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "run-state.json")

	ts := "2024-01-15T10:00:00Z"
	storyID := "US-002"
	original := &RunState{
		StartedAt:      &ts,
		CurrentStoryID: &storyID,
		Learnings:      []string{"l1", "l2"},
		Stories: map[string]*StoryState{
			"US-001": {Passes: true, Retries: 0, LastResult: &LastResult{Commit: "abc", Summary: "done"}},
			"US-002": {Passes: false, Retries: 2, Blocked: true, Notes: "stuck"},
		},
	}

	if err := SaveRunState(statePath, original); err != nil {
		t.Fatalf("SaveRunState failed: %v", err)
	}

	loaded, err := LoadRunState(statePath)
	if err != nil {
		t.Fatalf("LoadRunState failed: %v", err)
	}

	if *loaded.StartedAt != ts {
		t.Errorf("expected startedAt='%s', got '%s'", ts, *loaded.StartedAt)
	}
	if *loaded.CurrentStoryID != storyID {
		t.Errorf("expected currentStoryId='%s', got '%s'", storyID, *loaded.CurrentStoryID)
	}
	if len(loaded.Learnings) != 2 {
		t.Errorf("expected 2 learnings, got %d", len(loaded.Learnings))
	}
	if !loaded.Stories["US-001"].Passes {
		t.Error("expected US-001 passes=true")
	}
	if !loaded.Stories["US-002"].Blocked {
		t.Error("expected US-002 blocked=true")
	}
}

func TestLoadWorkingPRD_BothFiles(t *testing.T) {
	dir := t.TempDir()
	prdPath := writeV3PRD(t, dir)
	statePath := filepath.Join(dir, "run-state.json")

	state := &RunState{
		Learnings: []string{"l1"},
		Stories: map[string]*StoryState{
			"US-001": {Passes: true, Retries: 1, LastResult: &LastResult{Commit: "abc"}},
		},
	}
	SaveRunState(statePath, state)

	wprd, err := LoadWorkingPRD(prdPath, statePath)
	if err != nil {
		t.Fatalf("LoadWorkingPRD failed: %v", err)
	}
	prd := wprd.PRD()

	if prd.Project != "TestProject" {
		t.Errorf("expected project='TestProject', got '%s'", prd.Project)
	}
	if len(prd.Run.Learnings) != 1 {
		t.Errorf("expected 1 learning, got %d", len(prd.Run.Learnings))
	}

	// US-001 should have merged state
	s1 := GetStoryByID(prd, "US-001")
	if !s1.Passes {
		t.Error("expected US-001 passes=true from state")
	}
	if s1.Retries != 1 {
		t.Errorf("expected US-001 retries=1, got %d", s1.Retries)
	}

	// US-002 should have zero state (no state entry)
	s2 := GetStoryByID(prd, "US-002")
	if s2.Passes {
		t.Error("expected US-002 passes=false (no state)")
	}
}

func TestLoadWorkingPRD_StateMissing(t *testing.T) {
	dir := t.TempDir()
	prdPath := writeV3PRD(t, dir)
	statePath := filepath.Join(dir, "run-state.json") // does not exist

	wprd, err := LoadWorkingPRD(prdPath, statePath)
	if err != nil {
		t.Fatalf("LoadWorkingPRD failed: %v", err)
	}
	prd := wprd.PRD()

	if len(prd.UserStories) != 2 {
		t.Errorf("expected 2 stories, got %d", len(prd.UserStories))
	}
	// All stories should be zero state
	for _, s := range prd.UserStories {
		if s.Passes || s.Blocked || s.Retries != 0 {
			t.Errorf("expected zero state for %s, got passes=%v blocked=%v retries=%d", s.ID, s.Passes, s.Blocked, s.Retries)
		}
	}
}

func TestLoadWorkingPRD_SaveState(t *testing.T) {
	dir := t.TempDir()
	prdPath := writeV3PRD(t, dir)
	statePath := filepath.Join(dir, "run-state.json")

	wprd, err := LoadWorkingPRD(prdPath, statePath)
	if err != nil {
		t.Fatalf("LoadWorkingPRD failed: %v", err)
	}
	prd := wprd.PRD()

	// Modify state through prd methods
	prd.MarkStoryPassed("US-001", "commit123", "done")
	prd.AddLearning("new learning")

	if err := wprd.SaveState(); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	// Verify run-state.json was written
	loaded, err := LoadRunState(statePath)
	if err != nil {
		t.Fatalf("LoadRunState failed: %v", err)
	}
	if !loaded.Stories["US-001"].Passes {
		t.Error("expected US-001 passes=true in saved state")
	}
	if len(loaded.Learnings) != 1 || loaded.Learnings[0] != "new learning" {
		t.Errorf("expected learning saved, got %v", loaded.Learnings)
	}

	// Verify prd.json was NOT modified (still v3 definition only)
	def, err := LoadPRDDefinition(prdPath)
	if err != nil {
		t.Fatalf("LoadPRDDefinition failed after SaveState: %v", err)
	}
	if def.SchemaVersion != 3 {
		t.Errorf("expected prd.json still v3, got %d", def.SchemaVersion)
	}
}

func TestReconcileState_OrphanedStory(t *testing.T) {
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-001"},
			{ID: "US-002"},
		},
	}
	state := &RunState{
		Stories: map[string]*StoryState{
			"US-001": {Passes: true},
			"US-003": {Passes: true}, // orphaned — removed from PRD
		},
	}

	orphaned := ReconcileState(def, state)
	if len(orphaned) != 1 || orphaned[0] != "US-003" {
		t.Errorf("expected orphaned=[US-003], got %v", orphaned)
	}
}

func TestReconcileState_NoOrphans(t *testing.T) {
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-001"},
			{ID: "US-002"},
		},
	}
	state := &RunState{
		Stories: map[string]*StoryState{
			"US-001": {Passes: true},
		},
	}

	orphaned := ReconcileState(def, state)
	if len(orphaned) != 0 {
		t.Errorf("expected no orphans, got %v", orphaned)
	}
}

func TestReconcileState_NewStory(t *testing.T) {
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-001"},
			{ID: "US-002"},
			{ID: "US-003"}, // new story, no state yet
		},
	}
	state := &RunState{
		Stories: map[string]*StoryState{
			"US-001": {Passes: true},
		},
	}

	orphaned := ReconcileState(def, state)
	if len(orphaned) != 0 {
		t.Errorf("expected no orphans for new story, got %v", orphaned)
	}
}


