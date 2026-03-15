package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// ConsultTimeout is the per-framework consultation timeout.
const ConsultTimeout = 120 * time.Second

// Consultation markers
var (
	GuidanceStartPattern = regexp.MustCompile(`^<scrip>GUIDANCE_START</scrip>$`)
	GuidanceEndPattern   = regexp.MustCompile(`^<scrip>GUIDANCE_END</scrip>$`)
)

// ResourceConsultation is the result of consulting one framework.
type ResourceConsultation struct {
	Framework string
	Guidance  string        // extracted guidance text (between markers)
	Duration  time.Duration // how long the consultation took
	Error     error         // non-nil if consultation failed
}

// ConsultationResult collects all consultation results.
type ConsultationResult struct {
	Consultations []ResourceConsultation
	FallbackPaths []CachedResource // frameworks where consultation failed
}

// frameworkKeywords maps resource names to keywords that indicate relevance.
// An item must match 2+ keywords to be considered relevant.
var frameworkKeywords = map[string][]string{
	// Frontend
	"next": {"next", "app router", "server action", "server component", "client component",
		"middleware", "layout", "page.tsx", "route handler", "api route", "getServerSideProps",
		"getStaticProps", "revalidate", "next/image", "next/link", "next/font", "ssr", "ssg"},
	"react": {"react", "component", "hook", "useState", "useEffect", "useRef", "useContext",
		"jsx", "tsx", "props", "state", "render", "virtual dom", "context provider"},
	"svelte": {"svelte", "sveltekit", "svelte component", "+page", "+layout", "+server",
		"$:", "reactive", "store", "action"},
	"@sveltejs/kit": {"sveltekit", "kit", "+page", "+layout", "+server", "load function",
		"form action", "hooks"},
	"vue": {"vue", "composition api", "options api", "ref", "reactive", "computed",
		"template", "directive", "v-model", "v-if", "v-for"},
	"nuxt": {"nuxt", "nuxt3", "useFetch", "useAsyncData", "middleware", "plugin",
		"defineNuxtConfig", "server/api"},
	"angular": {"angular", "component", "service", "module", "directive", "pipe",
		"injectable", "ngModule", "rxjs", "observable"},

	// Styling
	"tailwindcss": {"tailwind", "className", "utility class", "responsive", "dark mode",
		"tailwind.config"},

	// Backend JS/TS
	"hono": {"hono", "middleware", "route", "c.json", "c.text", "c.html"},
	"fastify": {"fastify", "route", "plugin", "schema", "hook", "decorator"},
	"express": {"express", "middleware", "router", "req", "res", "app.get", "app.post"},
	"koa": {"koa", "middleware", "ctx", "router"},

	// ORM / Database
	"prisma":          {"prisma", "schema.prisma", "prisma client", "migration", "model", "relation", "findMany", "findUnique", "create", "update", "delete", "upsert"},
	"@prisma/client":  {"prisma", "prisma client", "findMany", "findUnique", "create", "update"},
	"drizzle-orm":     {"drizzle", "schema", "migration", "select", "insert", "table", "column", "relation", "pgTable", "sqliteTable"},

	// Testing
	"vitest":           {"vitest", "describe", "it(", "expect", "test(", "vi.mock", "vi.fn"},
	"playwright":       {"playwright", "e2e", "page.goto", "page.click", "locator", "expect(page"},
	"@playwright/test": {"playwright", "e2e", "page.goto", "page.click", "locator", "expect(page"},
	"jest":             {"jest", "describe", "it(", "expect", "test(", "mock"},

	// Validation
	"zod": {"zod", "z.object", "z.string", "z.number", "parse", "safeParse", "schema validation"},

	// Build Tools
	"vite":    {"vite", "vite.config", "hmr", "plugin", "build"},
	"esbuild": {"esbuild", "bundle", "minify", "build"},
	"webpack": {"webpack", "loader", "plugin", "bundle", "webpack.config"},

	// State Management
	"zustand": {"zustand", "create store", "useStore", "set", "get", "state management"},
	"jotai":   {"jotai", "atom", "useAtom", "primitive atom"},

	// Elixir/Phoenix
	"phoenix":           {"phoenix", "router", "controller", "view", "channel", "socket", "endpoint", "plug", "conn"},
	"phoenix_live_view": {"live view", "liveview", "mount", "handle_event", "handle_info", "socket", "assign", "live_component"},
	"ecto":              {"ecto", "schema", "changeset", "migration", "repo", "query", "has_many", "belongs_to"},
	"phoenix_html":      {"phoenix html", "form", "input", "link", "heex", "sigil_H"},
	"absinthe":          {"absinthe", "graphql", "query", "mutation", "resolver", "schema"},
	"oban":              {"oban", "job", "worker", "queue", "perform", "schedule"},

	// Go Frameworks
	"gin":   {"gin", "router", "handler", "context", "middleware", "c.JSON", "c.Bind"},
	"echo":  {"echo", "handler", "middleware", "context", "e.GET", "e.POST"},
	"fiber": {"fiber", "handler", "middleware", "c.JSON", "app.Get", "app.Post"},
	"chi":   {"chi", "router", "handler", "middleware", "r.Get", "r.Post"},
}

