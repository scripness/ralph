package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FeatureDir represents a feature directory in .ralph/
type FeatureDir struct {
	Name      string    // Full directory name (e.g., "2024-01-15-auth")
	Feature   string    // Feature suffix (e.g., "auth")
	Timestamp time.Time // Parsed datetime
	Path      string    // Full path to directory
	HasPrdMd  bool      // prd.md exists
	HasPrdJson bool     // prd.json exists
}

// FindFeatureDir finds a feature directory by suffix match.
// If multiple matches, returns most recent by datetime prefix.
// If no match and create is true, returns path for new directory.
func FindFeatureDir(projectRoot, feature string, create bool) (*FeatureDir, error) {
	ralphDir := filepath.Join(projectRoot, ".ralph")

	// Ensure .ralph exists
	if _, err := os.Stat(ralphDir); os.IsNotExist(err) {
		if !create {
			return nil, fmt.Errorf(".ralph directory not found - run 'ralph init' first")
		}
		if err := os.MkdirAll(ralphDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create .ralph directory: %w", err)
		}
	}

	// List all directories in .ralph/
	entries, err := os.ReadDir(ralphDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read .ralph directory: %w", err)
	}

	var matches []FeatureDir
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		
		// Skip special directories
		if name == "screenshots" || strings.HasPrefix(name, ".") {
			continue
		}

		// Parse datetime-feature format
		fd := parseFeatureDir(ralphDir, name)
		if fd == nil {
			continue
		}

		// Check if feature suffix matches
		if strings.EqualFold(fd.Feature, feature) {
			matches = append(matches, *fd)
		}
	}

	if len(matches) == 0 {
		if !create {
			return nil, fmt.Errorf("no feature directory found for '%s'", feature)
		}
		// Create new directory with today's date
		return newFeatureDir(ralphDir, feature), nil
	}

	// Sort by timestamp descending (most recent first)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Timestamp.After(matches[j].Timestamp)
	})

	return &matches[0], nil
}

// ListFeatures returns all feature directories
func ListFeatures(projectRoot string) ([]FeatureDir, error) {
	ralphDir := filepath.Join(projectRoot, ".ralph")

	entries, err := os.ReadDir(ralphDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var features []FeatureDir
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "screenshots" || strings.HasPrefix(name, ".") {
			continue
		}
		if fd := parseFeatureDir(ralphDir, name); fd != nil {
			features = append(features, *fd)
		}
	}

	// Sort by timestamp descending
	sort.Slice(features, func(i, j int) bool {
		return features[i].Timestamp.After(features[j].Timestamp)
	})

	return features, nil
}

// parseFeatureDir parses a directory name in format YYYY-MM-DD-feature
func parseFeatureDir(ralphDir, name string) *FeatureDir {
	// Expected format: YYYY-MM-DD-feature or YYYYMMDD-feature
	parts := strings.SplitN(name, "-", 4)
	if len(parts) < 4 {
		// Try YYYYMMDD-feature format
		if len(name) > 9 && name[8] == '-' {
			dateStr := name[:8]
			feature := name[9:]
			if t, err := time.Parse("20060102", dateStr); err == nil {
				path := filepath.Join(ralphDir, name)
				return &FeatureDir{
					Name:       name,
					Feature:    feature,
					Timestamp:  t,
					Path:       path,
					HasPrdMd:   fileExists(filepath.Join(path, "prd.md")),
					HasPrdJson: fileExists(filepath.Join(path, "prd.json")),
				}
			}
		}
		return nil
	}

	// Parse YYYY-MM-DD-feature
	dateStr := parts[0] + "-" + parts[1] + "-" + parts[2]
	feature := parts[3]
	
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return nil
	}

	path := filepath.Join(ralphDir, name)
	return &FeatureDir{
		Name:       name,
		Feature:    feature,
		Timestamp:  t,
		Path:       path,
		HasPrdMd:   fileExists(filepath.Join(path, "prd.md")),
		HasPrdJson: fileExists(filepath.Join(path, "prd.json")),
	}
}

// newFeatureDir creates a new FeatureDir with today's date
func newFeatureDir(ralphDir, feature string) *FeatureDir {
	now := time.Now()
	name := now.Format("2006-01-02") + "-" + feature
	path := filepath.Join(ralphDir, name)
	
	return &FeatureDir{
		Name:       name,
		Feature:    feature,
		Timestamp:  now,
		Path:       path,
		HasPrdMd:   false,
		HasPrdJson: false,
	}
}

// PrdMdPath returns the path to prd.md
func (fd *FeatureDir) PrdMdPath() string {
	return filepath.Join(fd.Path, "prd.md")
}

// PrdJsonPath returns the path to prd.json
func (fd *FeatureDir) PrdJsonPath() string {
	return filepath.Join(fd.Path, "prd.json")
}

// EnsureExists creates the feature directory if it doesn't exist
func (fd *FeatureDir) EnsureExists() error {
	return os.MkdirAll(fd.Path, 0755)
}

// LogsDir returns the path to the logs directory
func (fd *FeatureDir) LogsDir() string {
	return filepath.Join(fd.Path, "logs")
}
