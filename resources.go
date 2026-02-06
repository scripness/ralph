package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ResourcesConfig for ralph.config.json
type ResourcesConfig struct {
	Enabled  *bool      `json:"enabled,omitempty"`  // default: true
	CacheDir string     `json:"cacheDir,omitempty"` // default: ~/.ralph/resources
	Custom   []Resource `json:"custom,omitempty"`   // user overrides/additions
}

// IsEnabled returns whether resources are enabled (defaults to true).
func (c *ResourcesConfig) IsEnabled() bool {
	if c == nil || c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

// GetCacheDir returns the cache directory, expanding ~ to home.
func (c *ResourcesConfig) GetCacheDir() string {
	if c == nil || c.CacheDir == "" {
		return DefaultResourcesCacheDir()
	}
	return expandHomePath(c.CacheDir)
}

// DefaultResourcesCacheDir returns the default cache directory.
func DefaultResourcesCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".ralph/resources"
	}
	return filepath.Join(home, ".ralph", "resources")
}

// expandHomePath expands ~ to the user's home directory.
func expandHomePath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// ResourceManager handles source code repo lifecycle.
type ResourceManager struct {
	cacheDir     string
	resources    []Resource           // all available resources (defaults + custom)
	detected     map[string]*Resource // detected dependency name -> resource
	registry     *ResourceRegistry
	detectedDeps []string // original dependency names from project
}

// NewResourceManager creates a manager for detected dependencies.
func NewResourceManager(cfg *ResourcesConfig, detectedDeps []string) *ResourceManager {
	cacheDir := DefaultResourcesCacheDir()
	if cfg != nil && cfg.CacheDir != "" {
		cacheDir = cfg.GetCacheDir()
	}

	// Merge default resources with custom
	var custom []Resource
	if cfg != nil {
		custom = cfg.Custom
	}
	resources := MergeWithCustom(custom)

	// Map detected dependencies to resources
	detected := make(map[string]*Resource)
	for _, dep := range detectedDeps {
		if r := MapDependencyToResource(dep, resources); r != nil {
			// Use the resource name as key to dedupe
			detected[r.Name] = r
		}
	}

	return &ResourceManager{
		cacheDir:     cacheDir,
		resources:    resources,
		detected:     detected,
		detectedDeps: detectedDeps,
	}
}

// loadRegistry lazily loads the registry.
func (rm *ResourceManager) loadRegistry() error {
	if rm.registry != nil {
		return nil
	}
	reg, err := LoadResourceRegistry(rm.cacheDir)
	if err != nil {
		return err
	}
	rm.registry = reg
	return nil
}

// EnsureResources clones/syncs all resources for detected dependencies.
// Called automatically before ralph run.
func (rm *ResourceManager) EnsureResources() error {
	if len(rm.detected) == 0 {
		return nil
	}

	if err := rm.loadRegistry(); err != nil {
		return fmt.Errorf("failed to load resource registry: %w", err)
	}

	var synced, cloned, failed int
	for name, r := range rm.detected {
		repoPath := filepath.Join(rm.cacheDir, name)
		gitOps := NewExternalGitOps(repoPath, r.URL, r.Branch)

		if gitOps.Exists() {
			// Already cloned - sync if needed
			upToDate, err := gitOps.IsUpToDate()
			if err != nil {
				fmt.Printf("  Warning: could not check %s for updates: %v\n", name, err)
				continue
			}
			if !upToDate {
				fmt.Printf("  Syncing %s...\n", name)
				if err := gitOps.Pull(); err != nil {
					fmt.Printf("  Warning: failed to sync %s: %v\n", name, err)
					failed++
					continue
				}
				synced++
			}
		} else {
			// Clone new
			fmt.Printf("  Cloning %s...\n", name)
			if err := gitOps.Clone(true); err != nil {
				fmt.Printf("  Warning: failed to clone %s: %v\n", name, err)
				failed++
				continue
			}
			cloned++
		}

		// Update registry
		size, _ := gitOps.GetRepoSize()
		rm.registry.UpdateRepo(name, &CachedRepo{
			URL:      r.URL,
			Branch:   r.Branch,
			Commit:   gitOps.GetCurrentCommitShort(),
			SyncedAt: time.Now(),
			Size:     size,
		})
	}

	// Save registry
	if err := rm.registry.Save(rm.cacheDir); err != nil {
		fmt.Printf("  Warning: failed to save resource registry: %v\n", err)
	}

	if cloned > 0 || synced > 0 {
		fmt.Printf("  Resources: %d cloned, %d synced", cloned, synced)
		if failed > 0 {
			fmt.Printf(", %d failed", failed)
		}
		fmt.Println()
	}

	return nil
}