// frameworkTagMap maps item tags to relevant resource names.
var frameworkTagMap = map[string][]string{
	"ui":       {"next", "react", "svelte", "@sveltejs/kit", "vue", "nuxt", "angular", "tailwindcss", "phoenix_live_view"},
	"db":       {"prisma", "@prisma/client", "drizzle-orm", "ecto"},
	"api":      {"express", "fastify", "hono", "koa", "gin", "echo", "fiber", "chi", "phoenix"},
	"test":     {"vitest", "playwright", "@playwright/test", "jest"},
	"e2e":      {"playwright", "@playwright/test"},
	"graphql":  {"absinthe"},
	"jobs":     {"oban"},
	"queue":    {"oban"},
	"realtime": {"phoenix", "phoenix_live_view"},
}

// relevantFrameworks returns cached resources relevant to a plan item.
// Uses keyword matching (requires 2+ keyword hits) and name-based matching
// for auto-resolved deps without keyword entries. Caps at maxFrameworks.
func relevantFrameworks(item *PlanItem, cached []CachedResource, maxFrameworks int) []CachedResource {
	if len(cached) == 0 || item == nil {
		return nil
	}

	// Build searchable text from item
	searchText := strings.ToLower(item.Title + " " + strings.Join(item.Acceptance, " "))

	type scored struct {
		resource CachedResource
		score    int
	}

	var results []scored
	for _, cr := range cached {
		score := 0

		// Keyword matching (for deps with keyword entries)
		if keywords, ok := frameworkKeywords[cr.Name]; ok {
			hits := 0
			for _, kw := range keywords {
				if strings.Contains(searchText, strings.ToLower(kw)) {
					hits++
				}
			}
			score += hits
		} else {
			// Name-based matching for auto-resolved deps without keyword entries.
			// Check if any name variant appears in the item text.
			variants := dependencyNameVariants(cr.Name)
			for _, v := range variants {
				if len(v) >= 3 && strings.Contains(searchText, v) {
					score++
				}
			}
		}

		// Require at least 2 to include
		if score >= 2 {
			results = append(results, scored{resource: cr, score: score})
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		return results[i].resource.Name < results[j].resource.Name
	})

	// Cap
	if len(results) > maxFrameworks {
		results = results[:maxFrameworks]
	}

	out := make([]CachedResource, len(results))
	for i, r := range results {
		out[i] = r.resource
	}
	return out
}

// allCachedFrameworks returns all cached resources, capped at maxFrameworks.
// Used for feature-level consultations (plan, land) where all frameworks are relevant.
func allCachedFrameworks(cached []CachedResource, maxFrameworks int) []CachedResource {
	if len(cached) <= maxFrameworks {
		return cached
	}
	return cached[:maxFrameworks]
}

// consultCacheKey generates a deterministic cache key for an item+framework consultation.
func consultCacheKey(itemID, framework, commit, itemDescription string) string {
	h := sha256.New()
	h.Write([]byte(itemID + "\x00" + framework + "\x00" + commit + "\x00" + itemDescription))
	return fmt.Sprintf("%s-%s-%x", itemID, framework, h.Sum(nil)[:8])
}

