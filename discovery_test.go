package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectTechStack_Go(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test"), 0644)

	stack, pm := detectTechStack(dir)
	if stack != "go" {
		t.Errorf("expected stack='go', got '%s'", stack)
	}
	if pm != "go" {
		t.Errorf("expected pm='go', got '%s'", pm)
	}
}

func TestDetectTechStack_Rust(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]"), 0644)

	stack, pm := detectTechStack(dir)
	if stack != "rust" {
		t.Errorf("expected stack='rust', got '%s'", stack)
	}
	if pm != "cargo" {
		t.Errorf("expected pm='cargo', got '%s'", pm)
	}
}

func TestDetectTechStack_TypeScript_Bun(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "bun.lockb"), []byte(""), 0644)

	stack, pm := detectTechStack(dir)
	if stack != "typescript" {
		t.Errorf("expected stack='typescript', got '%s'", stack)
	}
	if pm != "bun" {
		t.Errorf("expected pm='bun', got '%s'", pm)
	}
}

func TestDetectTechStack_JavaScript_NPM(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte("{}"), 0644)

	stack, pm := detectTechStack(dir)
	if stack != "javascript" {
		t.Errorf("expected stack='javascript', got '%s'", stack)
	}
	if pm != "npm" {
		t.Errorf("expected pm='npm', got '%s'", pm)
	}
}

func TestDetectTechStack_TypeScript_Yarn(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "yarn.lock"), []byte(""), 0644)

	stack, pm := detectTechStack(dir)
	if stack != "typescript" {
		t.Errorf("expected stack='typescript', got '%s'", stack)
	}
	if pm != "yarn" {
		t.Errorf("expected pm='yarn', got '%s'", pm)
	}
}

func TestDetectTechStack_TypeScript_PNPM(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "pnpm-lock.yaml"), []byte(""), 0644)

	stack, pm := detectTechStack(dir)
	if stack != "typescript" {
		t.Errorf("expected stack='typescript', got '%s'", stack)
	}
	if pm != "pnpm" {
		t.Errorf("expected pm='pnpm', got '%s'", pm)
	}
}

func TestDetectTechStack_Python_Pyproject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]"), 0644)

	stack, pm := detectTechStack(dir)
	if stack != "python" {
		t.Errorf("expected stack='python', got '%s'", stack)
	}
	if pm != "pip" {
		t.Errorf("expected pm='pip', got '%s'", pm)
	}
}

func TestDetectTechStack_Python_Requirements(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("flask"), 0644)

	stack, pm := detectTechStack(dir)
	if stack != "python" {
		t.Errorf("expected stack='python', got '%s'", stack)
	}
	if pm != "pip" {
		t.Errorf("expected pm='pip', got '%s'", pm)
	}
}

func TestDetectTechStack_Unknown(t *testing.T) {
	dir := t.TempDir()

	stack, pm := detectTechStack(dir)
	if stack != "unknown" {
		t.Errorf("expected stack='unknown', got '%s'", stack)
	}
	if pm != "unknown" {
		t.Errorf("expected pm='unknown', got '%s'", pm)
	}
}

func TestDetectJSFrameworks_NextJS(t *testing.T) {
	dir := t.TempDir()
	pkgJson := `{"dependencies": {"next": "14.0.0", "react": "18.0.0"}}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgJson), 0644)

	frameworks := detectJSFrameworks(dir)
	hasNext := false
	hasReact := false
	for _, f := range frameworks {
		if f == "nextjs" {
			hasNext = true
		}
		if f == "react" {
			hasReact = true
		}
	}
	if !hasNext {
		t.Error("expected 'nextjs' in frameworks")
	}
	// React should NOT be listed separately when Next.js is present
	if hasReact {
		t.Error("expected 'react' to NOT be listed when 'nextjs' is present")
	}
}

func TestDetectJSFrameworks_ReactOnly(t *testing.T) {
	dir := t.TempDir()
	pkgJson := `{"dependencies": {"react": "18.0.0", "react-dom": "18.0.0"}}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgJson), 0644)

	frameworks := detectJSFrameworks(dir)
	hasReact := false
	for _, f := range frameworks {
		if f == "react" {
			hasReact = true
		}
	}
	if !hasReact {
		t.Error("expected 'react' in frameworks")
	}
}

func TestDetectJSFrameworks_Vue(t *testing.T) {
	dir := t.TempDir()
	pkgJson := `{"dependencies": {"vue": "3.0.0"}}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgJson), 0644)

	frameworks := detectJSFrameworks(dir)
	hasVue := false
	for _, f := range frameworks {
		if f == "vue" {
			hasVue = true
		}
	}
	if !hasVue {
		t.Error("expected 'vue' in frameworks")
	}
}

func TestDetectJSFrameworks_Testing(t *testing.T) {
	dir := t.TempDir()
	pkgJson := `{"devDependencies": {"vitest": "1.0.0", "@playwright/test": "1.40.0"}}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgJson), 0644)

	frameworks := detectJSFrameworks(dir)
	hasVitest := false
	hasPlaywright := false
	for _, f := range frameworks {
		if f == "vitest" {
			hasVitest = true
		}
		if f == "playwright" {
			hasPlaywright = true
		}
	}
	if !hasVitest {
		t.Error("expected 'vitest' in frameworks")
	}
	if !hasPlaywright {
		t.Error("expected 'playwright' in frameworks")
	}
}

