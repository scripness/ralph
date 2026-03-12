package main

import (
	"os"
	"path/filepath"
	"strings"
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

func TestLoadPRDDefinition_V2Error(t *testing.T) {
	dir := t.TempDir()
	prdPath := filepath.Join(dir, "prd.json")

	// Write a v2 PRD (schemaVersion: 2)
	v2JSON := `{
		"schemaVersion": 2,
		"project": "Test",
		"branchName": "ralph/test",
		"userStories": [{"id": "US-001", "title": "Test", "acceptanceCriteria": ["Works"]}]
	}`
	os.WriteFile(prdPath, []byte(v2JSON), 0644)

	_, err := LoadPRDDefinition(prdPath)
	if err == nil {
		t.Fatal("expected error for v2 PRD")
	}
	if !strings.Contains(err.Error(), "schema version 2") {
		t.Errorf("expected 'schema version 2' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "re-run 'ralph prd") {
		t.Errorf("expected re-run instructions in error, got: %v", err)
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

func TestIsAttempted_FalseForUnknown(t *testing.T) {
	state := NewRunState()
	if state.IsAttempted("US-999") {
		t.Error("expected IsAttempted=false for unknown story")
	}
}

func TestMarkAttempted(t *testing.T) {
	state := NewRunState()
	state.MarkAttempted("US-001")

	if !state.IsAttempted("US-001") {
		t.Error("expected IsAttempted=true after MarkAttempted")
	}
	if state.IsAttempted("US-002") {
		t.Error("expected IsAttempted=false for unattempted story")
	}
}

func TestMarkAttempted_Idempotent(t *testing.T) {
	state := NewRunState()
	state.MarkAttempted("US-001")
	state.MarkAttempted("US-001")
	state.MarkAttempted("US-001")

	if len(state.Attempted) != 1 {
		t.Errorf("expected 1 entry after idempotent MarkAttempted, got %d", len(state.Attempted))
	}
}

func TestIsAttempted_NilSlice(t *testing.T) {
	// Simulates loading old run-state.json without "attempted" field
	state := &RunState{
		Passed:  []string{},
		Skipped: []string{},
	}
	if state.IsAttempted("US-001") {
		t.Error("expected IsAttempted=false on nil Attempted slice")
	}
}

func TestSaveLoadRunState_AttemptedRoundTrip(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "run-state.json")

	original := NewRunState()
	original.MarkAttempted("US-001")
	original.MarkAttempted("US-003")

	if err := SaveRunState(statePath, original); err != nil {
		t.Fatalf("SaveRunState failed: %v", err)
	}

	loaded, err := LoadRunState(statePath)
	if err != nil {
		t.Fatalf("LoadRunState failed: %v", err)
	}
	if !loaded.IsAttempted("US-001") {
		t.Error("expected US-001 attempted after roundtrip")
	}
	if !loaded.IsAttempted("US-003") {
		t.Error("expected US-003 attempted after roundtrip")
	}
	if loaded.IsAttempted("US-002") {
		t.Error("expected US-002 not attempted after roundtrip")
	}
}

func TestLoadRunState_MissingAttemptedField(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "run-state.json")

	// Write old-format run-state.json without "attempted" field
	oldJSON := `{"passed":["US-001"],"skipped":[]}`
	os.WriteFile(statePath, []byte(oldJSON), 0644)

	loaded, err := LoadRunState(statePath)
	if err != nil {
		t.Fatalf("LoadRunState failed: %v", err)
	}
	if loaded.IsAttempted("US-001") {
		t.Error("expected IsAttempted=false when field missing from JSON")
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

// --- scrip v3: ItemState tests ---

func TestComputeItemState_Empty(t *testing.T) {
	s := ComputeItemState("Setup OAuth2", nil)
	if s.Passed || s.Skipped || s.Attempted {
		t.Error("expected all false for empty events")
	}
	if s.Attempts != 0 {
		t.Errorf("expected 0 attempts, got %d", s.Attempts)
	}
}

func TestComputeItemState_Passed(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressItemStart, Item: "Setup OAuth2", Attempt: 1},
		{Event: ProgressItemDone, Item: "Setup OAuth2", Status: "passed", Commit: "abc123", Learnings: []string{"callback URL matters"}},
	}
	s := ComputeItemState("Setup OAuth2", events)
	if !s.Passed {
		t.Error("expected passed=true")
	}
	if s.Skipped {
		t.Error("expected skipped=false")
	}
	if !s.Attempted {
		t.Error("expected attempted=true")
	}
	if s.Attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", s.Attempts)
	}
	if s.LastCommit != "abc123" {
		t.Errorf("expected commit abc123, got %s", s.LastCommit)
	}
	if len(s.Learnings) != 1 || s.Learnings[0] != "callback URL matters" {
		t.Errorf("unexpected learnings: %v", s.Learnings)
	}
}

func TestComputeItemState_Skipped(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressItemStart, Item: "Hard task", Attempt: 1},
		{Event: ProgressItemStuck, Item: "Hard task", Attempt: 1, Reason: "too complex"},
		{Event: ProgressItemStart, Item: "Hard task", Attempt: 2},
		{Event: ProgressItemStuck, Item: "Hard task", Attempt: 2, Reason: "still too complex"},
		{Event: ProgressItemDone, Item: "Hard task", Status: "skipped"},
	}
	s := ComputeItemState("Hard task", events)
	if s.Passed {
		t.Error("expected passed=false")
	}
	if !s.Skipped {
		t.Error("expected skipped=true")
	}
	if s.Attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", s.Attempts)
	}
	if s.LastFailure != "still too complex" {
		t.Errorf("expected last failure reason, got %q", s.LastFailure)
	}
}