// featureConsultCacheKey generates a cache key for feature-level consultation.
func featureConsultCacheKey(feature, framework, commit string) string {
	h := sha256.New()
	h.Write([]byte(feature + "\x00" + framework + "\x00" + commit))
	return fmt.Sprintf("feature-%s-%s-%x", feature, framework, h.Sum(nil)[:8])
}

// loadCachedConsultation reads a cached consultation result from disk.
func loadCachedConsultation(featurePath, cacheKey string) (string, bool) {
	path := filepath.Join(featurePath, "consultations", cacheKey+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}

// saveCachedConsultation writes a consultation result to disk.
func saveCachedConsultation(featurePath, cacheKey, guidance string) {
	dir := filepath.Join(featurePath, "consultations")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, cacheKey+".md"), []byte(guidance), 0644)
}

// FormatGuidance formats consultation results for prompt injection.
func FormatGuidance(result *ConsultationResult) string {
	if result == nil || (len(result.Consultations) == 0 && len(result.FallbackPaths) == 0) {
		return buildResourceFallbackInstructions()
	}

	var sections []string

	// Successful consultations
	if len(result.Consultations) > 0 {
		sections = append(sections, "## Framework Implementation Guidance")
		sections = append(sections, "")
		sections = append(sections, "The following guidance was generated from cached framework source code. Use it to ensure correct API usage and patterns.")
		sections = append(sections, "")

		for _, c := range result.Consultations {
			sections = append(sections, fmt.Sprintf("### %s", c.Framework))
			sections = append(sections, "")
			sections = append(sections, c.Guidance)
			sections = append(sections, "")
		}
	}

	// Fallback for failed consultations
	if len(result.FallbackPaths) > 0 {
		sections = append(sections, "## Additional Framework References")
		sections = append(sections, "")
		sections = append(sections, "Source code is cached locally for these frameworks — search for specific patterns:")
		sections = append(sections, "")
		for _, fb := range result.FallbackPaths {
			label := fb.Name
			if fb.Version != "" {
				label = fb.Name + " v" + fb.Version
			}
			sections = append(sections, fmt.Sprintf("- **%s**: `%s` — search this source for APIs/patterns you need", label, fb.Path))
		}
		sections = append(sections, "")
	}

	return strings.Join(sections, "\n")
}

// buildResourceFallbackInstructions returns web search fallback instructions.
// Used when no consultation is available (resources disabled, no cache, no relevant frameworks).
func buildResourceFallbackInstructions() string {
	return `## Documentation Verification

Before committing, verify your implementation against current official documentation using web search:

- Search for the official docs of any library or framework you used
- Confirm APIs you used are current and not deprecated
- Verify configuration patterns follow current best practices
- Check security patterns (input validation, auth, etc.) are up to date

Do not rely on memory alone — docs change between versions. Verify against the latest.
`
}

// extractGuidance extracts text between GUIDANCE_START and GUIDANCE_END markers.
// Returns the extracted guidance and true if markers were found and content is non-empty.
func extractGuidance(output string) (string, bool) {
	lines := strings.Split(output, "\n")
	var inGuidance bool
	var guidance []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if GuidanceStartPattern.MatchString(trimmed) {
			inGuidance = true
			guidance = nil // reset in case of multiple starts
			continue
		}
		if GuidanceEndPattern.MatchString(trimmed) {
			if inGuidance {
				result := strings.TrimSpace(strings.Join(guidance, "\n"))
				if result != "" {
					return result, true
				}
			}
			return "", false
		}
		if inGuidance {
			guidance = append(guidance, line)
		}
	}
	return "", false
}

// hasCitations checks if guidance contains at least one Source: citation line.
// Guidance without citations is treated as hallucination per the spec.
func hasCitations(guidance string) bool {
	for _, line := range strings.Split(guidance, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Source:") {
			return true
		}
	}
	return false
}

// generateConsultItemPrompt builds the consult-item.md prompt for per-item consultation.
func generateConsultItemPrompt(cr CachedResource, item *PlanItem, itemIndex int, techStack string) string {
	itemID := fmt.Sprintf("item-%d", itemIndex+1)

	var criteriaLines []string
	for _, c := range item.Acceptance {
		criteriaLines = append(criteriaLines, "- "+c)
	}

	return getPrompt("consult-item", map[string]string{
		"framework":          cr.Name,
		"frameworkPath":      cr.Path,
		"itemId":             itemID,
		"itemTitle":          item.Title,
		"itemDescription":    strings.Join(item.Acceptance, "\n"),
		"techStack":          techStack,
		"acceptanceCriteria": strings.Join(criteriaLines, "\n"),
	})
}