func TestDetectGoFrameworks(t *testing.T) {
	dir := t.TempDir()
	goMod := `module example.com/test

require (
	github.com/gin-gonic/gin v1.9.0
	gorm.io/gorm v1.25.0
	github.com/go-rod/rod v0.114.0
)`
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644)

	frameworks := detectGoFrameworks(dir)
	hasGin := false
	hasGorm := false
	hasRod := false
	for _, f := range frameworks {
		if f == "gin" {
			hasGin = true
		}
		if f == "gorm" {
			hasGorm = true
		}
		if f == "rod" {
			hasRod = true
		}
	}
	if !hasGin {
		t.Error("expected 'gin' in frameworks")
	}
	if !hasGorm {
		t.Error("expected 'gorm' in frameworks")
	}
	if !hasRod {
		t.Error("expected 'rod' in frameworks")
	}
}

func TestDetectPythonFrameworks(t *testing.T) {
	dir := t.TempDir()
	requirements := `Django==4.2.0
pytest==7.4.0
sqlalchemy==2.0.0`
	os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte(requirements), 0644)

	frameworks := detectPythonFrameworks(dir)
	hasDjango := false
	hasPytest := false
	hasSqlalchemy := false
	for _, f := range frameworks {
		if f == "django" {
			hasDjango = true
		}
		if f == "pytest" {
			hasPytest = true
		}
		if f == "sqlalchemy" {
			hasSqlalchemy = true
		}
	}
	if !hasDjango {
		t.Error("expected 'django' in frameworks")
	}
	if !hasPytest {
		t.Error("expected 'pytest' in frameworks")
	}
	if !hasSqlalchemy {
		t.Error("expected 'sqlalchemy' in frameworks")
	}
}

func TestDiscoverCodebase_WithConfig(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies": {"next": "14.0.0"}}`), 0644)
	os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "bun.lockb"), []byte(""), 0644)

	cfg := &RalphConfig{
		Services: []ServiceConfig{
			{Name: "dev", Ready: "http://localhost:3000"},
		},
		Verify: VerifyConfig{
			Default: []string{"bun run typecheck", "bun run lint", "bun run test"},
			UI:      []string{"bun run test:e2e"},
		},
	}

	ctx := DiscoverCodebase(dir, cfg)

	if ctx.TechStack != "typescript" {
		t.Errorf("expected stack='typescript', got '%s'", ctx.TechStack)
	}
	if ctx.PackageManager != "bun" {
		t.Errorf("expected pm='bun', got '%s'", ctx.PackageManager)
	}
	if len(ctx.Services) != 1 {
		t.Errorf("expected 1 service, got %d", len(ctx.Services))
	}
	if len(ctx.VerifyCommands) != 4 {
		t.Errorf("expected 4 verify commands, got %d", len(ctx.VerifyCommands))
	}
	if ctx.TestCommand != "bun run test" {
		t.Errorf("expected testCommand='bun run test', got '%s'", ctx.TestCommand)
	}
}

func TestFormatCodebaseContext_Empty(t *testing.T) {
	ctx := &CodebaseContext{TechStack: "unknown"}
	result := FormatCodebaseContext(ctx)
	if result != "" {
		t.Errorf("expected empty string for unknown stack, got '%s'", result)
	}
}

func TestFormatCodebaseContext_Nil(t *testing.T) {
	result := FormatCodebaseContext(nil)
	if result != "" {
		t.Errorf("expected empty string for nil context, got '%s'", result)
	}
}

func TestFormatCodebaseContext_Full(t *testing.T) {
	ctx := &CodebaseContext{
		TechStack:      "typescript",
		PackageManager: "bun",
		Frameworks:     []string{"nextjs", "prisma"},
		TestCommand:    "bun run test",
		Services:       []string{"dev (http://localhost:3000)"},
		VerifyCommands: []string{"bun run typecheck", "bun run test"},
	}

	result := FormatCodebaseContext(ctx)

	if !strings.Contains(result, "## Codebase Context") {
		t.Error("expected '## Codebase Context' header")
	}
	if !strings.Contains(result, "**Tech Stack:** typescript") {
		t.Error("expected tech stack in output")
	}
	if !strings.Contains(result, "**Package Manager:** bun") {
		t.Error("expected package manager in output")
	}
	if !strings.Contains(result, "**Frameworks:** nextjs, prisma") {
		t.Error("expected frameworks in output")
	}
	if !strings.Contains(result, "**Test Command:** `bun run test`") {
		t.Error("expected test command in output")
	}
	if !strings.Contains(result, "**Configured Services:**") {
		t.Error("expected services header")
	}
	if !strings.Contains(result, "**Verification Commands:**") {
		t.Error("expected verification commands header")
	}
}
