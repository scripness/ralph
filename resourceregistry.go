package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// ResourceRegistry tracks metadata about cached resources.
type ResourceRegistry struct {
	Repos       map[string]*CachedRepo `json:"repos"`
	TotalSize   int64                  `json:"totalSize"`
	LastCleaned time.Time              `json:"lastCleaned"`
}

// CachedRepo tracks a single cached resource.
type CachedRepo struct {
	URL      string    `json:"url"`
	Branch   string    `json:"branch"`
	Commit   string    `json:"commit"`
	SyncedAt time.Time `json:"syncedAt"`
	Size     int64     `json:"sizeBytes"`
}

const registryFileName = "registry.json"

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

// RemoveRepo removes a repo from the registry.
func (r *ResourceRegistry) RemoveRepo(name string) {
	if r.Repos == nil {
		return
	}
	if existing, ok := r.Repos[name]; ok {
		r.TotalSize -= existing.Size
		delete(r.Repos, name)
	}
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

// RecalculateTotalSize recalculates total size from all repos.
func (r *ResourceRegistry) RecalculateTotalSize() {
	r.TotalSize = 0
	for _, repo := range r.Repos {
		r.TotalSize += repo.Size
	}
}