// runConsultSubagent spawns a non-autonomous claude subagent for one framework consultation.
// Uses a 30-second per-subagent timeout. Returns the consultation result (may have Error set).
func runConsultSubagent(projectRoot string, cr CachedResource, item *PlanItem, itemIndex int, techStack string) ResourceConsultation {
	start := time.Now()
	prompt := generateConsultItemPrompt(cr, item, itemIndex, techStack)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	args := ScripProviderArgs(false)
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = projectRoot
	cmd.Stdin = strings.NewReader(prompt)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard

	if err := cmd.Run(); err != nil {
		return ResourceConsultation{
			Framework: cr.Name,
			Duration:  time.Since(start),
			Error:     fmt.Errorf("consultation subagent failed: %w", err),
		}
	}

	guidance, ok := extractGuidance(stdout.String())
	if !ok || guidance == "" {
		return ResourceConsultation{
			Framework: cr.Name,
			Duration:  time.Since(start),
			Error:     fmt.Errorf("no guidance markers in output"),
		}
	}

	if !hasCitations(guidance) {
		return ResourceConsultation{
			Framework: cr.Name,
			Duration:  time.Since(start),
			Error:     fmt.Errorf("guidance lacks citations — treated as hallucination"),
		}
	}

	return ResourceConsultation{
		Framework: cr.Name,
		Guidance:  guidance,
		Duration:  time.Since(start),
	}
}

// generateConsultFeaturePrompt builds the consult-feature.md prompt for feature-level consultation.
func generateConsultFeaturePrompt(cr CachedResource, feature, techStack string) string {
	label := cr.Name
	if cr.Version != "" {
		label = cr.Name + " v" + cr.Version
	}

	return getPrompt("consult-feature", map[string]string{
		"framework":     label,
		"frameworkPath": cr.Path,
		"feature":       feature,
		"techStack":     techStack,
	})
}

// runFeatureConsultSubagent spawns a non-autonomous claude subagent for one framework's feature-level consultation.
func runFeatureConsultSubagent(projectRoot string, cr CachedResource, feature, techStack string) ResourceConsultation {
	start := time.Now()
	prompt := generateConsultFeaturePrompt(cr, feature, techStack)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	args := ScripProviderArgs(false)
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = projectRoot
	cmd.Stdin = strings.NewReader(prompt)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard

	if err := cmd.Run(); err != nil {
		return ResourceConsultation{
			Framework: cr.Name,
			Duration:  time.Since(start),
			Error:     fmt.Errorf("consultation subagent failed: %w", err),
		}
	}

	guidance, ok := extractGuidance(stdout.String())
	if !ok || guidance == "" {
		return ResourceConsultation{
			Framework: cr.Name,
			Duration:  time.Since(start),
			Error:     fmt.Errorf("no guidance markers in output"),
		}
	}

	if !hasCitations(guidance) {
		return ResourceConsultation{
			Framework: cr.Name,
			Duration:  time.Since(start),
			Error:     fmt.Errorf("guidance lacks citations — treated as hallucination"),
		}
	}

	return ResourceConsultation{
		Framework: cr.Name,
		Guidance:  guidance,
		Duration:  time.Since(start),
	}
}

