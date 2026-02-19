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
		cacheDir:  cacheDir,
		resources: resources,
		detected:  detected,
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

// GetCacheDir returns the cache directory path.
func (rm *ResourceManager) GetCacheDir() string {
	return rm.cacheDir
}

// HasDetectedResources returns true if any dependencies map to known resources.
func (rm *ResourceManager) HasDetectedResources() bool {
	return len(rm.detected) > 0
}

// CachedResource represents a framework resource that is cached locally.
type CachedResource struct {
	Name     string // e.g., "next", "react"
	Path     string // local filesystem path to cached source
	URL      string // source repo URL
	Branch   string // branch name
	Commit   string // current commit hash (short)
}

// GetCachedResources returns all detected resources that are actually cached on disk.
func (rm *ResourceManager) GetCachedResources() []CachedResource {
	if err := rm.loadRegistry(); err != nil {
		return nil
	}

	var cached []CachedResource
	for name, r := range rm.detected {
		repoPath := filepath.Join(rm.cacheDir, name)
		if _, err := os.Stat(repoPath); err != nil {
			continue // not cached
		}
		commit := ""
		if repo := rm.registry.GetRepo(name); repo != nil {
			commit = repo.Commit
		}
		cached = append(cached, CachedResource{
			Name:   name,
			Path:   repoPath,
			URL:    r.URL,
			Branch: r.Branch,
			Commit: commit,
		})
	}

	// Sort for deterministic output
	sort.Slice(cached, func(i, j int) bool {
		return cached[i].Name < cached[j].Name
	})

	return cached
}

// ensureResourceSync syncs framework source code resources.
// Returns nil ResourceManager if resources are disabled or no deps detected.
// Non-fatal: prints warnings on errors but does not fail.
func ensureResourceSync(cfg *ResolvedConfig, codebaseCtx *CodebaseContext) *ResourceManager {
	if cfg.Config.Resources != nil && !cfg.Config.Resources.IsEnabled() {
		return nil
	}

	depNames := GetDependencyNames(codebaseCtx.Dependencies)
	rm := NewResourceManager(cfg.Config.Resources, depNames)
	if !rm.HasDetectedResources() {
		return rm
	}

	detected := rm.ListDetected()
	fmt.Printf("  Syncing %d framework resources (%s)...\n", len(detected), strings.Join(detected, ", "))
	if err := rm.EnsureResources(); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: resource sync failed: %s\n", err.Error())
	}

	return rm
}

