package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// PlanItem is a single work item parsed from plan.md.
type PlanItem struct {
	Title      string
	Acceptance []string
	DependsOn  []string
}

// Plan is the parsed representation of a plan.md file.
type Plan struct {
	Feature   string
	Created   string
	ItemCount int
	Items     []PlanItem
}

// PlanVerification holds verification results from a finalized planning round.
type PlanVerification struct {
	Items    int      `json:"items"`
	Warnings []string `json:"warnings,omitempty"`
}

// PlanRound is a single planning round entry in plan.jsonl.
type PlanRound struct {
	Round        int               `json:"round"`
	Timestamp    string            `json:"ts"`
	UserInput    string            `json:"user_input"`
	Consultation []string          `json:"consultation,omitempty"`
	AIResponse   string            `json:"ai_response"`
	HasPlanDraft bool              `json:"has_plan_draft,omitempty"`
	Finalized    bool              `json:"finalized,omitempty"`
	Verification *PlanVerification `json:"verification,omitempty"`
}

var (
	reItemTitle  = regexp.MustCompile(`^\d+\.\s+\*\*(.+?)\*\*`)
	reAcceptance = regexp.MustCompile(`^\s+-\s+Acceptance:\s*(.+)`)
	reDependsOn  = regexp.MustCompile(`^\s+-\s+Depends on:\s*(.+)`)
)

// ParsePlan parses a plan.md file into a Plan struct.
// Handles YAML frontmatter gracefully — if malformed, treats entire content as body.
func ParsePlan(content string) (*Plan, error) {
	plan := &Plan{}
	body := content

	if strings.HasPrefix(strings.TrimSpace(content), "---") {
		fm, rest, ok := splitFrontmatter(content)
		if ok {
			parseFrontmatter(fm, plan)
			body = rest
		}
	}

	plan.Items = parseItems(body)

	if plan.ItemCount == 0 {
		plan.ItemCount = len(plan.Items)
	}

	return plan, nil
}

// splitFrontmatter splits content at --- delimiters.
func splitFrontmatter(content string) (string, string, bool) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---") {
		return "", "", false
	}

	rest := trimmed[3:]
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	}
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", "", false
	}

	fm := strings.TrimSpace(rest[:idx])
	body := rest[idx+4:] // skip "\n---"
	return fm, body, true
}

// parseFrontmatter extracts key-value pairs from simple YAML-like frontmatter.
func parseFrontmatter(fm string, plan *Plan) {
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "feature":
			plan.Feature = val
		case "created":
			plan.Created = val
		case "item_count":
			if n, err := strconv.Atoi(val); err == nil {
				plan.ItemCount = n
			}
		}
	}
}

// parseItems extracts numbered items with acceptance criteria from markdown body.
func parseItems(body string) []PlanItem {
	var items []PlanItem
	var current *PlanItem

	for _, line := range strings.Split(body, "\n") {
		if m := reItemTitle.FindStringSubmatch(line); m != nil {
			if current != nil {
				items = append(items, *current)
			}
			current = &PlanItem{Title: strings.TrimSpace(m[1])}
			continue
		}

		if current == nil {
			continue
		}

		if m := reAcceptance.FindStringSubmatch(line); m != nil {
			current.Acceptance = append(current.Acceptance, strings.TrimSpace(m[1]))
		} else if m := reDependsOn.FindStringSubmatch(line); m != nil {
			current.DependsOn = append(current.DependsOn, strings.TrimSpace(m[1]))
		}
	}

	if current != nil {
		items = append(items, *current)
	}
	return items
}

// WritePlanMd writes a Plan to plan.md format with YAML frontmatter.
func WritePlanMd(path string, plan *Plan) error {
	var buf strings.Builder

	buf.WriteString("---\n")
	buf.WriteString(fmt.Sprintf("feature: %s\n", plan.Feature))
	buf.WriteString(fmt.Sprintf("created: %s\n", plan.Created))
	buf.WriteString(fmt.Sprintf("item_count: %d\n", len(plan.Items)))
	buf.WriteString("---\n\n")

	// Title: convert kebab-case feature name to title case
	title := strings.ReplaceAll(plan.Feature, "-", " ")
	words := strings.Fields(title)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	buf.WriteString("# " + strings.Join(words, " ") + "\n\n")
	buf.WriteString("## Items\n\n")

	for i, item := range plan.Items {
		buf.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, item.Title))
		for _, a := range item.Acceptance {
			buf.WriteString(fmt.Sprintf("   - Acceptance: %s\n", a))
		}
		for _, d := range item.DependsOn {
			buf.WriteString(fmt.Sprintf("   - Depends on: %s\n", d))
		}
		if i < len(plan.Items)-1 {
			buf.WriteString("\n")
		}
	}

	return AtomicWriteFile(path, []byte(buf.String()))
}

// AppendPlanRound appends a planning round to plan.jsonl.
func AppendPlanRound(path string, round *PlanRound) error {
	line, err := json.Marshal(round)
	if err != nil {
		return fmt.Errorf("marshal plan round: %w", err)
	}
	line = append(line, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create plan dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open plan.jsonl: %w", err)
	}
	defer f.Close()

	_, err = f.Write(line)
	return err
}

