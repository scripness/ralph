package main

import (
	"strings"
)

// Resource maps a framework/library to its source code repository
type Resource struct {
	Name   string // e.g., "nextjs", "react", "svelte"
	URL    string // GitHub repo URL (full source)
	Branch string // e.g., "main", "canary"
}

// DefaultResources is the built-in registry of popular frameworks.
// These are SOURCE CODE repos, not just docs.
var DefaultResources = []Resource{
	// Frontend Frameworks
	{Name: "next", URL: "https://github.com/vercel/next.js", Branch: "canary"},
	{Name: "react", URL: "https://github.com/facebook/react", Branch: "main"},
	{Name: "svelte", URL: "https://github.com/sveltejs/svelte", Branch: "main"},
	{Name: "@sveltejs/kit", URL: "https://github.com/sveltejs/kit", Branch: "main"},
	{Name: "vue", URL: "https://github.com/vuejs/core", Branch: "main"},
	{Name: "nuxt", URL: "https://github.com/nuxt/nuxt", Branch: "main"},
	{Name: "angular", URL: "https://github.com/angular/angular", Branch: "main"},

	// Styling
	{Name: "tailwindcss", URL: "https://github.com/tailwindlabs/tailwindcss", Branch: "main"},

	// Backend/Runtime - JavaScript/TypeScript
	{Name: "hono", URL: "https://github.com/honojs/hono", Branch: "main"},
	{Name: "fastify", URL: "https://github.com/fastify/fastify", Branch: "main"},
	{Name: "express", URL: "https://github.com/expressjs/express", Branch: "master"},
	{Name: "koa", URL: "https://github.com/koajs/koa", Branch: "master"},

	// ORMs and Database
	{Name: "prisma", URL: "https://github.com/prisma/prisma", Branch: "main"},
	{Name: "@prisma/client", URL: "https://github.com/prisma/prisma", Branch: "main"},
	{Name: "drizzle-orm", URL: "https://github.com/drizzle-team/drizzle-orm", Branch: "main"},

	// Testing
	{Name: "vitest", URL: "https://github.com/vitest-dev/vitest", Branch: "main"},
	{Name: "playwright", URL: "https://github.com/microsoft/playwright", Branch: "main"},
	{Name: "@playwright/test", URL: "https://github.com/microsoft/playwright", Branch: "main"},
	{Name: "jest", URL: "https://github.com/jestjs/jest", Branch: "main"},

	// Validation
	{Name: "zod", URL: "https://github.com/colinhacks/zod", Branch: "main"},

	// Build Tools
	{Name: "vite", URL: "https://github.com/vitejs/vite", Branch: "main"},
	{Name: "esbuild", URL: "https://github.com/evanw/esbuild", Branch: "main"},
	{Name: "webpack", URL: "https://github.com/webpack/webpack", Branch: "main"},

	// State Management
	{Name: "zustand", URL: "https://github.com/pmndrs/zustand", Branch: "main"},
	{Name: "jotai", URL: "https://github.com/pmndrs/jotai", Branch: "main"},

	// Elixir/Phoenix Ecosystem
	{Name: "phoenix", URL: "https://github.com/phoenixframework/phoenix", Branch: "main"},
	{Name: "phoenix_live_view", URL: "https://github.com/phoenixframework/phoenix_live_view", Branch: "main"},
	{Name: "ecto", URL: "https://github.com/elixir-ecto/ecto", Branch: "main"},
	{Name: "phoenix_html", URL: "https://github.com/phoenixframework/phoenix_html", Branch: "main"},
	{Name: "absinthe", URL: "https://github.com/absinthe-graphql/absinthe", Branch: "main"},
	{Name: "oban", URL: "https://github.com/oban-bg/oban", Branch: "main"},

	// Go Frameworks
	{Name: "gin", URL: "https://github.com/gin-gonic/gin", Branch: "master"},
	{Name: "echo", URL: "https://github.com/labstack/echo", Branch: "master"},
	{Name: "fiber", URL: "https://github.com/gofiber/fiber", Branch: "main"},
	{Name: "chi", URL: "https://github.com/go-chi/chi", Branch: "master"},
}

// MapDependencyToResource finds a Resource for a given dependency name.
// Handles aliases: "next" matches "next", "@next/font" matches "next".
func MapDependencyToResource(depName string, resources []Resource) *Resource {
	// Direct match first
	for i := range resources {
		if resources[i].Name == depName {
			return &resources[i]
		}
	}

	// Scoped package match (e.g., "@next/font" -> "next", "@prisma/client" -> "prisma")
	if strings.HasPrefix(depName, "@") {
		parts := strings.Split(depName, "/")
		if len(parts) >= 2 {
			// Extract scope name without @ (e.g., "@next/font" -> "next")
			scopeName := strings.TrimPrefix(parts[0], "@")
			for i := range resources {
				if resources[i].Name == scopeName {
					return &resources[i]
				}
			}
			// Also check the package part (e.g., "@prisma/client" where "prisma" might be in name)
			for i := range resources {
				if resources[i].Name == parts[1] {
					return &resources[i]
				}
			}
		}
	}

	// Prefix match for related packages (e.g., "react-dom" -> "react")
	for i := range resources {
		if strings.HasPrefix(depName, resources[i].Name+"-") {
			return &resources[i]
		}
	}

	return nil
}

// MergeWithCustom merges default resources with user-configured custom resources.
// Custom resources override defaults by name.
func MergeWithCustom(custom []Resource) []Resource {
	// Start with defaults
	merged := make([]Resource, len(DefaultResources))
	copy(merged, DefaultResources)

	// Create lookup map
	byName := make(map[string]int)
	for i, r := range merged {
		byName[r.Name] = i
	}

	// Apply custom resources
	for _, c := range custom {
		if idx, exists := byName[c.Name]; exists {
			// Override existing
			merged[idx] = c
		} else {
			// Add new
			merged = append(merged, c)
			byName[c.Name] = len(merged) - 1
		}
	}

	return merged
}

// GetResourceByName returns a resource by exact name match.
func GetResourceByName(name string, resources []Resource) *Resource {
	for i := range resources {
		if resources[i].Name == name {
			return &resources[i]
		}
	}
	return nil
}