func TestComputeItemState_Regression(t *testing.T) {
	// Item passes, then fails during verify-at-top (stuck), then passes again
	events := []ProgressEvent{
		{Event: ProgressItemStart, Item: "Login flow", Attempt: 1},
		{Event: ProgressItemDone, Item: "Login flow", Status: "passed", Commit: "abc"},
		// verify-at-top detects regression
		{Event: ProgressItemStuck, Item: "Login flow", Reason: "regression: tests failed"},
		{Event: ProgressItemStart, Item: "Login flow", Attempt: 2},
		{Event: ProgressItemDone, Item: "Login flow", Status: "passed", Commit: "def"},
	}
	s := ComputeItemState("Login flow", events)
	if !s.Passed {
		t.Error("expected passed=true after retry")
	}
	if s.LastCommit != "def" {
		t.Errorf("expected commit def, got %s", s.LastCommit)
	}
	if s.Attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", s.Attempts)
	}
}

func TestComputeItemState_IgnoresOtherItems(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressItemStart, Item: "Item A", Attempt: 1},
		{Event: ProgressItemDone, Item: "Item A", Status: "passed"},
		{Event: ProgressItemStart, Item: "Item B", Attempt: 1},
		{Event: ProgressItemStuck, Item: "Item B", Reason: "stuck"},
	}
	sA := ComputeItemState("Item A", events)
	if !sA.Passed {
		t.Error("Item A should be passed")
	}
	sB := ComputeItemState("Item B", events)
	if sB.Passed {
		t.Error("Item B should not be passed")
	}
	if sB.LastFailure != "stuck" {
		t.Errorf("Item B expected failure reason, got %q", sB.LastFailure)
	}
}

func TestComputeItemState_StuckClearsPassed(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressItemStart, Item: "X", Attempt: 1},
		{Event: ProgressItemDone, Item: "X", Status: "passed", Commit: "abc"},
		{Event: ProgressItemStuck, Item: "X", Reason: "regression"},
	}
	s := ComputeItemState("X", events)
	if s.Passed {
		t.Error("expected passed=false after stuck event (regression)")
	}
}

func TestGetNextItem_Basic(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "First"},
			{Title: "Second"},
			{Title: "Third"},
		},
	}
	next := GetNextItem(plan, nil)
	if next == nil || next.Title != "First" {
		t.Errorf("expected First, got %v", next)
	}
}

func TestGetNextItem_SkipsPassed(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "First"},
			{Title: "Second"},
		},
	}
	events := []ProgressEvent{
		{Event: ProgressItemStart, Item: "First", Attempt: 1},
		{Event: ProgressItemDone, Item: "First", Status: "passed"},
	}
	next := GetNextItem(plan, events)
	if next == nil || next.Title != "Second" {
		t.Errorf("expected Second, got %v", next)
	}
}

func TestGetNextItem_SkipsSkipped(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "First"},
			{Title: "Second"},
			{Title: "Third"},
		},
	}
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "First", Status: "skipped"},
	}
	next := GetNextItem(plan, events)
	if next == nil || next.Title != "Second" {
		t.Errorf("expected Second, got %v", next)
	}
}

func TestGetNextItem_AllComplete(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "Only"},
		},
	}
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "Only", Status: "passed"},
	}
	next := GetNextItem(plan, events)
	if next != nil {
		t.Errorf("expected nil when all complete, got %v", next)
	}
}

