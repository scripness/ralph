package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFindFeatureDir_NoMatch(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ralph"), 0755)

	_, err := FindFeatureDir(dir, "auth", false)
	if err == nil {
		t.Error("expected error for no matching feature")
	}
}

func TestFindFeatureDir_CreateNew(t *testing.T) {
	dir := t.TempDir()

	fd, err := FindFeatureDir(dir, "auth", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fd.Feature != "auth" {
		t.Errorf("expected feature='auth', got '%s'", fd.Feature)
	}

	today := time.Now().Format("2006-01-02")
	expectedName := today + "-auth"
	if fd.Name != expectedName {
		t.Errorf("expected name='%s', got '%s'", expectedName, fd.Name)
	}
}

func TestFindFeatureDir_MatchExisting(t *testing.T) {
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	featureDir := filepath.Join(ralphDir, "2024-01-15-auth")
	os.MkdirAll(featureDir, 0755)

	fd, err := FindFeatureDir(dir, "auth", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fd.Feature != "auth" {
		t.Errorf("expected feature='auth', got '%s'", fd.Feature)
	}
	if fd.Path != featureDir {
		t.Errorf("expected path='%s', got '%s'", featureDir, fd.Path)
	}
}

func TestFindFeatureDir_MatchMostRecent(t *testing.T) {
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	os.MkdirAll(filepath.Join(ralphDir, "2024-01-10-auth"), 0755)
	os.MkdirAll(filepath.Join(ralphDir, "2024-01-20-auth"), 0755)
	os.MkdirAll(filepath.Join(ralphDir, "2024-01-15-auth"), 0755)

	fd, err := FindFeatureDir(dir, "auth", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := filepath.Join(ralphDir, "2024-01-20-auth")
	if fd.Path != expected {
		t.Errorf("expected most recent '%s', got '%s'", expected, fd.Path)
	}
}

func TestFindFeatureDir_DetectsPrdFiles(t *testing.T) {
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	featureDir := filepath.Join(ralphDir, "2024-01-15-auth")
	os.MkdirAll(featureDir, 0755)

	// Create prd.md only
	os.WriteFile(filepath.Join(featureDir, "prd.md"), []byte("# PRD"), 0644)

	fd, err := FindFeatureDir(dir, "auth", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fd.HasPrdMd {
		t.Error("expected HasPrdMd=true")
	}
	if fd.HasPrdJson {
		t.Error("expected HasPrdJson=false")
	}

	// Add prd.json
	os.WriteFile(filepath.Join(featureDir, "prd.json"), []byte("{}"), 0644)

	fd, _ = FindFeatureDir(dir, "auth", false)
	if !fd.HasPrdJson {
		t.Error("expected HasPrdJson=true")
	}
}

func TestFindFeatureDir_YYYYMMDDFormat(t *testing.T) {
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	featureDir := filepath.Join(ralphDir, "20240115-auth")
	os.MkdirAll(featureDir, 0755)

	fd, err := FindFeatureDir(dir, "auth", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fd.Feature != "auth" {
		t.Errorf("expected feature='auth', got '%s'", fd.Feature)
	}
	if fd.Path != featureDir {
		t.Errorf("expected path='%s', got '%s'", featureDir, fd.Path)
	}
}

func TestListFeatures(t *testing.T) {
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	os.MkdirAll(filepath.Join(ralphDir, "2024-01-15-auth"), 0755)
	os.MkdirAll(filepath.Join(ralphDir, "2024-01-20-billing"), 0755)
	os.MkdirAll(filepath.Join(ralphDir, "logs"), 0755) // Should be ignored (no date prefix)

	features, err := ListFeatures(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(features) != 2 {
		t.Errorf("expected 2 features, got %d", len(features))
	}

	// Should be sorted by date descending
	if features[0].Feature != "billing" {
		t.Errorf("expected first feature='billing', got '%s'", features[0].Feature)
	}
	if features[1].Feature != "auth" {
		t.Errorf("expected second feature='auth', got '%s'", features[1].Feature)
	}
}

func TestFeatureDir_Paths(t *testing.T) {
	fd := &FeatureDir{
		Path: "/project/.ralph/2024-01-15-auth",
	}

	if fd.PrdMdPath() != "/project/.ralph/2024-01-15-auth/prd.md" {
		t.Errorf("unexpected PrdMdPath: %s", fd.PrdMdPath())
	}
	if fd.PrdJsonPath() != "/project/.ralph/2024-01-15-auth/prd.json" {
		t.Errorf("unexpected PrdJsonPath: %s", fd.PrdJsonPath())
	}
}

func TestFindFeatureDir_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	featureDir := filepath.Join(ralphDir, "2024-01-15-auth")
	os.MkdirAll(featureDir, 0755)

	cases := []string{"Auth", "AUTH", "auth", "AuTh"}
	for _, name := range cases {
		fd, err := FindFeatureDir(dir, name, false)
		if err != nil {
			t.Errorf("FindFeatureDir(%q) unexpected error: %v", name, err)
			continue
		}
		if fd.Path != featureDir {
			t.Errorf("FindFeatureDir(%q) = %q, want %q", name, fd.Path, featureDir)
		}
	}
}

func TestFeatureDir_EnsureExists(t *testing.T) {
	dir := t.TempDir()
	fd := &FeatureDir{
		Path: filepath.Join(dir, ".ralph", "2024-01-15-auth"),
	}

	if fileExists(fd.Path) {
		t.Error("directory should not exist yet")
	}

	err := fd.EnsureExists()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fileExists(fd.Path) {
		t.Error("directory should exist after EnsureExists")
	}
}

func TestFeatureDir_RunStatePath(t *testing.T) {
	fd := &FeatureDir{
		Path: "/project/.ralph/2024-01-15-auth",
	}

	expected := "/project/.ralph/2024-01-15-auth/run-state.json"
	if got := fd.RunStatePath(); got != expected {
		t.Errorf("RunStatePath() = %q, want %q", got, expected)
	}
}

// helper to write a run-state.json in a feature directory
func writeTestRunState(t *testing.T, featureDir string, state *RunState) {
	t.Helper()
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featureDir, "run-state.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestCollectCrossFeatureLearnings(t *testing.T) {
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")

	// Feature A: has learnings + passed stories
	featureA := filepath.Join(ralphDir, "2024-01-10-auth")
	os.MkdirAll(featureA, 0755)
	stateA := NewRunState()
	stateA.MarkPassed("US-001")
	stateA.Learnings = []string{"Use bcrypt for passwords", "Always validate input"}
	writeTestRunState(t, featureA, stateA)

	// Feature B: has learnings + passed stories, with one duplicate from A
	featureB := filepath.Join(ralphDir, "2024-01-20-billing")
	os.MkdirAll(featureB, 0755)
	stateB := NewRunState()
	stateB.MarkPassed("US-010")
	stateB.Learnings = []string{"Stripe requires idempotency keys", "use bcrypt for passwords"} // dup of A
	writeTestRunState(t, featureB, stateB)

	result := CollectCrossFeatureLearnings(dir, "search") // exclude "search" (doesn't exist, fine)

	if len(result) != 3 {
		t.Fatalf("expected 3 deduplicated learnings, got %d: %v", len(result), result)
	}

	// Most recent first (billing is 2024-01-20, auth is 2024-01-10)
	if result[0] != "Stripe requires idempotency keys" {
		t.Errorf("expected first learning from most recent feature, got %q", result[0])
	}

	// The duplicate "use bcrypt for passwords" should be skipped, so the third
	// should be "Always validate input" from auth
	found := false
	for _, l := range result {
		if l == "Always validate input" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'Always validate input' from auth feature in results")
	}
}

func TestCollectCrossFeatureLearnings_ExcludesCurrent(t *testing.T) {
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")

	featureA := filepath.Join(ralphDir, "2024-01-10-auth")
	os.MkdirAll(featureA, 0755)
	stateA := NewRunState()
	stateA.MarkPassed("US-001")
	stateA.Learnings = []string{"Should not appear"}
	writeTestRunState(t, featureA, stateA)

	result := CollectCrossFeatureLearnings(dir, "auth")
	if len(result) != 0 {
		t.Errorf("expected 0 learnings when only feature is excluded, got %d", len(result))
	}
}

func TestCollectCrossFeatureLearnings_SkipsNoPassedStories(t *testing.T) {
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")

	// Feature with learnings but no passed stories (abandoned)
	featureA := filepath.Join(ralphDir, "2024-01-10-abandoned")
	os.MkdirAll(featureA, 0755)
	stateA := NewRunState()
	stateA.Learnings = []string{"From abandoned feature"}
	writeTestRunState(t, featureA, stateA)

	// Feature with passed stories and learnings
	featureB := filepath.Join(ralphDir, "2024-01-20-complete")
	os.MkdirAll(featureB, 0755)
	stateB := NewRunState()
	stateB.MarkPassed("US-001")
	stateB.Learnings = []string{"From complete feature"}
	writeTestRunState(t, featureB, stateB)

	result := CollectCrossFeatureLearnings(dir, "other")

	if len(result) != 1 {
		t.Fatalf("expected 1 learning (skipping abandoned), got %d: %v", len(result), result)
	}
	if result[0] != "From complete feature" {
		t.Errorf("expected learning from complete feature, got %q", result[0])
	}
}

func TestCollectCrossFeatureLearnings_Empty(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ralph"), 0755)

	result := CollectCrossFeatureLearnings(dir, "anything")
	if result != nil {
		t.Errorf("expected nil for empty features, got %v", result)
	}
}

func TestCollectCrossFeatureLearnings_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")

	featureA := filepath.Join(ralphDir, "2024-01-10-auth")
	os.MkdirAll(featureA, 0755)
	stateA := NewRunState()
	stateA.MarkPassed("US-001")
	stateA.Learnings = []string{"Should be excluded"}
	writeTestRunState(t, featureA, stateA)

	// Exclude with different case
	for _, name := range []string{"Auth", "AUTH", "AuTh"} {
		result := CollectCrossFeatureLearnings(dir, name)
		if len(result) != 0 {
			t.Errorf("CollectCrossFeatureLearnings(exclude=%q) returned %d learnings, expected 0", name, len(result))
		}
	}
}

