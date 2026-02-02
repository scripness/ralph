package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
)

type QualityCommand struct {
	Name string `json:"name"`
	Cmd  string `json:"cmd"`
}

type AmpConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type RalphConfig struct {
	PrdPath    string           `json:"prdPath,omitempty"`
	Iterations int              `json:"iterations,omitempty"`
	Verify     *bool            `json:"verify,omitempty"`
	Quality    []QualityCommand `json:"quality,omitempty"`
	Amp        *AmpConfig       `json:"amp,omitempty"`
}

type ResolvedConfig struct {
	PrdPath     string
	Iterations  int
	Verify      bool
	Quality     []QualityCommand
	Amp         AmpConfig
	ProjectRoot string
	ProjectType string
}

func findGitRoot(start string) string {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return start
		}
		dir = parent
	}
}

func loadConfigFile(projectRoot string) *RalphConfig {
	paths := []string{
		filepath.Join(projectRoot, "ralph.config.json"),
		filepath.Join(projectRoot, "scripts", "ralph", "ralph.config.json"),
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cfg RalphConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			continue
		}
		return &cfg
	}
	return nil
}

func resolveConfig(iterations int, verify bool) ResolvedConfig {
	cwd, _ := os.Getwd()
	projectRoot := findGitRoot(cwd)
	projectType, quality := detectProject(projectRoot)

	fileCfg := loadConfigFile(projectRoot)

	// Defaults
	cfg := ResolvedConfig{
		PrdPath:     filepath.Join(projectRoot, "scripts", "ralph", "prd.json"),
		Iterations:  10,
		Verify:      true,
		Quality:     quality,
		Amp:         AmpConfig{Command: "amp", Args: []string{"--dangerously-allow-all"}},
		ProjectRoot: projectRoot,
		ProjectType: projectType,
	}

	// Override from file config
	if fileCfg != nil {
		if fileCfg.PrdPath != "" {
			cfg.PrdPath = filepath.Join(projectRoot, fileCfg.PrdPath)
		}
		if fileCfg.Iterations > 0 {
			cfg.Iterations = fileCfg.Iterations
		}
		if fileCfg.Verify != nil {
			cfg.Verify = *fileCfg.Verify
		}
		if len(fileCfg.Quality) > 0 {
			cfg.Quality = fileCfg.Quality
		}
		if fileCfg.Amp != nil {
			if fileCfg.Amp.Command != "" {
				cfg.Amp.Command = fileCfg.Amp.Command
			}
			if len(fileCfg.Amp.Args) > 0 {
				cfg.Amp.Args = fileCfg.Amp.Args
			}
		}
	}

	// Override from function args
	if iterations > 0 {
		cfg.Iterations = iterations
	}
	cfg.Verify = verify

	return cfg
}

func isCommandAvailable(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}
