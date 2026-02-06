package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Dependency represents an external package dependency
type Dependency struct {
	Name    string // package name (e.g., "next", "@sveltejs/kit")
	Version string // version spec (e.g., "^14.0.0", "1.0.0")
	IsDev   bool   // true if devDependency
}

// CodebaseContext contains discovered information about the codebase
// Used to provide context to PRD creation prompts
type CodebaseContext struct {
	TechStack      string       // "typescript", "go", "python", "rust", etc.
	PackageManager string       // "bun", "npm", "yarn", "pnpm", "go", "cargo", "pip"
	Frameworks     []string     // detected frameworks like ["nextjs", "react"] or ["gin", "gorm"]
	Dependencies   []Dependency // full dependency list
	TestCommand    string       // detected or configured test command
	Services       []string     // service names from ralph.config.json
	VerifyCommands []string     // verify commands from ralph.config.json
}

// DiscoverCodebase analyzes the project to detect tech stack and context
// This is lightweight detection - reads config files, doesn't run commands
func DiscoverCodebase(projectRoot string, cfg *RalphConfig) *CodebaseContext {
	ctx := &CodebaseContext{}

	// Detect tech stack and package manager
	ctx.TechStack, ctx.PackageManager = detectTechStack(projectRoot)

	// Detect frameworks
	ctx.Frameworks = detectFrameworks(projectRoot, ctx.TechStack)

	// Extract full dependency list
	ctx.Dependencies = ExtractDependencies(projectRoot, ctx.TechStack)

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

// ExtractDependencies returns all dependencies from the project
func ExtractDependencies(projectRoot string, techStack string) []Dependency {
	switch techStack {
	case "typescript", "javascript":
		return extractJSDependencies(projectRoot)
	case "go":
		return extractGoDependencies(projectRoot)
	case "python":
		return extractPythonDependencies(projectRoot)
	case "rust":
		return extractRustDependencies(projectRoot)
	}
	return nil
}

// extractJSDependencies parses package.json
func extractJSDependencies(projectRoot string) []Dependency {
	var deps []Dependency

	pkgPath := filepath.Join(projectRoot, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return deps
	}

	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return deps
	}

	for name, version := range pkg.Dependencies {
		deps = append(deps, Dependency{Name: name, Version: version, IsDev: false})
	}
	for name, version := range pkg.DevDependencies {
		deps = append(deps, Dependency{Name: name, Version: version, IsDev: true})
	}

	return deps
}

// extractGoDependencies parses go.mod
func extractGoDependencies(projectRoot string) []Dependency {
	var deps []Dependency

	modPath := filepath.Join(projectRoot, "go.mod")
	data, err := os.ReadFile(modPath)
	if err != nil {
		return deps
	}

	// Parse go.mod line by line
	lines := strings.Split(string(data), "\n")
	inRequire := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "require (" {
			inRequire = true
			continue
		}
		if line == ")" {
			inRequire = false
			continue
		}
		if inRequire && line != "" && !strings.HasPrefix(line, "//") {
			// Format: "module/path v1.2.3" or "module/path v1.2.3 // indirect"
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				deps = append(deps, Dependency{
					Name:    parts[0],
					Version: parts[1],
					IsDev:   strings.Contains(line, "// indirect"),
				})
			}
		}
		// Handle single-line require
		if strings.HasPrefix(line, "require ") && !strings.HasSuffix(line, "(") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				deps = append(deps, Dependency{
					Name:    parts[1],
					Version: parts[2],
					IsDev:   false,
				})
			}
		}
	}

	return deps
}

// extractPythonDependencies parses pyproject.toml or requirements.txt
func extractPythonDependencies(projectRoot string) []Dependency {
	var deps []Dependency

	// Try pyproject.toml first
	pyprojectPath := filepath.Join(projectRoot, "pyproject.toml")
	if data, err := os.ReadFile(pyprojectPath); err == nil {
		// Basic parsing - extract package names from dependencies
		content := string(data)
		lines := strings.Split(content, "\n")
		inDeps := false
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "[project.dependencies]" || line == "[tool.poetry.dependencies]" {
				inDeps = true
				continue
			}
			if strings.HasPrefix(line, "[") && inDeps {
				inDeps = false
				continue
			}
			if inDeps && line != "" && !strings.HasPrefix(line, "#") {
				// Format varies, try to extract package name
				if idx := strings.Index(line, "="); idx > 0 {
					name := strings.TrimSpace(line[:idx])
					name = strings.Trim(name, "\"'")
					if name != "" && name != "python" {
						deps = append(deps, Dependency{Name: name, IsDev: false})
					}
				} else if strings.HasPrefix(line, "\"") {
					// TOML array format
					name := strings.Trim(line, "\",")
					if name != "" {
						deps = append(deps, Dependency{Name: name, IsDev: false})
					}
				}
			}
		}
	}

	// Try requirements.txt
	reqPath := filepath.Join(projectRoot, "requirements.txt")
	if data, err := os.ReadFile(reqPath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
				continue
			}
			// Format: "package==1.2.3" or "package>=1.2.3" or just "package"
			for _, sep := range []string{"==", ">=", "<=", "~=", "!="} {
				if idx := strings.Index(line, sep); idx > 0 {
					deps = append(deps, Dependency{
						Name:    line[:idx],
						Version: line[idx+len(sep):],
						IsDev:   false,
					})
					break
				}
			}
		}
	}

	return deps
}

// extractRustDependencies parses Cargo.toml
func extractRustDependencies(projectRoot string) []Dependency {
	var deps []Dependency

	cargoPath := filepath.Join(projectRoot, "Cargo.toml")
	data, err := os.ReadFile(cargoPath)
	if err != nil {
		return deps
	}

	// Basic TOML parsing for dependencies section
	lines := strings.Split(string(data), "\n")
	inDeps := false
	inDevDeps := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "[dependencies]" {
			inDeps = true
			inDevDeps = false
			continue
		}
		if line == "[dev-dependencies]" {
			inDeps = false
			inDevDeps = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			inDeps = false
			inDevDeps = false
			continue
		}
		if (inDeps || inDevDeps) && line != "" && !strings.HasPrefix(line, "#") {
			// Format: "name = "version"" or "name = { version = "1.0" }"
			if idx := strings.Index(line, "="); idx > 0 {
				name := strings.TrimSpace(line[:idx])
				rest := strings.TrimSpace(line[idx+1:])
				version := ""
				if strings.HasPrefix(rest, "\"") {
					version = strings.Trim(rest, "\"")
				}
				deps = append(deps, Dependency{
					Name:    name,
					Version: version,
					IsDev:   inDevDeps,
				})
			}
		}
	}

	return deps
}

// GetDependencyNames returns just the names of dependencies
func GetDependencyNames(deps []Dependency) []string {
	var names []string
	for _, d := range deps {
		names = append(names, d.Name)
	}
	return names
}
