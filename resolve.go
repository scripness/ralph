package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// resolveWorkers is the number of concurrent resolution goroutines.
const resolveWorkers = 5

// resolveHTTPTimeout is the per-request timeout for registry API calls.
const resolveHTTPTimeout = 10 * time.Second

// ResolvedDep is a dependency with its resolved repo URL and exact version.
type ResolvedDep struct {
	Name    string
	Version string // exact version (e.g., "3.24.4")
	URL     string // normalized repo URL
	Tag     string // git tag to clone (e.g., "v3.24.4"), empty = default branch
}

// shouldResolve returns true if the dependency should be resolved.
func shouldResolve(dep Dependency, ecosystem string) bool {
	// Skip @types/* — DefinitelyTyped monorepo, not useful
	if strings.HasPrefix(dep.Name, "@types/") {
		return false
	}

	// Skip Go indirect deps
	if ecosystem == "go" && dep.IsDev {
		return false
	}

	// Skip workspace/file/link refs (npm local packages)
	v := dep.Version
	if strings.HasPrefix(v, "workspace:") || strings.HasPrefix(v, "file:") ||
		strings.HasPrefix(v, "link:") || strings.HasPrefix(v, "portal:") {
		return false
	}

	return true
}

// cleanVersion strips version prefixes like ^, ~, >=, ~>
func cleanVersion(v string) string {
	v = strings.TrimSpace(v)
	// Strip common prefixes
	for _, prefix := range []string{"^", "~>", "~", ">=", "<=", "==", "!=", ">", "<", "="} {
		v = strings.TrimPrefix(v, prefix)
	}
	return strings.TrimSpace(v)
}

// ecosystemFromTechStack maps tech stack names to ecosystem identifiers.
func ecosystemFromTechStack(techStack string) string {
	switch techStack {
	case "typescript", "javascript":
		return "npm"
	case "go":
		return "go"
	case "python":
		return "pypi"
	case "rust":
		return "crates"
	case "elixir":
		return "hex"
	default:
		return ""
	}
}

// normalizeRepoURL normalizes various git URL formats to HTTPS.
func normalizeRepoURL(raw string) string {
	if raw == "" {
		return ""
	}

	// Strip git+ prefix
	raw = strings.TrimPrefix(raw, "git+")

	// Convert git@ SSH URLs: git@github.com:user/repo → https://github.com/user/repo
	if strings.HasPrefix(raw, "git@") {
		raw = strings.TrimPrefix(raw, "git@")
		raw = strings.Replace(raw, ":", "/", 1)
		raw = "https://" + raw
	}

	// Convert ssh:// URLs
	if strings.HasPrefix(raw, "ssh://") {
		raw = strings.Replace(raw, "ssh://", "https://", 1)
		// Remove user@ if present
		if idx := strings.Index(raw, "@"); idx > 8 && idx < strings.Index(raw[8:], "/")+8 {
			raw = "https://" + raw[idx+1:]
		}
	}

	// Convert git:// URLs
	if strings.HasPrefix(raw, "git://") {
		raw = strings.Replace(raw, "git://", "https://", 1)
	}

	// Strip .git suffix
	raw = strings.TrimSuffix(raw, ".git")

	// Strip /tree/main/... monorepo subdirectory paths
	if idx := strings.Index(raw, "/tree/"); idx > 0 {
		raw = raw[:idx]
	}

	return raw
}

// resolveRepoURL resolves a package name to its source repo URL.
func resolveRepoURL(name, ecosystem string, client *http.Client) (string, error) {
	switch ecosystem {
	case "npm":
		return resolveNPM(name, client)
	case "go":
		return resolveGo(name)
	case "pypi":
		return resolvePyPI(name, client)
	case "crates":
		return resolveCrates(name, client)
	case "hex":
		return resolveHex(name, client)
	default:
		return "", fmt.Errorf("unsupported ecosystem: %s", ecosystem)
	}
}

