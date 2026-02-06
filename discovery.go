package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CodebaseContext contains discovered information about the codebase
// Used to provide context to PRD creation prompts
type CodebaseContext struct {
	TechStack      string   // "typescript", "go", "python", "rust", etc.
	PackageManager string   // "bun", "npm", "yarn", "pnpm", "go", "cargo", "pip"
	Frameworks     []string // detected frameworks like ["nextjs", "react"] or ["gin", "gorm"]
	TestCommand    string   // detected or configured test command
	Services       []string // service names from ralph.config.json
	VerifyCommands []string // verify commands from ralph.config.json
}

// DiscoverCodebase analyzes the project to detect tech stack and context
// This is lightweight detection - reads config files, doesn't run commands
func DiscoverCodebase(projectRoot string, cfg *RalphConfig) *CodebaseContext {
	ctx := &CodebaseContext{}

	// Detect tech stack and package manager
	ctx.TechStack, ctx.PackageManager = detectTechStack(projectRoot)

	// Detect frameworks
	ctx.Frameworks = detectFrameworks(projectRoot, ctx.TechStack)

	// Extract from ralph.config.json
	if cfg != nil {
		for _, svc := range cfg.Services {
			ctx.Services = append(ctx.Services, fmt.Sprintf("%s (%s)", svc.Name, svc.Ready))
		}
		ctx.VerifyCommands = append(ctx.VerifyCommands, cfg.Verify.Default...)
		ctx.VerifyCommands = append(ctx.VerifyCommands, cfg.Verify.UI...)

		// Try to extract test command from verify commands
		for _, cmd := range cfg.Verify.Default {
			if strings.Contains(cmd, "test") {
				ctx.TestCommand = cmd
				break
			}
		}
	}

	return ctx
}

// detectTechStack detects the primary language and package manager
func detectTechStack(projectRoot string) (techStack, packageManager string) {
	// Check for Go
	if fileExists(filepath.Join(projectRoot, "go.mod")) {
		return "go", "go"
	}

	// Check for Rust
	if fileExists(filepath.Join(projectRoot, "Cargo.toml")) {
		return "rust", "cargo"
	}

	// Check for Python
	if fileExists(filepath.Join(projectRoot, "pyproject.toml")) {
		return "python", "pip"
	}
	if fileExists(filepath.Join(projectRoot, "requirements.txt")) {
		return "python", "pip"
	}
	if fileExists(filepath.Join(projectRoot, "setup.py")) {
		return "python", "pip"
	}

	// Check for Node.js/TypeScript (check package.json and lock files)
	if fileExists(filepath.Join(projectRoot, "package.json")) {
		// Determine TypeScript vs JavaScript
		techStack = "javascript"
		if fileExists(filepath.Join(projectRoot, "tsconfig.json")) {
			techStack = "typescript"
		}

		// Determine package manager from lock file
		if fileExists(filepath.Join(projectRoot, "bun.lockb")) || fileExists(filepath.Join(projectRoot, "bun.lock")) {
			return techStack, "bun"
		}
		if fileExists(filepath.Join(projectRoot, "pnpm-lock.yaml")) {
			return techStack, "pnpm"
		}
		if fileExists(filepath.Join(projectRoot, "yarn.lock")) {
			return techStack, "yarn"
		}
		return techStack, "npm"
	}

	return "unknown", "unknown"
}

// detectFrameworks detects frameworks based on config files and dependencies
func detectFrameworks(projectRoot string, techStack string) []string {
	var frameworks []string

	switch techStack {
	case "typescript", "javascript":
		frameworks = detectJSFrameworks(projectRoot)
	case "go":
		frameworks = detectGoFrameworks(projectRoot)
	case "python":
		frameworks = detectPythonFrameworks(projectRoot)
	case "rust":
		frameworks = detectRustFrameworks(projectRoot)
	}

	return frameworks
}

// detectJSFrameworks detects JavaScript/TypeScript frameworks
func detectJSFrameworks(projectRoot string) []string {
	var frameworks []string

	// Read package.json
	pkgPath := filepath.Join(projectRoot, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return frameworks
	}

	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return frameworks
	}

	// Merge dependencies
	allDeps := make(map[string]bool)
	for dep := range pkg.Dependencies {
		allDeps[dep] = true
	}
	for dep := range pkg.DevDependencies {
		allDeps[dep] = true
	}

	// Check for Next.js
	if allDeps["next"] {
		frameworks = append(frameworks, "nextjs")
	}

	// Check for React (but not if Next.js already detected)
	if allDeps["react"] && !allDeps["next"] {
		frameworks = append(frameworks, "react")
	}

	// Check for Vue
	if allDeps["vue"] {
		frameworks = append(frameworks, "vue")
	}

	// Check for Nuxt
	if allDeps["nuxt"] {
		frameworks = append(frameworks, "nuxt")
	}

	// Check for Svelte/SvelteKit
	if allDeps["@sveltejs/kit"] {
		frameworks = append(frameworks, "sveltekit")
	} else if allDeps["svelte"] {
		frameworks = append(frameworks, "svelte")
	}

	// Check for Express
	if allDeps["express"] {
		frameworks = append(frameworks, "express")
	}

	// Check for Fastify
	if allDeps["fastify"] {
		frameworks = append(frameworks, "fastify")
	}

	// Check for Hono
	if allDeps["hono"] {
		frameworks = append(frameworks, "hono")
	}

	// Check for testing frameworks
	if allDeps["vitest"] {
		frameworks = append(frameworks, "vitest")
	} else if allDeps["jest"] {
		frameworks = append(frameworks, "jest")
	}

	// Check for Playwright
	if allDeps["@playwright/test"] || allDeps["playwright"] {
		frameworks = append(frameworks, "playwright")
	}

	// Check for Drizzle ORM
	if allDeps["drizzle-orm"] {
		frameworks = append(frameworks, "drizzle")
	}

	// Check for Prisma
	if allDeps["@prisma/client"] || allDeps["prisma"] {
		frameworks = append(frameworks, "prisma")
	}

	return frameworks
}

