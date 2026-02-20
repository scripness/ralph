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
	Enabled  *bool  `json:"enabled,omitempty"`  // default: true
	CacheDir string `json:"cacheDir,omitempty"` // default: ~/.ralph/resources
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
	cacheDir    string
	detected    map[string]*Resource // key: "name@version" → resource
	registry    *ResourceRegistry
	ecosystem   string
	projectRoot string
}

// NewResourceManager creates a manager for detected dependencies.
// It resolves repo URLs and exact versions for all dependencies.
func NewResourceManager(cfg *ResourcesConfig, deps []Dependency, ecosystem, projectRoot string) *ResourceManager {
	cacheDir := DefaultResourcesCacheDir()
	if cfg != nil && cfg.CacheDir != "" {
		cacheDir = cfg.GetCacheDir()
	}

	rm := &ResourceManager{
		cacheDir:    cacheDir,
		detected:    make(map[string]*Resource),
		ecosystem:   ecosystem,
		projectRoot: projectRoot,
	}

	if len(deps) == 0 {
		return rm
	}

	// Load registry for resolution caching
	reg, err := LoadResourceRegistry(cacheDir)
	if err != nil {
		fmt.Printf("  Warning: failed to load resource registry: %v\n", err)
		reg = &ResourceRegistry{Repos: make(map[string]*CachedRepo)}
	}
	rm.registry = reg

	// Resolve all dependencies
	resolved := ResolveAll(deps, ecosystem, projectRoot, reg)

	for _, r := range resolved {
		key := r.Name + "@" + r.Version
		rm.detected[key] = &Resource{
			Name:    r.Name,
			URL:     r.URL,
			Branch:  r.Tag, // tag or empty (default branch)
			Version: r.Version,
		}
	}

	// Save registry (URL cache + unresolvable markers)
	if err := reg.Save(cacheDir); err != nil {
		fmt.Printf("  Warning: failed to save resource registry: %v\n", err)
	}

	return rm
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
func (rm *ResourceManager) EnsureResources() error {
	if len(rm.detected) == 0 {
		return nil
	}

	if err := rm.loadRegistry(); err != nil {
		return fmt.Errorf("failed to load resource registry: %w", err)
	}

	var synced, cloned, failed int
	for key, r := range rm.detected {
		repoPath := filepath.Join(rm.cacheDir, key) // name@version
		ref := r.Branch
		if ref == "" {
			ref = "main" // default branch fallback
		}
		gitOps := NewExternalGitOps(repoPath, r.URL, ref)

		if gitOps.Exists() {
			// Already cloned — for version-pinned repos, no sync needed
			// (the tag content doesn't change)
			if r.Version != "" && r.Branch != "" {
				// Tag-based clone: content is immutable, skip sync
				continue
			}
			// Default branch clone: check for updates
			upToDate, err := gitOps.IsUpToDate()
			if err != nil {
				fmt.Printf("  Warning: could not check %s for updates: %v\n", key, err)
				continue
			}
			if !upToDate {
				fmt.Printf("  Syncing %s...\n", key)
				if err := gitOps.Pull(); err != nil {
					fmt.Printf("  Warning: failed to sync %s: %v\n", key, err)
					failed++
					continue
				}
				synced++
			}
		} else {
			// Clone new
			fmt.Printf("  Cloning %s...\n", key)

			// If no tag was found during resolution, try to find one now
			if r.Branch == "" && r.Version != "" {
				tag := findVersionTag(r.URL, r.Version)
				if tag != "" {
					r.Branch = tag
					gitOps = NewExternalGitOps(repoPath, r.URL, tag)
				} else {
					fmt.Printf("  Warning: no version tag found for %s v%s, using default branch\n", r.Name, r.Version)
				}
			}

			if err := gitOps.Clone(true); err != nil {
				fmt.Printf("  Warning: failed to clone %s: %v\n", key, err)
				failed++
				continue
			}
			cloned++
		}

		// Update registry
		size, _ := gitOps.GetRepoSize()
		rm.registry.UpdateRepo(key, &CachedRepo{
			URL:      r.URL,
			Tag:      r.Branch,
			Version:  r.Version,
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
		cached := len(rm.detected) - cloned - synced - failed
		fmt.Printf("  Resources: %d cloned, %d synced, %d cached", cloned, synced, cached)
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

// ListDetected returns names of detected resources (may not be cached yet).
func (rm *ResourceManager) ListDetected() []string {
	var names []string
	for key := range rm.detected {
		names = append(names, key)
	}
	sort.Strings(names)
	return names
}

// GetCacheDir returns the cache directory path.
func (rm *ResourceManager) GetCacheDir() string {
	return rm.cacheDir
}

// HasDetectedResources returns true if any dependencies were resolved.
func (rm *ResourceManager) HasDetectedResources() bool {
	return len(rm.detected) > 0
}

// CachedResource represents a framework resource that is cached locally.
type CachedResource struct {
	Name    string // e.g., "next", "react"
	Version string // e.g., "15.0.0"
	Path    string // local filesystem path to cached source
	URL     string // source repo URL
	Ref     string // tag or branch name
	Commit  string // current commit hash (short)
}

// GetCachedResources returns all detected resources that are actually cached on disk.
func (rm *ResourceManager) GetCachedResources() []CachedResource {
	if err := rm.loadRegistry(); err != nil {
		return nil
	}

	var cached []CachedResource
	for key, r := range rm.detected {
		repoPath := filepath.Join(rm.cacheDir, key) // name@version
		if _, err := os.Stat(repoPath); err != nil {
			continue // not cached
		}
		commit := ""
		if repo := rm.registry.GetRepo(key); repo != nil {
			commit = repo.Commit
		}
		cached = append(cached, CachedResource{
			Name:    r.Name,
			Version: r.Version,
			Path:    repoPath,
			URL:     r.URL,
			Ref:     r.Branch,
			Commit:  commit,
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

	ecosystem := ecosystemFromTechStack(codebaseCtx.TechStack)
	rm := NewResourceManager(cfg.Config.Resources, codebaseCtx.Dependencies, ecosystem, cfg.ProjectRoot)
	if !rm.HasDetectedResources() {
		return rm
	}

	detected := rm.ListDetected()
	fmt.Printf("  Resolving %d dependencies...\n", len(detected))
	if err := rm.EnsureResources(); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: resource sync failed: %s\n", err.Error())
	}

	return rm
}
