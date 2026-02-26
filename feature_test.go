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

func TestSummaryPath(t *testing.T) {
	got := SummaryPath("/project")
	want := "/project/.ralph/summary.md"
	if got != want {
		t.Errorf("SummaryPath() = %q, want %q", got, want)
	}
}

func TestLoadSummary_Missing(t *testing.T) {
	dir := t.TempDir()
	result := LoadSummary(dir)
	if result != "" {
		t.Errorf("expected empty string for missing summary, got %q", result)
	}
}

func TestLoadSummary_Exists(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ralph"), 0755)
	content := "# Feature Summaries\n\n---\n\n## auth (2026-02-25)\n\nSummary content."
	os.WriteFile(filepath.Join(dir, ".ralph", "summary.md"), []byte(content), 0644)

	result := LoadSummary(dir)
	if result != content {
		t.Errorf("LoadSummary() = %q, want %q", result, content)
	}
}

func TestIsFeatureArchived(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ralph"), 0755)

	content := "# Feature Summaries\n\n---\n\n## auth (2026-02-25)\n\nAuth summary.\n\n---\n\n## billing (2026-02-28)\n\nBilling summary.\n"
	os.WriteFile(filepath.Join(dir, ".ralph", "summary.md"), []byte(content), 0644)

	if !isFeatureArchived(dir, "auth") {
		t.Error("expected auth to be archived")
	}
	if !isFeatureArchived(dir, "Auth") {
		t.Error("expected case-insensitive match for Auth")
	}
	if !isFeatureArchived(dir, "billing") {
		t.Error("expected billing to be archived")
	}
	if isFeatureArchived(dir, "search") {
		t.Error("expected search to NOT be archived")
	}
}

func TestIsFeatureArchived_NoSummary(t *testing.T) {
	dir := t.TempDir()
	if isFeatureArchived(dir, "auth") {
		t.Error("expected false when no summary.md exists")
	}
}


