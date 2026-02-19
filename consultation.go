package main

import (
	"bufio"
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
	"syscall"
	"time"
)

// ConsultTimeout is the per-framework consultation timeout.
const ConsultTimeout = 120 * time.Second

// Consultation markers
var (
	GuidanceStartPattern = regexp.MustCompile(`^<ralph>GUIDANCE_START</ralph>$`)
	GuidanceEndPattern   = regexp.MustCompile(`^<ralph>GUIDANCE_END</ralph>$`)
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
// Uses tag matching and keyword matching (requires 2+ keyword hits).
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

		// Keyword matching
		if keywords, ok := frameworkKeywords[cr.Name]; ok {
			hits := 0
			for _, kw := range keywords {
				if strings.Contains(searchText, strings.ToLower(kw)) {
					hits++
				}
			}
			score += hits
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

// runConsultSubagent spawns a provider subprocess for framework consultation.
// Returns the text between GUIDANCE_START and GUIDANCE_END markers.
func runConsultSubagent(cfg *ResolvedConfig, prompt string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	p := cfg.Config.Provider
	args, promptFile, err := buildProviderArgs(p.Args, p.PromptMode, p.PromptFlag, prompt)
	if err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, p.Command, args...)
	cmd.Dir = cfg.ProjectRoot
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 5 * time.Second

	// Setup stdin pipe for stdin mode
	var stdinPipe io.WriteCloser
	if p.PromptMode == "stdin" || p.PromptMode == "" {
		stdinPipe, err = cmd.StdinPipe()
		if err != nil {
			if promptFile != "" {
				os.Remove(promptFile)
			}
			return "", fmt.Errorf("failed to create stdin pipe: %w", err)
		}
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		if promptFile != "" {
			os.Remove(promptFile)
		}
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		if promptFile != "" {
			os.Remove(promptFile)
		}
		return "", fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		if promptFile != "" {
			os.Remove(promptFile)
		}
		return "", fmt.Errorf("failed to start consult subagent: %w", err)
	}

	if promptFile != "" {
		defer os.Remove(promptFile)
	}

	// Write prompt to stdin (for stdin mode)
	if stdinPipe != nil {
		go func() {
			defer stdinPipe.Close()
			io.WriteString(stdinPipe, prompt)
		}()
	}

	// Collect output and extract guidance
	var mu sync.Mutex
	var guidanceLines []string
	inGuidance := false

	processConsultLine := func(line string) {
		trimmed := strings.TrimSpace(line)
		if GuidanceStartPattern.MatchString(trimmed) {
			inGuidance = true
			return
		}
		if GuidanceEndPattern.MatchString(trimmed) {
			inGuidance = false
			return
		}
		if inGuidance {
			guidanceLines = append(guidanceLines, line)
		}
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		s := bufio.NewScanner(stderr)
		s.Buffer(make([]byte, 64*1024), 1024*1024)
		for s.Scan() {
			mu.Lock()
			processConsultLine(s.Text())
			mu.Unlock()
		}
	}()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		mu.Lock()
		processConsultLine(scanner.Text())
		mu.Unlock()
	}

	wg.Wait()
	cmd.Wait()

	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("consult subagent timed out after %v", timeout)
	}

	if len(guidanceLines) == 0 {
		return "", fmt.Errorf("consultation produced no guidance (missing GUIDANCE_START/GUIDANCE_END markers)")
	}

	guidance := strings.Join(guidanceLines, "\n")

	// Validate: must have at least one Source: citation
	if !strings.Contains(strings.ToLower(guidance), "source:") {
		return "", fmt.Errorf("consultation produced no source citations (likely hallucinated, didn't read actual source)")
	}

	return strings.TrimSpace(guidance), nil
}

// consultFramework runs a single framework consultation.
func consultFramework(ctx context.Context, cfg *ResolvedConfig, story *StoryDefinition, resource CachedResource, codebaseCtx *CodebaseContext) ResourceConsultation {
	start := time.Now()

	// Build acceptance criteria list
	var criteria []string
	for _, c := range story.AcceptanceCriteria {
		criteria = append(criteria, "- "+c)
	}
	criteriaStr := strings.Join(criteria, "\n")

	techStack := ""
	if codebaseCtx != nil {
		techStack = codebaseCtx.TechStack
	}

	prompt := getPrompt("consult", map[string]string{
		"framework":          resource.Name,
		"frameworkPath":      resource.Path,
		"storyId":            story.ID,
		"storyTitle":         story.Title,
		"storyDescription":   story.Description,
		"acceptanceCriteria": criteriaStr,
		"techStack":          techStack,
	})

	guidance, err := runConsultSubagent(cfg, prompt, ConsultTimeout)
	return ResourceConsultation{
		Framework: resource.Name,
		Guidance:  guidance,
		Duration:  time.Since(start),
		Error:     err,
	}
}