// detectGoFrameworks detects Go frameworks from go.mod
func detectGoFrameworks(projectRoot string) []string {
	var frameworks []string

	modPath := filepath.Join(projectRoot, "go.mod")
	data, err := os.ReadFile(modPath)
	if err != nil {
		return frameworks
	}

	content := string(data)

	// Check for common Go frameworks/libraries
	frameworkChecks := map[string]string{
		"github.com/gin-gonic/gin":   "gin",
		"github.com/labstack/echo":   "echo",
		"github.com/gofiber/fiber":   "fiber",
		"github.com/go-chi/chi":      "chi",
		"github.com/gorilla/mux":     "gorilla",
		"gorm.io/gorm":               "gorm",
		"github.com/jmoiron/sqlx":    "sqlx",
		"github.com/go-rod/rod":      "rod",
		"github.com/playwright-community/playwright-go": "playwright",
	}

	for dep, name := range frameworkChecks {
		if strings.Contains(content, dep) {
			frameworks = append(frameworks, name)
		}
	}

	return frameworks
}

// detectPythonFrameworks detects Python frameworks
func detectPythonFrameworks(projectRoot string) []string {
	var frameworks []string

	// Try pyproject.toml first
	pyprojectPath := filepath.Join(projectRoot, "pyproject.toml")
	if data, err := os.ReadFile(pyprojectPath); err == nil {
		content := string(data)
		frameworks = checkPythonDeps(content, frameworks)
	}

	// Try requirements.txt
	reqPath := filepath.Join(projectRoot, "requirements.txt")
	if data, err := os.ReadFile(reqPath); err == nil {
		content := string(data)
		frameworks = checkPythonDeps(content, frameworks)
	}

	return frameworks
}

func checkPythonDeps(content string, frameworks []string) []string {
	deps := map[string]string{
		"django":     "django",
		"flask":      "flask",
		"fastapi":    "fastapi",
		"pytest":     "pytest",
		"sqlalchemy": "sqlalchemy",
	}

	for dep, name := range deps {
		if strings.Contains(strings.ToLower(content), dep) {
			frameworks = append(frameworks, name)
		}
	}

	return frameworks
}

// detectRustFrameworks detects Rust frameworks from Cargo.toml
func detectRustFrameworks(projectRoot string) []string {
	var frameworks []string

	cargoPath := filepath.Join(projectRoot, "Cargo.toml")
	data, err := os.ReadFile(cargoPath)
	if err != nil {
		return frameworks
	}

	content := string(data)

	deps := map[string]string{
		"actix-web": "actix",
		"axum":      "axum",
		"rocket":    "rocket",
		"tokio":     "tokio",
		"diesel":    "diesel",
		"sqlx":      "sqlx",
	}

	for dep, name := range deps {
		if strings.Contains(content, dep) {
			frameworks = append(frameworks, name)
		}
	}

	return frameworks
}

// FormatCodebaseContext formats the discovered context for inclusion in prompts
func FormatCodebaseContext(ctx *CodebaseContext) string {
	if ctx == nil || ctx.TechStack == "unknown" {
		return ""
	}

	var lines []string
	lines = append(lines, "## Codebase Context")
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("**Tech Stack:** %s", ctx.TechStack))
	lines = append(lines, fmt.Sprintf("**Package Manager:** %s", ctx.PackageManager))

	if len(ctx.Frameworks) > 0 {
		lines = append(lines, fmt.Sprintf("**Frameworks:** %s", strings.Join(ctx.Frameworks, ", ")))
	}

	if ctx.TestCommand != "" {
		lines = append(lines, fmt.Sprintf("**Test Command:** `%s`", ctx.TestCommand))
	}

	if len(ctx.Services) > 0 {
		lines = append(lines, "")
		lines = append(lines, "**Configured Services:**")
		for _, svc := range ctx.Services {
			lines = append(lines, fmt.Sprintf("- %s", svc))
		}
	}

	if len(ctx.VerifyCommands) > 0 {
		lines = append(lines, "")
		lines = append(lines, "**Verification Commands:**")
		for _, cmd := range ctx.VerifyCommands {
			lines = append(lines, fmt.Sprintf("- `%s`", cmd))
		}
	}

	lines = append(lines, "")
	return strings.Join(lines, "\n")
}