// consultForFeature runs feature-level consultation with all cached frameworks.
// Spawns subagents in parallel, caches results, and returns formatted guidance
// for injection into plan/land prompts. Falls back to web search instructions
// when no frameworks are cached or all consultations fail.
func consultForFeature(projectRoot string, featureDir *FeatureDir, rm *ResourceManager, techStack string, logger *RunLogger) string {
	cached := rm.GetCachedResources()
	frameworks := allCachedFrameworks(cached, 5)

	if len(frameworks) == 0 {
		return buildResourceFallbackInstructions()
	}

	git := NewGitOps(projectRoot)
	commit := git.GetLastCommit()

	result := &ConsultationResult{}

	type consultJob struct {
		cr       CachedResource
		cacheKey string
	}
	var jobs []consultJob

	for _, cr := range frameworks {
		cacheKey := featureConsultCacheKey(featureDir.Feature, cr.Name, commit)

		if cachedGuidance, ok := loadCachedConsultation(featureDir.Path, cacheKey); ok {
			result.Consultations = append(result.Consultations, ResourceConsultation{
				Framework: cr.Name,
				Guidance:  cachedGuidance,
			})
			continue
		}

		jobs = append(jobs, consultJob{cr: cr, cacheKey: cacheKey})
	}

	if len(jobs) > 0 {
		var wg sync.WaitGroup
		var mu sync.Mutex

		for _, job := range jobs {
			wg.Add(1)
			go func(j consultJob) {
				defer wg.Done()
				consultation := runFeatureConsultSubagent(projectRoot, j.cr, featureDir.Feature, techStack)

				mu.Lock()
				defer mu.Unlock()
				if consultation.Error != nil || consultation.Guidance == "" {
					result.FallbackPaths = append(result.FallbackPaths, j.cr)
					if logger != nil && consultation.Error != nil {
						logger.Warning(fmt.Sprintf("feature consultation for %s: %v", j.cr.Name, consultation.Error))
					}
				} else {
					result.Consultations = append(result.Consultations, consultation)
					saveCachedConsultation(featureDir.Path, j.cacheKey, consultation.Guidance)
				}
			}(job)
		}

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(ConsultTimeout):
			if logger != nil {
				logger.Warning("feature consultation budget exceeded — proceeding with partial results")
			}
		}
	}

	return FormatGuidance(result)
}

// consultForItem runs per-item consultation with relevant cached frameworks.
// Spawns subagents in parallel, caches results, and returns formatted guidance
// for injection into the exec-build prompt. Falls back to web search instructions
// when no frameworks are relevant or all consultations fail.
func consultForItem(projectRoot string, featureDir *FeatureDir, item *PlanItem, itemIndex int, rm *ResourceManager, techStack string, logger *RunLogger) string {
	cached := rm.GetCachedResources()
	relevant := relevantFrameworks(item, cached, 5)

	if len(relevant) == 0 {
		return buildResourceFallbackInstructions()
	}

	git := NewGitOps(projectRoot)
	commit := git.GetLastCommit()
	itemID := fmt.Sprintf("item-%d", itemIndex+1)
	itemDescription := strings.Join(item.Acceptance, "\n")

	result := &ConsultationResult{}

	// Check cache for each relevant framework; collect uncached jobs
	type consultJob struct {
		cr       CachedResource
		cacheKey string
	}
	var jobs []consultJob

	for _, cr := range relevant {
		cacheKey := consultCacheKey(itemID, cr.Name, commit, itemDescription)

		if cachedGuidance, ok := loadCachedConsultation(featureDir.Path, cacheKey); ok {
			result.Consultations = append(result.Consultations, ResourceConsultation{
				Framework: cr.Name,
				Guidance:  cachedGuidance,
			})
			continue
		}

		jobs = append(jobs, consultJob{cr: cr, cacheKey: cacheKey})
	}

	// Run uncached consultations in parallel
	if len(jobs) > 0 {
		var wg sync.WaitGroup
		var mu sync.Mutex

		for _, job := range jobs {
			wg.Add(1)
			go func(j consultJob) {
				defer wg.Done()
				consultation := runConsultSubagent(projectRoot, j.cr, item, itemIndex, techStack)

				mu.Lock()
				defer mu.Unlock()
				if consultation.Error != nil || consultation.Guidance == "" {
					result.FallbackPaths = append(result.FallbackPaths, j.cr)
					if logger != nil && consultation.Error != nil {
						logger.Warning(fmt.Sprintf("consultation for %s: %v", j.cr.Name, consultation.Error))
					}
				} else {
					result.Consultations = append(result.Consultations, consultation)
					saveCachedConsultation(featureDir.Path, j.cacheKey, consultation.Guidance)
				}
			}(job)
		}

		// Wait with total budget timeout
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// All consultations finished
		case <-time.After(ConsultTimeout):
			if logger != nil {
				logger.Warning("consultation budget exceeded — proceeding with partial results")
			}
		}
	}

	return FormatGuidance(result)
}

