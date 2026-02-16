package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetNextStory_PicksHighestPriority(t *testing.T) {
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-003", Priority: 3},
			{ID: "US-001", Priority: 1},
			{ID: "US-002", Priority: 2},
		},
	}
	state := NewRunState()

	next := GetNextStory(def, state)
	if next == nil {
		t.Fatal("expected next story, got nil")
	}
	if next.ID != "US-001" {
		t.Errorf("expected US-001, got %s", next.ID)
	}
}

func TestGetNextStory_SkipsPassedStories(t *testing.T) {
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-001", Priority: 1},
			{ID: "US-002", Priority: 2},
			{ID: "US-003", Priority: 3},
		},
	}
	state := NewRunState()
	state.MarkPassed("US-001")

	next := GetNextStory(def, state)
	if next == nil {
		t.Fatal("expected next story, got nil")
	}
	if next.ID != "US-002" {
		t.Errorf("expected US-002, got %s", next.ID)
	}
}

func TestGetNextStory_SkipsSkippedStories(t *testing.T) {
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-001", Priority: 1},
			{ID: "US-002", Priority: 2},
			{ID: "US-003", Priority: 3},
		},
	}
	state := NewRunState()
	state.MarkSkipped("US-001", "too hard")

	next := GetNextStory(def, state)
	if next == nil {
		t.Fatal("expected next story, got nil")
	}
	if next.ID != "US-002" {
		t.Errorf("expected US-002, got %s", next.ID)
	}
}

func TestGetNextStory_ReturnsNilWhenAllPass(t *testing.T) {
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-001", Priority: 1},
			{ID: "US-002", Priority: 2},
		},
	}
	state := NewRunState()
	state.MarkPassed("US-001")
	state.MarkPassed("US-002")

	next := GetNextStory(def, state)
	if next != nil {
		t.Errorf("expected nil, got %s", next.ID)
	}
}

func TestAllComplete_True(t *testing.T) {
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-001"},
			{ID: "US-002"},
		},
	}
	state := NewRunState()
	state.MarkPassed("US-001")
	state.MarkPassed("US-002")

	if !AllComplete(def, state) {
		t.Error("expected all stories complete")
	}
}

func TestAllComplete_False(t *testing.T) {
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-001"},
			{ID: "US-002"},
		},
	}
	state := NewRunState()
	state.MarkPassed("US-001")

	if AllComplete(def, state) {
		t.Error("expected not all stories complete")
	}
}

func TestAllComplete_SkippedCountsAsComplete(t *testing.T) {
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-001"},
			{ID: "US-002"},
		},
	}
	state := NewRunState()
	state.MarkPassed("US-001")
	state.MarkSkipped("US-002", "impossible")

	if !AllComplete(def, state) {
		t.Error("expected all stories complete (skipped counts as done)")
	}
}

func TestIsUIStory(t *testing.T) {
	uiStory := &StoryDefinition{Tags: []string{"ui"}}
	if !IsUIStory(uiStory) {
		t.Error("expected story with 'ui' tag to be UI story")
	}

	nonUIStory := &StoryDefinition{Tags: []string{"backend"}}
	if IsUIStory(nonUIStory) {
		t.Error("expected story without 'ui' tag to not be UI story")
	}

	emptyTags := &StoryDefinition{Tags: []string{}}
	if IsUIStory(emptyTags) {
		t.Error("expected story with empty tags to not be UI story")
	}
}

func TestMarkFailed(t *testing.T) {
	state := NewRunState()
	state.MarkFailed("US-001", "test failure", 3)

	if state.GetRetries("US-001") != 1 {
		t.Errorf("expected retries=1, got %d", state.GetRetries("US-001"))
	}
	if state.GetLastFailure("US-001") != "test failure" {
		t.Errorf("expected lastFailure='test failure', got '%s'", state.GetLastFailure("US-001"))
	}
	if state.IsSkipped("US-001") {
		t.Error("expected not skipped after first failure")
	}
}

func TestMarkFailed_AutoSkip(t *testing.T) {
	state := NewRunState()
	state.Retries["US-001"] = 2

	state.MarkFailed("US-001", "third failure", 3)

	if state.GetRetries("US-001") != 3 {
		t.Errorf("expected retries=3, got %d", state.GetRetries("US-001"))
	}
	if !state.IsSkipped("US-001") {
		t.Error("expected skipped after reaching maxRetries")
	}
}

