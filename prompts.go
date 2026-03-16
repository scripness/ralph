package main

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed prompts/*
var promptsFS embed.FS

func getPrompt(name string, vars map[string]string) string {
	data, err := promptsFS.ReadFile("prompts/" + name + ".md")
	if err != nil {
		panic("prompt not found: " + name)
	}

	content := string(data)
	for key, value := range vars {
		content = strings.ReplaceAll(content, "{{"+key+"}}", value)
	}
	return content
}

const maxLearningsInPrompt = 50

// buildLearnings formats learnings for prompt injection, capped at maxLearningsInPrompt most recent.
func buildLearnings(learnings []string, heading string) string {
	if len(learnings) == 0 {
		return ""
	}
	s := heading + "\n\n"
	start := 0
	if len(learnings) > maxLearningsInPrompt {
		s += fmt.Sprintf("_(showing %d most recent of %d learnings)_\n\n", maxLearningsInPrompt, len(learnings))
		start = len(learnings) - maxLearningsInPrompt
	}
	for _, l := range learnings[start:] {
		s += "- " + l + "\n"
	}
	return s
}


