package main

// Resource maps a framework/library to its source code repository
type Resource struct {
	Name    string // e.g., "nextjs", "react", "svelte"
	URL     string // GitHub repo URL (full source)
	Branch  string // e.g., "main", "canary" (legacy, used as ref)
	Version string // resolved exact version (e.g., "3.24.4")
}