func TestMarkPassed(t *testing.T) {
	state := NewRunState()
	state.MarkPassed("US-001")

	if !state.IsPassed("US-001") {
		t.Error("expected passed=true")
	}
}

func TestMarkPassed_RemovesFromSkipped(t *testing.T) {
	state := NewRunState()
	state.MarkSkipped("US-001", "was skipped")

	state.MarkPassed("US-001")

	if !state.IsPassed("US-001") {
		t.Error("expected passed=true")
	}
	if state.IsSkipped("US-001") {
		t.Error("expected not skipped after marking passed")
	}
}

func TestMarkSkipped(t *testing.T) {
	state := NewRunState()
	state.MarkSkipped("US-001", "impossible")

	if !state.IsSkipped("US-001") {
		t.Error("expected skipped=true")
	}
	if state.GetLastFailure("US-001") != "impossible" {
		t.Errorf("expected lastFailure='impossible', got '%s'", state.GetLastFailure("US-001"))
	}
}

func TestMarkSkipped_RemovesFromPassed(t *testing.T) {
	state := NewRunState()
	state.MarkPassed("US-001")

	state.MarkSkipped("US-001", "regressed")

	if state.IsPassed("US-001") {
		t.Error("expected not passed after marking skipped")
	}
	if !state.IsSkipped("US-001") {
		t.Error("expected skipped=true")
	}
}

func TestUnmarkPassed(t *testing.T) {
	state := NewRunState()
	state.MarkPassed("US-001")

	state.UnmarkPassed("US-001")

	if state.IsPassed("US-001") {
		t.Error("expected not passed after unmark")
	}
}

func TestAddLearning_Deduplication(t *testing.T) {
	state := NewRunState()

	state.AddLearning("first learning")
	state.AddLearning("second learning")
	state.AddLearning("first learning") // duplicate

	if len(state.Learnings) != 2 {
		t.Errorf("expected 2 learnings (deduplicated), got %d: %v", len(state.Learnings), state.Learnings)
	}
}

func TestAddLearning_UniqueAdded(t *testing.T) {
	state := NewRunState()

	state.AddLearning("learning A")
	state.AddLearning("learning B")
	state.AddLearning("learning C")

	if len(state.Learnings) != 3 {
		t.Errorf("expected 3 unique learnings, got %d", len(state.Learnings))
	}
}

func TestAddLearning_NormalizedDedup(t *testing.T) {
	state := NewRunState()

	state.AddLearning("Must restart dev server after schema changes")
	state.AddLearning("must restart dev server after schema changes")  // case diff
	state.AddLearning("Must restart dev server after schema changes.") // trailing period

	if len(state.Learnings) != 1 {
		t.Errorf("expected 1 learning (normalized dedup), got %d: %v", len(state.Learnings), state.Learnings)
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

func TestCountPassed(t *testing.T) {
	state := NewRunState()
	state.MarkPassed("US-001")
	state.MarkPassed("US-003")

	if count := CountPassed(state); count != 2 {
		t.Errorf("expected 2 passed, got %d", count)
	}
}

func TestCountSkipped(t *testing.T) {
	state := NewRunState()
	state.MarkSkipped("US-002", "hard")
	state.MarkSkipped("US-003", "impossible")

	if count := CountSkipped(state); count != 2 {
		t.Errorf("expected 2 skipped, got %d", count)
	}
}

func TestGetStoryByID(t *testing.T) {
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-001", Title: "First"},
			{ID: "US-002", Title: "Second"},
		},
	}

	story := GetStoryByID(def, "US-002")
	if story == nil {
		t.Fatal("expected story, got nil")
	}
	if story.Title != "Second" {
		t.Errorf("expected title='Second', got '%s'", story.Title)
	}

	notFound := GetStoryByID(def, "US-999")
	if notFound != nil {
		t.Errorf("expected nil for non-existent ID, got %v", notFound)
	}
}

func TestGetPendingStories(t *testing.T) {
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-001"},
			{ID: "US-002"},
			{ID: "US-003"},
			{ID: "US-004"},
		},
	}
	state := NewRunState()
	state.MarkPassed("US-001")
	state.MarkSkipped("US-003", "impossible")

	pending := GetPendingStories(def, state)
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending, got %d", len(pending))
	}
	if pending[0].ID != "US-002" || pending[1].ID != "US-004" {
		t.Errorf("expected US-002 and US-004, got %s and %s", pending[0].ID, pending[1].ID)
	}
}