func TestGetNextItem_RespectsDependencies(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "Setup DB"},
			{Title: "Auth flow", DependsOn: []string{"item 1"}},
			{Title: "Dashboard"},
		},
	}
	// Nothing done yet — Auth flow blocked by Setup DB, but Dashboard is free
	// Actually, GetNextItem returns the first available in plan order.
	// Setup DB has no deps, so it should be returned first.
	next := GetNextItem(plan, nil)
	if next == nil || next.Title != "Setup DB" {
		t.Errorf("expected Setup DB (no deps), got %v", next)
	}

	// After Setup DB passes, Auth flow is unblocked
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "Setup DB", Status: "passed"},
	}
	next = GetNextItem(plan, events)
	if next == nil || next.Title != "Auth flow" {
		t.Errorf("expected Auth flow (dep satisfied), got %v", next)
	}
}

func TestGetNextItem_BlockedByDep(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "First", DependsOn: []string{"item 2"}},
			{Title: "Second", DependsOn: []string{"item 1"}},
		},
	}
	// Circular dep — both blocked. Neither should be returned.
	// Actually, item 1 depends on item 2 and vice versa. Both are blocked.
	next := GetNextItem(plan, nil)
	if next != nil {
		t.Errorf("expected nil for circular deps, got %v", next)
	}
}

func TestGetPendingItems(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "A"},
			{Title: "B"},
			{Title: "C"},
		},
	}
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "A", Status: "passed"},
		{Event: ProgressItemDone, Item: "C", Status: "skipped"},
	}
	pending := GetPendingItems(plan, events)
	if len(pending) != 1 || pending[0].Title != "B" {
		t.Errorf("expected [B] pending, got %v", pending)
	}
}

func TestGetPendingItems_Empty(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{{Title: "Only"}},
	}
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "Only", Status: "passed"},
	}
	pending := GetPendingItems(plan, events)
	if len(pending) != 0 {
		t.Errorf("expected 0 pending, got %d", len(pending))
	}
}

func TestAllItemsComplete_True(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "A"},
			{Title: "B"},
		},
	}
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "A", Status: "passed"},
		{Event: ProgressItemDone, Item: "B", Status: "skipped"},
	}
	if !AllItemsComplete(plan, events) {
		t.Error("expected all items complete")
	}
}

func TestAllItemsComplete_False(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "A"},
			{Title: "B"},
		},
	}
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "A", Status: "passed"},
	}
	if AllItemsComplete(plan, events) {
		t.Error("expected not all items complete")
	}
}

func TestAllItemsComplete_EmptyPlan(t *testing.T) {
	plan := &Plan{}
	if !AllItemsComplete(plan, nil) {
		t.Error("expected empty plan to be complete")
	}
}

func TestCountItemsPassed(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "A", Status: "passed"},
		{Event: ProgressItemDone, Item: "B", Status: "skipped"},
		{Event: ProgressItemDone, Item: "C", Status: "passed"},
	}
	if n := CountItemsPassed(events); n != 2 {
		t.Errorf("expected 2 passed, got %d", n)
	}
}

func TestCountItemsSkipped(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "A", Status: "passed"},
		{Event: ProgressItemDone, Item: "B", Status: "skipped"},
		{Event: ProgressItemDone, Item: "C", Status: "skipped"},
	}
	if n := CountItemsSkipped(events); n != 2 {
		t.Errorf("expected 2 skipped, got %d", n)
	}
}

func TestCountItemsPassed_Regression(t *testing.T) {
	// Item passes, then gets stuck (regression), counts as NOT passed
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "A", Status: "passed"},
		{Event: ProgressItemStuck, Item: "A", Reason: "regression"},
	}
	if n := CountItemsPassed(events); n != 0 {
		t.Errorf("expected 0 passed after regression, got %d", n)
	}
}

func TestCountItemsSkipped_OverriddenByPass(t *testing.T) {
	// Skipped then later passed (manually re-attempted)
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "A", Status: "skipped"},
		{Event: ProgressItemDone, Item: "A", Status: "passed"},
	}
	if n := CountItemsSkipped(events); n != 0 {
		t.Errorf("expected 0 skipped after pass override, got %d", n)
	}
	if n := CountItemsPassed(events); n != 1 {
		t.Errorf("expected 1 passed after pass override, got %d", n)
	}
}

func TestCollectLearnings_FromItemDone(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "A", Status: "passed", Learnings: []string{"learning 1", "learning 2"}},
		{Event: ProgressItemDone, Item: "B", Status: "passed", Learnings: []string{"learning 3"}},
	}
	learnings := CollectLearnings(events)
	if len(learnings) != 3 {
		t.Errorf("expected 3 learnings, got %d: %v", len(learnings), learnings)
	}
}

