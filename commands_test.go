package main

import (
	"bufio"
	"strings"
	"testing"
)

func TestPromptProviderSelection_KnownProvider(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"aider", "1\n", "aider"},
		{"amp", "2\n", "amp"},
		{"claude", "3\n", "claude"},
		{"codex", "4\n", "codex"},
		{"opencode", "5\n", "opencode"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.input))
			got := promptProviderSelection(reader)
			if got != tt.want {
				t.Errorf("promptProviderSelection() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPromptProviderSelection_CustomProvider(t *testing.T) {
	// Select "other" (6), then enter custom command
	input := "6\nmy-custom-ai\n"
	reader := bufio.NewReader(strings.NewReader(input))
	got := promptProviderSelection(reader)
	if got != "my-custom-ai" {
		t.Errorf("promptProviderSelection() = %q, want %q", got, "my-custom-ai")
	}
}

func TestPromptProviderSelection_InvalidThenValid(t *testing.T) {
	// Invalid input first, then valid
	input := "0\n99\nabc\n3\n"
	reader := bufio.NewReader(strings.NewReader(input))
	got := promptProviderSelection(reader)
	if got != "claude" {
		t.Errorf("promptProviderSelection() = %q, want %q", got, "claude")
	}
}

func TestProviderChoices_Sorted(t *testing.T) {
	// Verify provider choices are in alphabetical order
	for i := 1; i < len(providerChoices); i++ {
		if providerChoices[i] < providerChoices[i-1] {
			t.Errorf("providerChoices not sorted: %q comes after %q", providerChoices[i], providerChoices[i-1])
		}
	}
}

func TestProviderChoices_MatchKnownProviders(t *testing.T) {
	// Every choice should be in the knownProviders map
	for _, choice := range providerChoices {
		if _, ok := knownProviders[choice]; !ok {
			t.Errorf("providerChoices contains %q which is not in knownProviders", choice)
		}
	}
	// Every known provider should be in the choices
	for name := range knownProviders {
		found := false
		for _, choice := range providerChoices {
			if choice == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("knownProviders has %q which is not in providerChoices", name)
		}
	}
}
