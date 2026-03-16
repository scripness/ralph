package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFindFeatureDir_NoMatch(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".scrip"), 0755)

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
	scripDir := filepath.Join(dir, ".scrip")
	featureDir := filepath.Join(scripDir, "2024-01-15-auth")
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
	scripDir := filepath.Join(dir, ".scrip")
	os.MkdirAll(filepath.Join(scripDir, "2024-01-10-auth"), 0755)
	os.MkdirAll(filepath.Join(scripDir, "2024-01-20-auth"), 0755)
	os.MkdirAll(filepath.Join(scripDir, "2024-01-15-auth"), 0755)

	fd, err := FindFeatureDir(dir, "auth", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := filepath.Join(scripDir, "2024-01-20-auth")
	if fd.Path != expected {
		t.Errorf("expected most recent '%s', got '%s'", expected, fd.Path)
	}
}


func TestFindFeatureDir_YYYYMMDDFormat(t *testing.T) {
	dir := t.TempDir()
	scripDir := filepath.Join(dir, ".scrip")
	featureDir := filepath.Join(scripDir, "20240115-auth")
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
	scripDir := filepath.Join(dir, ".scrip")
	os.MkdirAll(filepath.Join(scripDir, "2024-01-15-auth"), 0755)
	os.MkdirAll(filepath.Join(scripDir, "2024-01-20-billing"), 0755)
	os.MkdirAll(filepath.Join(scripDir, "logs"), 0755) // Should be ignored (no date prefix)

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


func TestFindFeatureDir_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	scripDir := filepath.Join(dir, ".scrip")
	featureDir := filepath.Join(scripDir, "2024-01-15-auth")
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
		Path: filepath.Join(dir, ".scrip", "2024-01-15-auth"),
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

func TestSummaryMdPath(t *testing.T) {
	fd := &FeatureDir{Path: "/project/.scrip/2024-01-15-auth"}
	got := fd.SummaryMdPath()
	want := "/project/.scrip/2024-01-15-auth/summary.md"
	if got != want {
		t.Errorf("SummaryMdPath() = %q, want %q", got, want)
	}
}

func TestLoadFeatureSummary_Missing(t *testing.T) {
	dir := t.TempDir()
	fd := &FeatureDir{Path: dir}
	result := LoadFeatureSummary(fd)
	if result != "" {
		t.Errorf("expected empty string for missing summary, got %q", result)
	}
}

func TestLoadFeatureSummary_Exists(t *testing.T) {
	dir := t.TempDir()
	content := "## auth (2026-02-25)\n\nSummary content."
	os.WriteFile(filepath.Join(dir, "summary.md"), []byte(content), 0644)

	fd := &FeatureDir{Path: dir}
	result := LoadFeatureSummary(fd)
	if result != content {
		t.Errorf("LoadFeatureSummary() = %q, want %q", result, content)
	}
}

func TestFeatureArchived_SummaryExists(t *testing.T) {
	dir := t.TempDir()
	featureDir := filepath.Join(dir, ".scrip", "2024-01-15-auth")
	os.MkdirAll(featureDir, 0755)

	fd := &FeatureDir{Path: featureDir, Feature: "auth"}

	// No summary.md — not archived
	if fileExists(fd.SummaryMdPath()) {
		t.Error("expected not archived when no summary.md")
	}

	// Write summary.md — archived
	os.WriteFile(fd.SummaryMdPath(), []byte("## auth (2026-02-25)\n\nAuth summary."), 0644)
	if !fileExists(fd.SummaryMdPath()) {
		t.Error("expected archived when summary.md exists")
	}
}