// LoadPlanRounds reads all planning rounds from plan.jsonl.
// Returns (nil, nil) if the file does not exist.
func LoadPlanRounds(path string) ([]PlanRound, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open plan.jsonl: %w", err)
	}
	defer f.Close()

	var rounds []PlanRound
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var round PlanRound
		if err := json.Unmarshal([]byte(line), &round); err != nil {
			continue // skip corrupt lines
		}
		rounds = append(rounds, round)
	}
	return rounds, scanner.Err()
}

const planHistoryMaxBytes = 32768 // ~8,000 tokens

// BuildPlanHistory constructs the {{planHistory}} template variable from plan.jsonl rounds.
// Progressive compression (deterministic, no AI summarization):
//   - Recent 5 rounds (last 5): verbatim with all fields
//   - Middle rounds (6 through N-5): decision-only — round, ts, user_input, first 200 chars of ai_response
//   - Old rounds (1-5 if N>10): one-line digest
//   - Max ~32KB budget; drops oldest digests first, then oldest middle rounds
func BuildPlanHistory(rounds []PlanRound) string {
	n := len(rounds)
	if n == 0 {
		return ""
	}

	recentStart := n - 5
	if recentStart < 0 {
		recentStart = 0
	}

	// Old rounds: indices 0-4 only if N > 10
	var oldDigests []string
	if n > 10 {
		end := 5
		for i := 0; i < end; i++ {
			oldDigests = append(oldDigests, formatDigestRound(rounds[i]))
		}
	}

	// Middle rounds: between old and recent
	middleStart := 0
	if n > 10 {
		middleStart = 5
	}
	var middleEntries []string
	for i := middleStart; i < recentStart; i++ {
		middleEntries = append(middleEntries, formatDecisionRound(rounds[i]))
	}

	// Recent rounds: last 5 (or all if N <= 5)
	var recentEntries []string
	for i := recentStart; i < n; i++ {
		recentEntries = append(recentEntries, formatVerbatimRound(rounds[i]))
	}

	result := assemblePlanHistory(oldDigests, middleEntries, recentEntries)

	// Enforce budget: drop oldest digests first, then oldest middle rounds
	for len(result) > planHistoryMaxBytes {
		if len(oldDigests) > 0 {
			oldDigests = oldDigests[1:]
			result = assemblePlanHistory(oldDigests, middleEntries, recentEntries)
			continue
		}
		if len(middleEntries) > 0 {
			middleEntries = middleEntries[1:]
			result = assemblePlanHistory(oldDigests, middleEntries, recentEntries)
			continue
		}
		break
	}

	return result
}

// formatDigestRound formats a round as a one-line digest.
func formatDigestRound(r PlanRound) string {
	resp := r.AIResponse
	if len(resp) > 80 {
		resp = resp[:80] + "…"
	}
	return fmt.Sprintf("Round %d: %s → %s", r.Round, r.UserInput, resp)
}

// formatDecisionRound formats a round as decision-only (no consultation).
func formatDecisionRound(r PlanRound) string {
	resp := r.AIResponse
	if len(resp) > 200 {
		resp = resp[:200] + "…"
	}
	return fmt.Sprintf("### Round %d (%s)\n**User:** %s\n**AI:** %s", r.Round, r.Timestamp, r.UserInput, resp)
}

// formatVerbatimRound formats a round with all fields.
func formatVerbatimRound(r PlanRound) string {
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("### Round %d (%s)\n", r.Round, r.Timestamp))
	buf.WriteString(fmt.Sprintf("**User:** %s\n", r.UserInput))

	if len(r.Consultation) > 0 {
		buf.WriteString("**Consultation:**\n")
		for _, c := range r.Consultation {
			buf.WriteString(fmt.Sprintf("- %s\n", c))
		}
	}

	buf.WriteString(fmt.Sprintf("**AI:** %s", r.AIResponse))

	if r.HasPlanDraft {
		buf.WriteString("\n*(plan draft produced)*")
	}
	if r.Finalized {
		buf.WriteString("\n*(plan finalized)*")
	}
	if r.Verification != nil {
		buf.WriteString(fmt.Sprintf("\n**Verification:** %d items", r.Verification.Items))
		if len(r.Verification.Warnings) > 0 {
			buf.WriteString(fmt.Sprintf(", warnings: %s", strings.Join(r.Verification.Warnings, "; ")))
		}
	}
	return buf.String()
}

// assemblePlanHistory joins the three compression tiers into a single string.
func assemblePlanHistory(oldDigests, middleEntries, recentEntries []string) string {
	var sections []string
	if len(oldDigests) > 0 {
		sections = append(sections, "**Early rounds (summary):**\n"+strings.Join(oldDigests, "\n"))
	}
	if len(middleEntries) > 0 {
		sections = append(sections, strings.Join(middleEntries, "\n\n"))
	}
	if len(recentEntries) > 0 {
		sections = append(sections, strings.Join(recentEntries, "\n\n"))
	}
	return strings.Join(sections, "\n\n---\n\n")
}