// consultFrameworkForFeature runs a feature-level framework consultation.
func consultFrameworkForFeature(ctx context.Context, cfg *ResolvedConfig, feature string, resource CachedResource, codebaseCtx *CodebaseContext) ResourceConsultation {
	start := time.Now()

	techStack := ""
	if codebaseCtx != nil {
		techStack = codebaseCtx.TechStack
	}

	prompt := getPrompt("consult-feature", map[string]string{
		"framework":     resource.Name,
		"frameworkPath": resource.Path,
		"feature":       feature,
		"techStack":     techStack,
	})

	guidance, err := runConsultSubagent(cfg, prompt, ConsultTimeout)
	return ResourceConsultation{
		Framework: resource.Name,
		Guidance:  guidance,
		Duration:  time.Since(start),
		Error:     err,
	}
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

// ConsultResources runs story-level consultation for relevant frameworks.
// Spawns consultants in parallel, returns results.
func ConsultResources(ctx context.Context, cfg *ResolvedConfig, story *StoryDefinition, rm *ResourceManager, codebaseCtx *CodebaseContext, featurePath string) *ConsultationResult {
	result := &ConsultationResult{}

	cached := rm.GetCachedResources()
	relevant := relevantFrameworks(story, cached, 3)
	if len(relevant) == 0 {
		return result
	}

	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, resource := range relevant {
		resource := resource // capture loop var
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Check cache first
			cacheKey := consultCacheKey(story.ID, resource.Name, resource.Commit, story.Description)
			if guidance, ok := loadCachedConsultation(featurePath, cacheKey); ok {
				mu.Lock()
				result.Consultations = append(result.Consultations, ResourceConsultation{
					Framework: resource.Name,
					Guidance:  guidance,
				})
				mu.Unlock()
				return
			}

			consultation := consultFramework(ctx, cfg, story, resource, codebaseCtx)
			mu.Lock()
			if consultation.Error != nil {
				result.FallbackPaths = append(result.FallbackPaths, resource)
			} else {
				result.Consultations = append(result.Consultations, consultation)
				// Cache successful result
				saveCachedConsultation(featurePath, cacheKey, consultation.Guidance)
			}
			mu.Unlock()
		}()
	}

	wg.Wait()

	// Sort for deterministic output
	sort.Slice(result.Consultations, func(i, j int) bool {
		return result.Consultations[i].Framework < result.Consultations[j].Framework
	})
	sort.Slice(result.FallbackPaths, func(i, j int) bool {
		return result.FallbackPaths[i].Name < result.FallbackPaths[j].Name
	})

	return result
}

// ConsultResourcesForFeature runs feature-level consultation for all cached frameworks.
// Used by ralph prd and ralph verify.
func ConsultResourcesForFeature(ctx context.Context, cfg *ResolvedConfig, feature string, rm *ResourceManager, codebaseCtx *CodebaseContext, featurePath string) *ConsultationResult {
	result := &ConsultationResult{}

	cached := rm.GetCachedResources()
	frameworks := allCachedFrameworks(cached, 3)
	if len(frameworks) == 0 {
		return result
	}

	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, resource := range frameworks {
		resource := resource
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Check cache
			cacheKey := featureConsultCacheKey(feature, resource.Name, resource.Commit)
			if guidance, ok := loadCachedConsultation(featurePath, cacheKey); ok {
				mu.Lock()
				result.Consultations = append(result.Consultations, ResourceConsultation{
					Framework: resource.Name,
					Guidance:  guidance,
				})
				mu.Unlock()
				return
			}

			consultation := consultFrameworkForFeature(ctx, cfg, feature, resource, codebaseCtx)
			mu.Lock()
			if consultation.Error != nil {
				result.FallbackPaths = append(result.FallbackPaths, resource)
			} else {
				result.Consultations = append(result.Consultations, consultation)
				saveCachedConsultation(featurePath, cacheKey, consultation.Guidance)
			}
			mu.Unlock()
		}()
	}

	wg.Wait()

	sort.Slice(result.Consultations, func(i, j int) bool {
		return result.Consultations[i].Framework < result.Consultations[j].Framework
	})
	sort.Slice(result.FallbackPaths, func(i, j int) bool {
		return result.FallbackPaths[i].Name < result.FallbackPaths[j].Name
	})

	return result
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
		sections = append(sections, "Consultation failed for these frameworks. Their source is cached locally — search for specific patterns:")
		sections = append(sections, "")
		for _, fb := range result.FallbackPaths {
			sections = append(sections, fmt.Sprintf("- **%s**: `%s` — grep for specific APIs/patterns you need", fb.Name, fb.Path))
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

// extractBetweenMarkers extracts text between GUIDANCE_START and GUIDANCE_END markers.
// Exported for testing.
func extractBetweenMarkers(text string) (string, bool) {
	lines := strings.Split(text, "\n")
	var result []string
	inGuidance := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if GuidanceStartPattern.MatchString(trimmed) {
			inGuidance = true
			continue
		}
		if GuidanceEndPattern.MatchString(trimmed) {
			if inGuidance {
				return strings.TrimSpace(strings.Join(result, "\n")), true
			}
			return "", false
		}
		if inGuidance {
			result = append(result, line)
		}
	}

	return "", false
}
