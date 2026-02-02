package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func detectProject(root string) (projectType string, quality []QualityCommand) {
	// Check for Bun/Node
	if cmds := detectJSProject(root); cmds != nil {
		if fileExists(filepath.Join(root, "bun.lock")) {
			return "bun", cmds
		}
		return "node", cmds
	}

	// Check for Cargo (Rust)
	if fileExists(filepath.Join(root, "Cargo.toml")) {
		return "cargo", []QualityCommand{
			{Name: "check", Cmd: "cargo check"},
			{Name: "clippy", Cmd: "cargo clippy -- -D warnings"},
			{Name: "test", Cmd: "cargo test"},
		}
	}

	// Check for Go
	if fileExists(filepath.Join(root, "go.mod")) {
		return "go", []QualityCommand{
			{Name: "build", Cmd: "go build ./..."},
			{Name: "vet", Cmd: "go vet ./..."},
			{Name: "test", Cmd: "go test ./..."},
		}
	}

	return "unknown", nil
}

func detectJSProject(root string) []QualityCommand {
	pkgPath := filepath.Join(root, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return nil
	}

	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}

	isBun := fileExists(filepath.Join(root, "bun.lock"))
	runner := "npm run"
	if isBun {
		runner = "bun run"
	}

	var cmds []QualityCommand

	if _, ok := pkg.Scripts["typecheck"]; ok {
		cmds = append(cmds, QualityCommand{Name: "typecheck", Cmd: runner + " typecheck"})
	} else if fileExists(filepath.Join(root, "tsconfig.json")) {
		if isBun {
			cmds = append(cmds, QualityCommand{Name: "typecheck", Cmd: "bun run tsc --noEmit"})
		} else {
			cmds = append(cmds, QualityCommand{Name: "typecheck", Cmd: "npx tsc --noEmit"})
		}
	}

	if _, ok := pkg.Scripts["lint"]; ok {
		cmds = append(cmds, QualityCommand{Name: "lint", Cmd: runner + " lint"})
	}

	if _, ok := pkg.Scripts["test"]; ok {
		cmds = append(cmds, QualityCommand{Name: "test", Cmd: runner + " test"})
	}

	if len(cmds) == 0 {
		return nil
	}
	return cmds
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