func TestCollectLearnings_FromLearningEvents(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressLearning, Text: "standalone insight"},
		{Event: ProgressItemDone, Item: "A", Status: "passed", Learnings: []string{"item insight"}},
	}
	learnings := CollectLearnings(events)
	if len(learnings) != 2 {
		t.Errorf("expected 2 learnings, got %d", len(learnings))
	}
}

func TestCollectLearnings_Deduplication(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "A", Learnings: []string{"callback URL must match exactly"}},
		{Event: ProgressLearning, Text: "Callback URL must match exactly."},
		{Event: ProgressItemDone, Item: "B", Learnings: []string{"CALLBACK URL MUST MATCH EXACTLY"}},
	}
	learnings := CollectLearnings(events)
	if len(learnings) != 1 {
		t.Errorf("expected 1 learning (deduplicated), got %d: %v", len(learnings), learnings)
	}
}

func TestCollectLearnings_Empty(t *testing.T) {
	learnings := CollectLearnings(nil)
	if len(learnings) != 0 {
		t.Errorf("expected 0 learnings from nil events, got %d", len(learnings))
	}
}

func TestResolveItemRef_ItemNumber(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "First"},
			{Title: "Second"},
			{Title: "Third"},
		},
	}
	tests := []struct {
		ref  string
		want string
	}{
		{"item 1", "First"},
		{"item 2", "Second"},
		{"Item 3", "Third"},
		{"item 0", ""},  // out of range
		{"item 4", ""},  // out of range
		{"item -1", ""}, // not a valid number
	}
	for _, tt := range tests {
		got := resolveItemRef(tt.ref, plan)
		if got != tt.want {
			t.Errorf("resolveItemRef(%q) = %q, want %q", tt.ref, got, tt.want)
		}
	}
}

func TestResolveItemRef_TitleMatch(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "Setup OAuth2 dependencies"},
			{Title: "Google login flow"},
		},
	}
	tests := []struct {
		ref  string
		want string
	}{
		{"Setup OAuth2 dependencies", "Setup OAuth2 dependencies"},
		{"setup oauth2 dependencies", "Setup OAuth2 dependencies"},
		{"Google", "Google login flow"}, // substring
		{"nonexistent", ""},
	}
	for _, tt := range tests {
		got := resolveItemRef(tt.ref, plan)
		if got != tt.want {
			t.Errorf("resolveItemRef(%q) = %q, want %q", tt.ref, got, tt.want)
		}
	}
}

func TestDepsResolved_NoDeps(t *testing.T) {
	item := &PlanItem{Title: "A"}
	plan := &Plan{Items: []PlanItem{{Title: "A"}}}
	states := map[string]ItemState{"A": {}}
	if !depsResolved(item, plan, states) {
		t.Error("expected true for item with no deps")
	}
}

func TestDepsResolved_DepPassed(t *testing.T) {
	item := &PlanItem{Title: "B", DependsOn: []string{"item 1"}}
	plan := &Plan{Items: []PlanItem{{Title: "A"}, {Title: "B"}}}
	states := map[string]ItemState{
		"A": {Passed: true},
		"B": {},
	}
	if !depsResolved(item, plan, states) {
		t.Error("expected true when dep is passed")
	}
}

func TestDepsResolved_DepNotPassed(t *testing.T) {
	item := &PlanItem{Title: "B", DependsOn: []string{"item 1"}}
	plan := &Plan{Items: []PlanItem{{Title: "A"}, {Title: "B"}}}
	states := map[string]ItemState{
		"A": {Attempted: true},
		"B": {},
	}
	if depsResolved(item, plan, states) {
		t.Error("expected false when dep is not passed")
	}
}

func TestComputeAllItemStates(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "A"},
			{Title: "B"},
		},
	}
	events := []ProgressEvent{
		{Event: ProgressItemStart, Item: "A", Attempt: 1},
		{Event: ProgressItemDone, Item: "A", Status: "passed", Commit: "abc"},
		{Event: ProgressItemStart, Item: "B", Attempt: 1},
		{Event: ProgressItemStuck, Item: "B", Reason: "hard"},
	}
	states := ComputeAllItemStates(plan, events)
	if len(states) != 2 {
		t.Fatalf("expected 2 states, got %d", len(states))
	}
	if !states["A"].Passed {
		t.Error("expected A passed")
	}
	if states["B"].Passed {
		t.Error("expected B not passed")
	}
	if states["B"].LastFailure != "hard" {
		t.Errorf("expected B failure reason, got %q", states["B"].LastFailure)
	}
}
