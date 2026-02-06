package main

import (
	"strings"
)

// Resource maps a framework/library to its source code repository
type Resource struct {
	Name        string   // e.g., "nextjs", "react", "svelte"
	URL         string   // GitHub repo URL (full source)
	Branch      string   // e.g., "main", "canary"
	SearchPaths []string // Optional: focus areas (empty = search all)
	Notes       string   // Guidance for AI agent
}

// DefaultResources is the built-in registry of popular frameworks.
// These are SOURCE CODE repos, not just docs.
var DefaultResources = []Resource{
	// Frontend Frameworks - FULL SOURCE
	{Name: "next", URL: "https://github.com/vercel/next.js", Branch: "canary",
		Notes: "Next.js framework. Check packages/next/src for core, docs/ for patterns."},
	{Name: "react", URL: "https://github.com/facebook/react", Branch: "main",
		Notes: "React library. Check packages/react/src for core implementation."},
	{Name: "svelte", URL: "https://github.com/sveltejs/svelte", Branch: "main",
		Notes: "Svelte compiler and runtime. Check packages/svelte/src."},
	{Name: "@sveltejs/kit", URL: "https://github.com/sveltejs/kit", Branch: "main",
		Notes: "SvelteKit framework. Check packages/kit/src."},
	{Name: "vue", URL: "https://github.com/vuejs/core", Branch: "main",
		Notes: "Vue 3 core. Check packages/vue/src and packages/runtime-core/src."},
	{Name: "nuxt", URL: "https://github.com/nuxt/nuxt", Branch: "main",
		Notes: "Nuxt framework. Check packages/nuxt/src."},
	{Name: "angular", URL: "https://github.com/angular/angular", Branch: "main",
		Notes: "Angular framework. Check packages/core/src."},

	// Styling
	{Name: "tailwindcss", URL: "https://github.com/tailwindlabs/tailwindcss", Branch: "main",
		Notes: "Tailwind CSS. Check src/ for plugin system and utilities."},

	// Backend/Runtime - JavaScript/TypeScript
	{Name: "hono", URL: "https://github.com/honojs/hono", Branch: "main",
		Notes: "Hono web framework. Check src/ for middleware patterns."},
	{Name: "fastify", URL: "https://github.com/fastify/fastify", Branch: "main",
		Notes: "Fastify framework. Check lib/ for core, types/ for TypeScript."},
	{Name: "express", URL: "https://github.com/expressjs/express", Branch: "master",
		Notes: "Express framework. Check lib/ for core middleware patterns."},
	{Name: "koa", URL: "https://github.com/koajs/koa", Branch: "master",
		Notes: "Koa framework. Check lib/ for middleware patterns."},

	// ORMs and Database
	{Name: "prisma", URL: "https://github.com/prisma/prisma", Branch: "main",
		Notes: "Prisma ORM. Check packages/client/src for client generation."},
	{Name: "@prisma/client", URL: "https://github.com/prisma/prisma", Branch: "main",
		Notes: "Prisma ORM. Check packages/client/src for client generation."},
	{Name: "drizzle-orm", URL: "https://github.com/drizzle-team/drizzle-orm", Branch: "main",
		Notes: "Drizzle ORM. Check drizzle-orm/src for query builders."},

	// Testing
	{Name: "vitest", URL: "https://github.com/vitest-dev/vitest", Branch: "main",
		Notes: "Vitest testing framework. Check packages/vitest/src."},
	{Name: "playwright", URL: "https://github.com/microsoft/playwright", Branch: "main",
		Notes: "Playwright testing. Check packages/playwright-core/src."},
	{Name: "@playwright/test", URL: "https://github.com/microsoft/playwright", Branch: "main",
		Notes: "Playwright testing. Check packages/playwright-core/src."},
	{Name: "jest", URL: "https://github.com/jestjs/jest", Branch: "main",
		Notes: "Jest testing framework. Check packages/jest/src."},

	// Validation
	{Name: "zod", URL: "https://github.com/colinhacks/zod", Branch: "main",
		Notes: "Zod validation. Check src/ for schema definitions."},

	// Build Tools
	{Name: "vite", URL: "https://github.com/vitejs/vite", Branch: "main",
		Notes: "Vite build tool. Check packages/vite/src."},
	{Name: "esbuild", URL: "https://github.com/evanw/esbuild", Branch: "main",
		Notes: "esbuild bundler. Check pkg/api for Go API."},
	{Name: "webpack", URL: "https://github.com/webpack/webpack", Branch: "main",
		Notes: "Webpack bundler. Check lib/ for core."},

	// State Management
	{Name: "zustand", URL: "https://github.com/pmndrs/zustand", Branch: "main",
		Notes: "Zustand state management. Check src/ for core store implementation."},
	{Name: "jotai", URL: "https://github.com/pmndrs/jotai", Branch: "main",
		Notes: "Jotai atomic state management. Check src/ for atom implementation."},

	// Go Frameworks
	{Name: "gin", URL: "https://github.com/gin-gonic/gin", Branch: "master",
		Notes: "Gin web framework for Go. Check .go files for middleware patterns."},
	{Name: "echo", URL: "https://github.com/labstack/echo", Branch: "master",
		Notes: "Echo web framework for Go. Check echo.go for core."},
	{Name: "fiber", URL: "https://github.com/gofiber/fiber", Branch: "main",
		Notes: "Fiber web framework for Go. Check app.go for core."},
	{Name: "chi", URL: "https://github.com/go-chi/chi", Branch: "master",
		Notes: "Chi router for Go. Check chi.go and mux.go for routing."},
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