// resolveNPM looks up an npm package's repository URL.
func resolveNPM(name string, client *http.Client) (string, error) {
	url := fmt.Sprintf("https://registry.npmjs.org/%s", name)
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("npm registry request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", fmt.Errorf("package not found on npm: %s", name)
	}
	if resp.StatusCode == 429 {
		return "", fmt.Errorf("npm rate limited")
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("npm registry returned %d for %s", resp.StatusCode, name)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return "", fmt.Errorf("failed to read npm response: %w", err)
	}

	// Parse repository field — can be string or {type, url} object
	var pkg struct {
		Repository json.RawMessage `json:"repository"`
	}
	if err := json.Unmarshal(body, &pkg); err != nil {
		return "", fmt.Errorf("failed to parse npm response: %w", err)
	}

	if pkg.Repository == nil {
		return "", fmt.Errorf("no repository field for npm package %s", name)
	}

	// Try string first
	var repoStr string
	if json.Unmarshal(pkg.Repository, &repoStr) == nil && repoStr != "" {
		// npm shorthand like "github:user/repo"
		if strings.HasPrefix(repoStr, "github:") {
			repoStr = "https://github.com/" + strings.TrimPrefix(repoStr, "github:")
		}
		return normalizeRepoURL(repoStr), nil
	}

	// Try object
	var repoObj struct {
		URL       string `json:"url"`
		Directory string `json:"directory"`
	}
	if json.Unmarshal(pkg.Repository, &repoObj) == nil && repoObj.URL != "" {
		return normalizeRepoURL(repoObj.URL), nil
	}

	return "", fmt.Errorf("could not parse repository field for npm package %s", name)
}

// resolveGo resolves a Go module path to a repo URL.
func resolveGo(modulePath string) (string, error) {
	// GitHub paths: truncate to github.com/user/repo
	if strings.HasPrefix(modulePath, "github.com/") {
		parts := strings.Split(modulePath, "/")
		if len(parts) >= 3 {
			return "https://" + strings.Join(parts[:3], "/"), nil
		}
	}

	// gitlab.com paths
	if strings.HasPrefix(modulePath, "gitlab.com/") {
		parts := strings.Split(modulePath, "/")
		if len(parts) >= 3 {
			return "https://" + strings.Join(parts[:3], "/"), nil
		}
	}

	// Vanity URL — try ?go-get=1 meta tag resolution
	url, err := resolveGoVanity(modulePath)
	if err != nil {
		return "", err
	}
	return normalizeRepoURL(url), nil
}

