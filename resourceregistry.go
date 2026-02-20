package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// ResourceRegistry tracks metadata about cached resources.
type ResourceRegistry struct {
	Repos        map[string]*CachedRepo    `json:"repos"`
	TotalSize    int64                     `json:"totalSize"`
	Resolved     map[string]*ResolvedEntry `json:"resolved,omitempty"`     // dep name → repo URL + timestamp (30-day expiry)
	Unresolvable map[string]time.Time      `json:"unresolvable,omitempty"` // dep name → last checked (7-day expiry)
}

// ResolvedEntry caches a dep name → repo URL mapping with expiry.
type ResolvedEntry struct {
	URL        string    `json:"url"`
	ResolvedAt time.Time `json:"resolvedAt"`
}

// CachedRepo tracks a single cached resource.
type CachedRepo struct {
	URL      string    `json:"url"`
	Branch   string    `json:"branch,omitempty"` // legacy field, kept for backwards compat
	Tag      string    `json:"tag,omitempty"`    // git tag (e.g., "v15.0.0") or branch name
	Version  string    `json:"version,omitempty"` // clean version (e.g., "15.0.0")
	Commit   string    `json:"commit"`
	SyncedAt time.Time `json:"syncedAt"`
	Size     int64     `json:"sizeBytes"`
}

const registryFileName = "registry.json"

// unresolvableExpiry is how long to cache "unresolvable" status for a dep.
const unresolvableExpiry = 7 * 24 * time.Hour

// resolvedURLExpiry is how long to cache resolved dep→URL mappings.
const resolvedURLExpiry = 30 * 24 * time.Hour

// LoadResourceRegistry loads or creates the registry file.
func LoadResourceRegistry(cacheDir string) (*ResourceRegistry, error) {
	registryPath := filepath.Join(cacheDir, registryFileName)

	// Ensure cache directory exists
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, err
	}

	// Try to load existing registry
	data, err := os.ReadFile(registryPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty registry
			return &ResourceRegistry{
				Repos: make(map[string]*CachedRepo),
			}, nil
		}
		return nil, err
	}

	var registry ResourceRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		// Corrupted registry, start fresh
		return &ResourceRegistry{
			Repos: make(map[string]*CachedRepo),
		}, nil
	}

	if registry.Repos == nil {
		registry.Repos = make(map[string]*CachedRepo)
	}

	return &registry, nil
}

// Save atomically saves the registry.
func (r *ResourceRegistry) Save(cacheDir string) error {
	registryPath := filepath.Join(cacheDir, registryFileName)
	return AtomicWriteJSON(registryPath, r)
}

// UpdateRepo updates metadata for a cached repo.
func (r *ResourceRegistry) UpdateRepo(name string, repo *CachedRepo) {
	if r.Repos == nil {
		r.Repos = make(map[string]*CachedRepo)
	}

	// Update total size
	if existing, ok := r.Repos[name]; ok {
		r.TotalSize -= existing.Size
	}
	r.TotalSize += repo.Size

	r.Repos[name] = repo
}

// GetRepo returns metadata for a cached repo.
func (r *ResourceRegistry) GetRepo(name string) *CachedRepo {
	if r.Repos == nil {
		return nil
	}
	return r.Repos[name]
}

// ListCached returns names of all cached repos.
func (r *ResourceRegistry) ListCached() []string {
	var names []string
	for name := range r.Repos {
		names = append(names, name)
	}
	return names
}

// GetResolvedURL returns the cached repo URL for a dep name, if available.
// Returns false if the entry has expired (30-day TTL).
func (r *ResourceRegistry) GetResolvedURL(name string) (string, bool) {
	if r.Resolved == nil {
		return "", false
	}
	entry, ok := r.Resolved[name]
	if !ok || entry == nil {
		return "", false
	}
	// Check expiry
	if !entry.ResolvedAt.IsZero() && time.Since(entry.ResolvedAt) > resolvedURLExpiry {
		delete(r.Resolved, name)
		return "", false
	}
	return entry.URL, true
}

// SetResolvedURL caches a dep name → repo URL mapping with a timestamp.
func (r *ResourceRegistry) SetResolvedURL(name, url string) {
	if r.Resolved == nil {
		r.Resolved = make(map[string]*ResolvedEntry)
	}
	r.Resolved[name] = &ResolvedEntry{
		URL:        url,
		ResolvedAt: time.Now(),
	}
}

// IsUnresolvable returns true if the dep was recently marked as unresolvable.
func (r *ResourceRegistry) IsUnresolvable(name string) bool {
	if r.Unresolvable == nil {
		return false
	}
	checked, ok := r.Unresolvable[name]
	if !ok {
		return false
	}
	// Expired?
	if time.Since(checked) > unresolvableExpiry {
		delete(r.Unresolvable, name)
		return false
	}
	return true
}

// MarkUnresolvable records that a dep could not be resolved.
func (r *ResourceRegistry) MarkUnresolvable(name string) {
	if r.Unresolvable == nil {
		r.Unresolvable = make(map[string]time.Time)
	}
	r.Unresolvable[name] = time.Now()
}