func TestGetPendingStories_AllComplete(t *testing.T) {
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-001"},
			{ID: "US-002"},
		},
	}
	state := NewRunState()
	state.MarkPassed("US-001")
	state.MarkSkipped("US-002", "blocked")

	pending := GetPendingStories(def, state)
	if len(pending) != 0 {
		t.Errorf("expected 0 pending, got %d", len(pending))
	}
}

func TestBrowserSteps_InStoryDefinition(t *testing.T) {
	def := &PRDDefinition{
		SchemaVersion: 3,
		Project:       "Test",
		BranchName:    "ralph/test",
		UserStories: []StoryDefinition{
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

	story := GetStoryByID(def, "US-001")
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

// --- v3 definition + flat RunState tests ---

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
	if len(state.Passed) != 0 {
		t.Errorf("expected empty passed, got %d entries", len(state.Passed))
	}
}

func TestLoadRunState_Valid(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "run-state.json")

	state := NewRunState()
	state.Learnings = []string{"learned something"}
	state.MarkPassed("US-001")
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
	if !loaded.IsPassed("US-001") {
		t.Error("expected US-001 passed=true")
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

	original := NewRunState()
	original.Learnings = []string{"l1", "l2"}
	original.MarkPassed("US-001")
	original.MarkFailed("US-002", "stuck", 5)
	original.MarkSkipped("US-003", "impossible")

	if err := SaveRunState(statePath, original); err != nil {
		t.Fatalf("SaveRunState failed: %v", err)
	}

	loaded, err := LoadRunState(statePath)
	if err != nil {
		t.Fatalf("LoadRunState failed: %v", err)
	}

	if len(loaded.Learnings) != 2 {
		t.Errorf("expected 2 learnings, got %d", len(loaded.Learnings))
	}
	if !loaded.IsPassed("US-001") {
		t.Error("expected US-001 passed=true")
	}
	if loaded.GetRetries("US-002") != 1 {
		t.Errorf("expected US-002 retries=1, got %d", loaded.GetRetries("US-002"))
	}
	if !loaded.IsSkipped("US-003") {
		t.Error("expected US-003 skipped=true")
	}
}

func TestLoadRunState_MigrateOldFormat(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "run-state.json")

	// Write old format with "stories" key
	oldState := `{
		"learnings": ["old learning"],
		"stories": {
			"US-001": {"passes": true, "retries": 0},
			"US-002": {"passes": false, "retries": 2, "blocked": true, "notes": "stuck"}
		}
	}`
	os.WriteFile(statePath, []byte(oldState), 0644)

	loaded, err := LoadRunState(statePath)
	if err != nil {
		t.Fatalf("LoadRunState migration failed: %v", err)
	}

	if len(loaded.Learnings) != 1 || loaded.Learnings[0] != "old learning" {
		t.Errorf("expected migrated learnings, got %v", loaded.Learnings)
	}
	if !loaded.IsPassed("US-001") {
		t.Error("expected US-001 migrated to passed")
	}
	if !loaded.IsSkipped("US-002") {
		t.Error("expected US-002 migrated to skipped (was blocked)")
	}
	if loaded.GetRetries("US-002") != 2 {
		t.Errorf("expected US-002 retries=2, got %d", loaded.GetRetries("US-002"))
	}
	if loaded.GetLastFailure("US-002") != "stuck" {
		t.Errorf("expected US-002 lastFailure='stuck', got '%s'", loaded.GetLastFailure("US-002"))
	}
}

func TestNewRunState(t *testing.T) {
	state := NewRunState()
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if state.Retries == nil {
		t.Error("expected non-nil Retries map")
	}
	if state.LastFailure == nil {
		t.Error("expected non-nil LastFailure map")
	}
}

func TestGetRetries_ZeroForUnknown(t *testing.T) {
	state := NewRunState()
	if state.GetRetries("US-999") != 0 {
		t.Errorf("expected 0 retries for unknown story")
	}
}

func TestGetLastFailure_EmptyForUnknown(t *testing.T) {
	state := NewRunState()
	if state.GetLastFailure("US-999") != "" {
		t.Errorf("expected empty lastFailure for unknown story")
	}
}
