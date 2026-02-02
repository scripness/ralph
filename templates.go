package main

import (
	"embed"
	"strings"
)

//go:embed templates/*
var templatesFS embed.FS

func getTemplate(name string, vars map[string]string) string {
	data, err := templatesFS.ReadFile("templates/" + name + ".md")
	if err != nil {
		panic("template not found: " + name)
	}

	content := string(data)
	for key, value := range vars {
		content = strings.ReplaceAll(content, "{{"+key+"}}", value)
	}
	return content
}

func getExamplePRD() []byte {
	data, err := templatesFS.ReadFile("templates/prd.json.example")
	if err != nil {
		panic("prd.json.example not found")
	}
	return data
}

func generateRunPrompt(cfg ResolvedConfig) string {
	var qualityLines []string
	for _, q := range cfg.Quality {
		qualityLines = append(qualityLines, "   "+q.Cmd+"   # "+q.Name)
	}
	qualityCommands := strings.Join(qualityLines, "\n")
	if qualityCommands == "" {
		qualityCommands = "   # No quality commands configured - check project setup"
	}

	return getTemplate("run", map[string]string{
		"prdPath":         cfg.PrdPath,
		"projectRoot":     cfg.ProjectRoot,
		"qualityCommands": qualityCommands,
	})
}

func generateVerifyPrompt(cfg ResolvedConfig) string {
	var qualityLines []string
	for _, q := range cfg.Quality {
		qualityLines = append(qualityLines, "   "+q.Cmd+"   # "+q.Name)
	}
	qualityCommands := strings.Join(qualityLines, "\n")
	if qualityCommands == "" {
		qualityCommands = "   # No quality commands configured - check project setup"
	}

	return getTemplate("verify", map[string]string{
		"prdPath":         cfg.PrdPath,
		"projectRoot":     cfg.ProjectRoot,
		"qualityCommands": qualityCommands,
	})
}