// SyncResource updates a single resource to latest.
// Checks if up-to-date via ls-remote (fast), pulls only if behind.
func (rm *ResourceManager) SyncResource(name string) error {
	if err := rm.loadRegistry(); err != nil {
		return err
	}

	// Find resource
	r := GetResourceByName(name, rm.resources)
	if r == nil {
		return fmt.Errorf("unknown resource: %s", name)
	}

	repoPath := filepath.Join(rm.cacheDir, name)
	gitOps := NewExternalGitOps(repoPath, r.URL, r.Branch)

	if !gitOps.Exists() {
		// Clone if not present
		if err := gitOps.Clone(true); err != nil {
			return fmt.Errorf("failed to clone %s: %w", name, err)
		}
	} else {
		// Sync if needed
		upToDate, err := gitOps.IsUpToDate()
		if err != nil {
			return fmt.Errorf("failed to check %s: %w", name, err)
		}
		if !upToDate {
			if err := gitOps.Pull(); err != nil {
				return fmt.Errorf("failed to sync %s: %w", name, err)
			}
		}
	}

	// Update registry
	size, _ := gitOps.GetRepoSize()
	rm.registry.UpdateRepo(name, &CachedRepo{
		URL:      r.URL,
		Branch:   r.Branch,
		Commit:   gitOps.GetCurrentCommitShort(),
		SyncedAt: time.Now(),
		Size:     size,
	})

	return rm.registry.Save(rm.cacheDir)
}

// GetResourcePath returns the local path to a cached resource.
func (rm *ResourceManager) GetResourcePath(name string) string {
	return filepath.Join(rm.cacheDir, name)
}

// ListCached returns names of all cached resources.
func (rm *ResourceManager) ListCached() ([]string, error) {
	if err := rm.loadRegistry(); err != nil {
		return nil, err
	}
	names := rm.registry.ListCached()
	sort.Strings(names)
	return names, nil
}

// ListDetected returns names of detected resources (may not be cached yet).
func (rm *ResourceManager) ListDetected() []string {
	var names []string
	for name := range rm.detected {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ClearCache removes all cached resources.
func (rm *ResourceManager) ClearCache() error {
	entries, err := os.ReadDir(rm.cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.Name() == registryFileName {
			continue // Keep registry file until the end
		}
		path := filepath.Join(rm.cacheDir, entry.Name())
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("failed to remove %s: %w", entry.Name(), err)
		}
	}

	// Clear registry
	rm.registry = &ResourceRegistry{
		Repos:       make(map[string]*CachedRepo),
		LastCleaned: time.Now(),
	}
	return rm.registry.Save(rm.cacheDir)
}

// GetCacheSize returns total size of cached resources.
func (rm *ResourceManager) GetCacheSize() (int64, error) {
	if err := rm.loadRegistry(); err != nil {
		return 0, err
	}
	return rm.registry.TotalSize, nil
}

// GetCacheDir returns the cache directory path.
func (rm *ResourceManager) GetCacheDir() string {
	return rm.cacheDir
}

// GetCachedRepoInfo returns info about a cached repo.
func (rm *ResourceManager) GetCachedRepoInfo(name string) *CachedRepo {
	if err := rm.loadRegistry(); err != nil {
		return nil
	}
	return rm.registry.GetRepo(name)
}

// HasDetectedResources returns true if any dependencies map to known resources.
func (rm *ResourceManager) HasDetectedResources() bool {
	return len(rm.detected) > 0
}

// FormatSize formats bytes as human-readable string.
func FormatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1fGB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1fMB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1fKB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