// resolveGoVanity resolves a Go vanity import via go-get=1 meta tag.
func resolveGoVanity(modulePath string) (string, error) {
	client := &http.Client{Timeout: resolveHTTPTimeout}
	resp, err := client.Get("https://" + modulePath + "?go-get=1")
	if err != nil {
		return "", fmt.Errorf("vanity URL resolution failed for %s: %w", modulePath, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return "", err
	}

	// Look for <meta name="go-import" content="prefix vcs repoURL">
	re := regexp.MustCompile(`<meta\s+name="go-import"\s+content="([^"]+)"`)
	matches := re.FindSubmatch(body)
	if matches == nil {
		return "", fmt.Errorf("no go-import meta tag found for %s", modulePath)
	}

	fields := strings.Fields(string(matches[1]))
	if len(fields) < 3 {
		return "", fmt.Errorf("invalid go-import meta for %s", modulePath)
	}
	return fields[2], nil
}

// resolvePyPI looks up a Python package's repository URL.
func resolvePyPI(name string, client *http.Client) (string, error) {
	url := fmt.Sprintf("https://pypi.org/pypi/%s/json", name)
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("pypi request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", fmt.Errorf("package not found on pypi: %s", name)
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("pypi returned %d for %s", resp.StatusCode, name)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	var pkg struct {
		Info struct {
			ProjectURLs map[string]string `json:"project_urls"`
		} `json:"info"`
	}
	if err := json.Unmarshal(body, &pkg); err != nil {
		return "", err
	}

	// Try common key names for source repo
	for _, key := range []string{"Source", "Repository", "GitHub", "Source Code", "Homepage", "Code"} {
		if url, ok := pkg.Info.ProjectURLs[key]; ok && strings.Contains(url, "github.com") {
			return normalizeRepoURL(url), nil
		}
	}

	// Try any URL containing github.com
	for _, url := range pkg.Info.ProjectURLs {
		if strings.Contains(url, "github.com") || strings.Contains(url, "gitlab.com") {
			return normalizeRepoURL(url), nil
		}
	}

	return "", fmt.Errorf("no repository URL found for pypi package %s", name)
}

// resolveCrates looks up a Rust crate's repository URL.
func resolveCrates(name string, client *http.Client) (string, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("https://crates.io/api/v1/crates/%s", name), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "ralph-cli (https://github.com/scripness/ralph)")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("crates.io request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", fmt.Errorf("crate not found: %s", name)
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("crates.io returned %d for %s", resp.StatusCode, name)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	var crate struct {
		Crate struct {
			Repository string `json:"repository"`
		} `json:"crate"`
	}
	if err := json.Unmarshal(body, &crate); err != nil {
		return "", err
	}

	if crate.Crate.Repository == "" {
		return "", fmt.Errorf("no repository URL for crate %s", name)
	}

	return normalizeRepoURL(crate.Crate.Repository), nil
}

// resolveHex looks up an Elixir/Erlang package's repository URL.
func resolveHex(name string, client *http.Client) (string, error) {
	resp, err := client.Get(fmt.Sprintf("https://hex.pm/api/packages/%s", name))
	if err != nil {
		return "", fmt.Errorf("hex.pm request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", fmt.Errorf("package not found on hex.pm: %s", name)
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("hex.pm returned %d for %s", resp.StatusCode, name)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	var pkg struct {
		Meta struct {
			Links map[string]string `json:"links"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &pkg); err != nil {
		return "", err
	}

	for _, key := range []string{"GitHub", "Source", "Repository"} {
		if url, ok := pkg.Meta.Links[key]; ok {
			return normalizeRepoURL(url), nil
		}
	}

	// Try any URL containing github.com or gitlab.com
	for _, url := range pkg.Meta.Links {
		if strings.Contains(url, "github.com") || strings.Contains(url, "gitlab.com") {
			return normalizeRepoURL(url), nil
		}
	}

	return "", fmt.Errorf("no repository URL found for hex package %s", name)
}

// resolveExactVersions extracts exact versions from lock files.
// Falls back to cleaning manifest version specs if no lock file is available.
func resolveExactVersions(deps []Dependency, ecosystem, projectRoot string) map[string]string {
	versions := make(map[string]string)

	switch ecosystem {
	case "npm":
		versions = resolveNPMVersions(deps, projectRoot)
	case "go":
		// Go module versions in go.mod are exact
		for _, d := range deps {
			v := strings.TrimPrefix(d.Version, "v")
			versions[d.Name] = v
		}
	default:
		// Python, Rust, Elixir: clean manifest spec
		for _, d := range deps {
			if d.Version != "" {
				versions[d.Name] = cleanVersion(d.Version)
			}
		}
	}

	return versions
}

// resolveNPMVersions resolves exact versions from bun.lock or package-lock.json.
func resolveNPMVersions(deps []Dependency, projectRoot string) map[string]string {
	versions := make(map[string]string)

	// Try lock files in priority order
	lockParsers := []func(string) map[string]string{
		parseBunLock,
		parsePackageLock,
		parseYarnLock,
		parsePnpmLock,
	}
	for _, parser := range lockParsers {
		if v := parser(projectRoot); len(v) > 0 {
			for _, d := range deps {
				if exact, ok := v[d.Name]; ok {
					versions[d.Name] = exact
				} else if d.Version != "" {
					versions[d.Name] = cleanVersion(d.Version)
				}
			}
			return versions
		}
	}

	// Fallback: clean manifest versions
	for _, d := range deps {
		if d.Version != "" {
			versions[d.Name] = cleanVersion(d.Version)
		}
	}
	return versions
}

// parseBunLock parses bun.lock (JSONC format) to extract package versions.
// bun.lock format: { "packages": { "name@version": [...] } } at top level,
// but the actual format is a JSONC array where packages is a map with
// key "pkgName@spec" and value array where first element is "name@version".
func parseBunLock(projectRoot string) map[string]string {
	data, err := os.ReadFile(filepath.Join(projectRoot, "bun.lock"))
	if err != nil {
		return nil
	}

	// bun.lock is JSONC — strip comments for parsing
	content := stripJSONComments(string(data))

	var lockfile struct {
		Packages map[string]json.RawMessage `json:"packages"`
	}
	if err := json.Unmarshal([]byte(content), &lockfile); err != nil {
		return nil
	}

	versions := make(map[string]string)
	for key := range lockfile.Packages {
		// Key format: "name@version" (e.g., "zod@3.24.4" or "@prisma/client@5.22.0")
		name, version := splitAtVersion(key)
		if name != "" && version != "" {
			versions[name] = version
		}
	}
	return versions
}

// stripJSONComments removes // line comments from JSONC content.
func stripJSONComments(s string) string {
	lines := strings.Split(s, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

// splitAtVersion splits "name@version" into (name, version).
// Handles scoped packages like "@scope/name@version".
func splitAtVersion(s string) (string, string) {
	// Find the last @ that isn't at position 0 (scoped packages start with @)
	lastAt := strings.LastIndex(s, "@")
	if lastAt <= 0 {
		return s, ""
	}

	// For scoped packages, the first @ is the scope prefix
	// "@scope/name@version" → lastAt points to the version separator
	name := s[:lastAt]
	version := s[lastAt+1:]

	// Skip non-version entries (workspace:, file:, etc.)
	if strings.ContainsAny(version, ":") {
		return "", ""
	}

	return name, version
}

// parsePackageLock parses package-lock.json (v3) to extract package versions.
func parsePackageLock(projectRoot string) map[string]string {
	data, err := os.ReadFile(filepath.Join(projectRoot, "package-lock.json"))
	if err != nil {
		return nil
	}

	var lockfile struct {
		Packages map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(data, &lockfile); err != nil {
		return nil
	}

	versions := make(map[string]string)
	for key, pkg := range lockfile.Packages {
		// Key format: "node_modules/name" or "node_modules/@scope/name"
		if !strings.HasPrefix(key, "node_modules/") {
			continue
		}
		name := strings.TrimPrefix(key, "node_modules/")
		// Skip nested node_modules
		if strings.Contains(name, "node_modules/") {
			continue
		}
		if pkg.Version != "" {
			versions[name] = pkg.Version
		}
	}
	return versions
}

// parseYarnLock parses yarn.lock (v1 or berry) to extract package versions.
// yarn.lock v1 format:
//
//	"name@spec":
//	  version "1.2.3"
//
// yarn.lock berry (v2+) format:
//
//	"name@npm:spec":
//	  version: 1.2.3
func parseYarnLock(projectRoot string) map[string]string {
	data, err := os.ReadFile(filepath.Join(projectRoot, "yarn.lock"))
	if err != nil {
		return nil
	}

	versions := make(map[string]string)
	lines := strings.Split(string(data), "\n")

	var currentNames []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Package header line — not indented, ends with ":"
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && strings.HasSuffix(trimmed, ":") {
			currentNames = parseYarnLockHeader(strings.TrimSuffix(trimmed, ":"))
			continue
		}

		// Version line — indented, starts with "version"
		if len(currentNames) > 0 && strings.HasPrefix(trimmed, "version") {
			version := extractYarnVersion(trimmed)
			if version != "" {
				for _, name := range currentNames {
					versions[name] = version
				}
			}
			currentNames = nil
		}
	}

	return versions
}

// parseYarnLockHeader extracts package names from a yarn.lock header line.
// Handles: "zod@^3.22.0", "@prisma/client@^5.0.0, @prisma/client@^5.1.0"
// Berry format: "zod@npm:^3.22.0", "@prisma/client@npm:^5.0.0"
func parseYarnLockHeader(header string) []string {
	seen := make(map[string]bool)
	var names []string

	// Split on ", " for multiple version specs
	specs := strings.Split(header, ", ")
	for _, spec := range specs {
		spec = strings.Trim(spec, "\"")

		// Strip npm: protocol prefix (yarn berry): "zod@npm:^3.22.0" → "zod@^3.22.0"
		if idx := strings.Index(spec, "@npm:"); idx >= 0 {
			spec = spec[:idx] + "@" + spec[idx+5:]
		}

		name, _ := splitAtVersion(spec)
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}

	return names
}

// extractYarnVersion extracts the version from a yarn.lock version line.
// Handles: '  version "3.24.4"' (v1) and '  version: 3.24.4' (berry)
func extractYarnVersion(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "version")
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, ":")
	line = strings.TrimSpace(line)
	line = strings.Trim(line, "\"")
	return line
}

// parsePnpmLock parses pnpm-lock.yaml to extract package versions.
// pnpm-lock.yaml format (v6+):
//
//	packages:
//	  /name@version:
//	    ...
//
// v9+ format:
//
//	packages:
//	  name@version:
//	    ...
func parsePnpmLock(projectRoot string) map[string]string {
	data, err := os.ReadFile(filepath.Join(projectRoot, "pnpm-lock.yaml"))
	if err != nil {
		return nil
	}

	versions := make(map[string]string)
	lines := strings.Split(string(data), "\n")

	inPackages := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Find the packages: section
		if trimmed == "packages:" {
			inPackages = true
			continue
		}

		// Detect end of packages section (new top-level key)
		if inPackages && len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			break
		}

		if !inPackages {
			continue
		}

		// Package entry lines are indented exactly 2 or 4 spaces and end with ":"
		// Format: "  /name@version:" (v6-v8) or "  name@version:" (v9+)
		if !strings.HasSuffix(trimmed, ":") {
			continue
		}

		entry := strings.TrimSuffix(trimmed, ":")
		// Count leading spaces to determine indentation level
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent != 2 && indent != 4 {
			continue
		}

		// Strip YAML quotes (single or double)
		entry = strings.Trim(entry, "'\"")

		// Strip leading / (v6-v8 format)
		entry = strings.TrimPrefix(entry, "/")

		// Skip entries with parenthesized peer deps like "name@version(peer@ver)"
		if idx := strings.Index(entry, "("); idx > 0 {
			entry = entry[:idx]
		}

		name, version := splitAtVersion(entry)
		if name != "" && version != "" {
			versions[name] = version
		}
	}

	return versions
}

// findVersionTag checks remote tags to find a matching tag for the given version.
// Returns the tag name (e.g., "v3.24.4") or empty string if no match.
func findVersionTag(repoURL, version string) string {
	if version == "" {
		return ""
	}

	cmd := exec.Command("git", "ls-remote", "--tags", repoURL)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Parse tags from ls-remote output
	var tags []string
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		ref := fields[1]
		// Skip ^{} dereferenced tags
		if strings.HasSuffix(ref, "^{}") {
			continue
		}
		tag := strings.TrimPrefix(ref, "refs/tags/")
		tags = append(tags, tag)
	}

	// Try exact matches in priority order
	candidates := []string{
		"v" + version,    // v3.24.4
		version,          // 3.24.4
	}
	for _, c := range candidates {
		for _, tag := range tags {
			if tag == c {
				return tag
			}
		}
	}

	// Try name@version format (monorepo convention)
	for _, tag := range tags {
		if strings.HasSuffix(tag, "@"+version) {
			return tag
		}
	}

	return ""
}

// dependencyNameVariants generates searchable name variants for a dep.
// "@scope/name" → ["scope", "name"], "pino" → ["pino"]
func dependencyNameVariants(name string) []string {
	var variants []string

	if strings.HasPrefix(name, "@") {
		// Scoped package: @scope/name → [scope, name]
		trimmed := strings.TrimPrefix(name, "@")
		parts := strings.SplitN(trimmed, "/", 2)
		for _, p := range parts {
			if p != "" {
				variants = append(variants, strings.ToLower(p))
			}
		}
	} else {
		variants = append(variants, strings.ToLower(name))
	}

	return variants
}

// ResolveAll resolves repo URLs and versions for all dependencies.
// Uses a worker pool with concurrency limiting and deduplication.
func ResolveAll(deps []Dependency, ecosystem, projectRoot string, registry *ResourceRegistry) []ResolvedDep {
	if len(deps) == 0 {
		return nil
	}

	// Resolve exact versions first (local operation, no network)
	versions := resolveExactVersions(deps, ecosystem, projectRoot)

	// Filter deps
	var toResolve []Dependency
	for _, dep := range deps {
		if !shouldResolve(dep, ecosystem) {
			continue
		}
		// Skip if already known to be unresolvable
		if registry != nil && registry.IsUnresolvable(dep.Name) {
			continue
		}
		toResolve = append(toResolve, dep)
	}

	if len(toResolve) == 0 {
		return nil
	}

	client := &http.Client{Timeout: resolveHTTPTimeout}

	type resolveResult struct {
		dep ResolvedDep
		err error
	}

	results := make(chan resolveResult, len(toResolve))
	work := make(chan Dependency, len(toResolve))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < resolveWorkers && i < len(toResolve); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for dep := range work {
				version := versions[dep.Name]
				cacheKey := dep.Name + "@" + version

				// Check if already cached in registry
				if registry != nil {
					if repo := registry.GetRepo(cacheKey); repo != nil {
						results <- resolveResult{dep: ResolvedDep{
							Name:    dep.Name,
							Version: version,
							URL:     repo.URL,
							Tag:     repo.Tag,
						}}
						continue
					}
				}

				// Check URL cache in registry
				var repoURL string
				if registry != nil {
					if url, ok := registry.GetResolvedURL(dep.Name); ok {
						repoURL = url
					}
				}

				// Resolve URL if not cached
				if repoURL == "" {
					url, err := resolveRepoURL(dep.Name, ecosystem, client)
					if err != nil {
						results <- resolveResult{err: fmt.Errorf("%s: %w", dep.Name, err)}
						continue
					}
					repoURL = url
					if registry != nil {
						registry.SetResolvedURL(dep.Name, repoURL)
					}
				}

				results <- resolveResult{dep: ResolvedDep{
					Name:    dep.Name,
					Version: version,
					URL:     repoURL,
				}}
			}
		}()
	}

	// Send work
	for _, dep := range toResolve {
		work <- dep
	}
	close(work)

	// Collect results
	go func() {
		wg.Wait()
		close(results)
	}()

	// Deduplicate by normalized URL (monorepo deps → single clone)
	seen := make(map[string]bool)
	var resolved []ResolvedDep
	var unresolved int
	for r := range results {
		if r.err != nil {
			unresolved++
			// Mark unresolvable in registry (404, no repo field, etc.)
			depName := strings.SplitN(r.err.Error(), ":", 2)[0]
			if registry != nil {
				registry.MarkUnresolvable(depName)
			}
			continue
		}
		key := r.dep.Name + "@" + r.dep.Version
		if seen[key] {
			continue
		}
		seen[key] = true
		resolved = append(resolved, r.dep)
	}

	return resolved
}
