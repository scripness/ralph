package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
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
// A story must match 2+ keywords to be considered relevant.
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

// frameworkTagMap maps story tags to relevant resource names.
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

// relevantFrameworks returns cached resources relevant to a story.
// Uses tag matching, keyword matching (requires 2+ keyword hits), and
// name-based matching for auto-resolved deps without keyword entries.
// Caps at maxFrameworks.
func relevantFrameworks(story *StoryDefinition, cached []CachedResource, maxFrameworks int) []CachedResource {
	if len(cached) == 0 || story == nil {
		return nil
	}

	// Build set of candidate names from tags
	tagCandidates := make(map[string]bool)
	for _, tag := range story.Tags {
		tag = strings.ToLower(tag)
		if names, ok := frameworkTagMap[tag]; ok {
			for _, n := range names {
				tagCandidates[n] = true
			}
		}
	}

	// Build searchable text from story
	searchText := strings.ToLower(story.Title + " " + story.Description + " " + strings.Join(story.AcceptanceCriteria, " "))

	type scored struct {
		resource CachedResource
		score    int
	}

	var results []scored
	for _, cr := range cached {
		score := 0

		// Tag match = 2 points (enough to qualify on its own)
		if tagCandidates[cr.Name] {
			score += 2
		}

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
			// Check if any name variant appears in the story text.
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
// Used for feature-level consultations (PRD, verify) where all frameworks are relevant.
func allCachedFrameworks(cached []CachedResource, maxFrameworks int) []CachedResource {
	if len(cached) <= maxFrameworks {
		return cached
	}
	return cached[:maxFrameworks]
}

// consultCacheKey generates a deterministic cache key for a story+framework consultation.
func consultCacheKey(storyID, framework, commit, storyDescription string) string {
	h := sha256.New()
	h.Write([]byte(storyID + "\x00" + framework + "\x00" + commit + "\x00" + storyDescription))
	return fmt.Sprintf("%s-%s-%x", storyID, framework, h.Sum(nil)[:8])
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

