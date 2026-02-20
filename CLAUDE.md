# CLAUDE.md

## Project Overview

Ralph v2 is a Go CLI that orchestrates AI coding agents in an infinite loop to autonomously implement software features defined in a PRD (Product Requirements Document). It is a complete rewrite of the original [snarktank/ralph](https://github.com/snarktank/ralph) bash script.

The core idea ("the Ralph Pattern"): break work into small user stories, spawn fresh AI instances to implement them one at a time, verify each with automated tests, persist learnings for future iterations, and repeat until all stories pass.

## Architecture

```
main.go           CLI entry point, command dispatch
commands.go       Command handlers (init, run, verify, prd, status, doctor, logs)
config.go         Configuration loading, validation, provider defaults, readiness checks
schema.go         PRD definition types, flat RunState, validation, state load/save
prompts.go        Prompt template loading + variable substitution (//go:embed prompts/*)
loop.go           Main agent loop, provider subprocess management, verify-at-top-of-loop
logger.go         JSONL event logging, run history, timestamped console output
prd.go            Interactive PRD state machine (create/refine/finalize/regenerate)
feature.go        Date-prefixed feature directory management, cross-feature learnings aggregation
services.go       Dev server lifecycle, output capture, health checks
git.go            Git operations (branch, commit, status, working tree checks, test file detection)
lock.go           Concurrency lock file (.ralph/ralph.lock)
cleanup.go        CleanupCoordinator for graceful signal handling and resource cleanup
atomic.go         Atomic file writes (temp + rename)
upgrade.go        Self-update via go-selfupdate (scripness/ralph)
update_check.go   Background update check with 24h cache
discovery.go      Codebase context discovery (tech stack, frameworks, package manager, dependencies)
resources.go      Framework source code caching, management (ResourceManager), ensureResourceSync helper
consultation.go   Resource consultation system (subagent spawning, framework matching, caching, guidance formatting)
resolve.go        Dependency resolution engine (repo URL lookup, version extraction, lock file parsing, tag matching)
resourcereg.go    Resource type definition
resourceregistry.go  Cache metadata (registry.json) management, URL/unresolvable caches
external_git.go   Git operations for external repos (clone, fetch, pull) — uses ref (branch or tag)
utils.go          fileExists helper
```

Prompt templates live in `prompts/` and are embedded at compile time:
- `run.md` - story implementation instructions sent to provider
- `verify-fix.md` - interactive verify-fix session prompt (used by `ralph verify` on failures)
- `prd-create.md` - PRD brainstorming prompt
- `prd-finalize.md` - PRD to JSON conversion prompt
- `refine.md` - interactive refine session context prompt (used by `ralph prd` refine options)
- `verify-analyze.md` - AI deep verification analysis prompt (used by `ralph verify`)
- `consult.md` - story-level resource consultation prompt (subagent searches cached framework source)
- `consult-feature.md` - feature-level resource consultation prompt (subagent searches cached framework source)

## Build and Test

```bash
make build    # go build -ldflags="-s -w" -o ralph .
make test     # go test ./...
make test-e2e # go test -tags e2e -timeout 60m -v -run TestE2E ./...
```

Go version: 1.25.6. Key dependency: `github.com/creativeprojects/go-selfupdate` for self-update.

## How the CLI Works (End-to-End)

1. `ralph init` detects the project's tech stack, prompts for provider selection, verify commands (with auto-detected defaults from package.json/go.mod/Cargo.toml/pyproject.toml/mix.exs/requirements.txt), and dev server config (services are required), then creates `ralph.config.json` and `.ralph/` directory (with `.ralph/.gitignore`). Use `--force` to overwrite existing config.
2. `ralph prd <feature>` runs an interactive state machine with a menu-driven flow: create prd.md -> finalize to prd.json. When prd.md exists but no prd.json, offers Finalize/Refine with AI/Edit/Quit. When both exist, offers Refine with AI/Regenerate prd.json/Edit prd.md/Edit prd.json/Quit. Refine opens an interactive AI session pre-loaded with comprehensive feature context (prd.md, prd.json, progress, story status, learnings, git diff, codebase context, verify commands, service URLs, resource consultation guidance). Regenerate safely re-runs finalization because run-state.json is separate from prd.json. When creating a new PRD, discovery runs first to detect the codebase context (tech stack, frameworks, verify commands), resource consultation produces framework briefs, and both are included in the prompt.
3. `ralph run <feature>` enters the main loop:
   - Readiness gate: refuses to run if not inside a git repo, `sh` is missing, `.ralph/` is not writable, QA commands are missing/placeholder, or command binaries aren't in PATH
   - Signal handling: SIGINT/SIGTERM releases the lock and exits with code 130
   - Loads PRDDefinition + RunState, acquires lock, ensures git branch, starts services
   - Each iteration: picks next story (highest priority, not passed/skipped)
   - **Verify-at-top**: runs verification first — if story already passes, mark passed and skip to next (guarded by `HasNonRalphChanges()` to skip on fresh branches)
   - **Resource consultation**: spawns lightweight subagent(s) to search cached framework source and produce focused guidance (200-800 tokens per framework)
   - Generates prompt (with consultation guidance injected), spawns provider subprocess, captures output with marker detection
   - After provider signals `<ralph>DONE</ralph>`, CLI runs verification commands + service health checks
   - Pass -> mark story complete; Fail -> retry (up to maxRetries, then auto-skip)
   - When all stories are passed or skipped -> print summary, exit
   - Learnings are saved on every path (including timeout/error)
4. `ralph verify <feature>` runs all verification checks (verify commands + services + AI deep analysis), prints a structured report. On failures, offers an interactive AI session to fix issues.
5. `ralph logs <feature>` views run history and logs:
   - `--list` shows all runs with timestamps and outcomes
   - `--summary` shows detailed summary of latest run (stories, durations, verification results)
   - `--run N` views a specific run number
   - `--follow` / `-f` tails the log in real-time (like `tail -f`), including live provider output
   - `--type TYPE` filters by event type (e.g., `error`, `warning`, `marker_detected`)
   - `--story ID` filters events for a specific story
   - `--json` outputs raw JSONL for piping to other tools

## Responsibility Split: CLI vs AI Provider

This is the most important architectural decision. In the original v1, the AI agent did almost everything (picked stories, ran tests, updated prd.json, committed). In v2, the CLI is the orchestrator and the AI is a pure implementer.

### CLI handles (the provider must NOT do these):
- **Readiness enforcement**: Hard-fails before run/verify if not in a git repo, `sh` is missing, `.ralph/` is not writable, QA commands are missing/placeholder, or not in PATH
- **Story selection**: CLI picks the next story by priority, skipping passed/skipped
- **Branch management**: CLI creates/switches to the `ralph/<feature>` branch (both `ralph prd` and `ralph run` call `EnsureBranch` before any commits)
- **State updates**: CLI marks stories passed/failed/skipped based on verification results
- **Verification**: CLI runs `verify.default` and `verify.ui` commands, not the provider
- **Service management**: CLI starts/stops/restarts dev servers; captures output; checks health during verification
- **Learning management**: CLI deduplicates learnings and saves them on every code path (including timeout/error)
- **Concurrency control**: CLI uses lock file to prevent parallel runs
- **Resource consultation**: CLI spawns lightweight subagents to search cached framework source and inject pre-digested guidance into prompts. The main agent never reads raw framework repos directly.
- **PRD persistence**: CLI commits prd.md, prd.json, and run-state.json. `ralph prd` commits prd.md after creation and both prd files after finalization. `ralph run` commits run-state.json state changes atomically (including STUCK/timeout/no-DONE paths)

### Provider handles (told via prompts in prompts/run.md):
- **Code implementation**: Write the code for the assigned story
- **Writing tests**: Create tests for the implementation
- **Local checks**: Run linters/tests before committing (as a sanity check)
- **Git commits**: Commit implementation with `feat: US-XXX - Title` format
- **Signal markers**: Output `<ralph>DONE</ralph>`, `<ralph>STUCK</ralph>`, etc.
- **Knowledge updates**: Update AGENTS.md/CLAUDE.md with discovered patterns
- **Learnings**: Output `<ralph>LEARNING:...</ralph>` for cross-iteration memory
- **Documentation verification**: Verify implementations using pre-digested framework guidance (provided by CLI consultation) or web search

### User Contract

**User must provide (prompted during `ralph init`):**
- `provider.command` — which AI CLI to use
- `verify.default` — typecheck/lint/test commands
- `services` — dev server config (start command + ready URL)

**User must provide (manual config, only if needed):**
- `verify.ui` — e2e test commands (only if UI stories)

**Ralph handles automatically:**
- Provider args, promptMode, knowledgeFile (auto-detected from known providers)
- Verify command suggestions (auto-detected from package.json scripts, go.mod, Cargo.toml, pyproject.toml, mix.exs, requirements.txt)
- Resource caching (auto-detected from dependencies)
- Git branch management, story selection, verification orchestration
- All PRD state management

### Provider communication protocol (markers in stdout or stderr):
| Marker | Meaning |
|--------|---------|
| `<ralph>DONE</ralph>` | Story implementation complete, ready for CLI verification |
| `<ralph>STUCK:reason</ralph>` | Cannot proceed, counts as failed attempt. Reason text saved for debugging. Auto-skips at maxRetries. |
| `<ralph>LEARNING:text</ralph>` | Save insight for future iterations |

## Provider Integration

Ralph is provider-agnostic. It spawns any AI CLI as a subprocess and communicates via stdin/stdout (markers are scanned on both stdout and stderr).

Three prompt delivery modes:
- `stdin` (default): pipe prompt text to provider's stdin
- `arg`: pass prompt as final command argument (optionally preceded by `promptFlag`)
- `file`: write prompt to temp file, pass path as argument (optionally preceded by `promptFlag`)

Auto-detected defaults by provider command name (all fields auto-detected when only `command` is set):
| Provider | promptMode | promptFlag | defaultArgs | knowledgeFile |
|----------|-----------|------------|-------------|---------------|
| `amp` | stdin | | `--dangerously-allow-all` | AGENTS.md |
| `claude` | stdin | | `--print --dangerously-skip-permissions` | CLAUDE.md |
| `opencode` | arg | | `run` | AGENTS.md |
| `aider` | arg | `--message` | `--yes-always` | AGENTS.md |
| `codex` | arg | | `exec --full-auto` | AGENTS.md |
| other | stdin | | | AGENTS.md |

`defaultArgs` are applied only when `args` key is absent from config JSON. Setting `"args": []` explicitly opts out of default args.

For interactive sessions (`ralph prd`), `stdin` prompt mode is automatically overridden to `arg` since the provider needs stdin for interactive input. Non-interactive flags (`--print`, `-p`) are also stripped so the provider runs as a full interactive CLI session where the user can converse with the AI.

## Key Differences from Original Ralph (snarktank/ralph v1)

The original was a ~90-line bash script (`ralph.sh`) with a simple for loop:

| Aspect | v1 (bash) | v2 (this repo) |
|--------|-----------|----------------|
| Language | Bash script | Go binary |
| Providers | Amp or Claude (hard-coded if/else) | Any AI CLI (provider-agnostic) |
| Story selection | AI agent decides | CLI decides (deterministic) |
| State management | AI updates prd.json directly | CLI manages all state |
| Verification | AI runs checks itself | CLI runs verification commands |
| Completion signal | `<promise>COMPLETE</promise>` | Marker protocol (DONE, STUCK, LEARNING) |
| Memory | `progress.txt` (append-only file) | `run-state.json` learnings |
| Loop limit | Fixed iterations (default 10) | Infinite until verified |
| Browser testing | Delegated to provider skills (dev-browser) | Project's own e2e test suite via verify.ui |
| Service management | None | Start/ready/restart lifecycle |
| Crash recovery | None | Verify-at-top-of-loop (re-checks on restart) |
| Concurrency safety | None | Lock file |
| Multi-feature | Manual archive/switch | Date-prefixed feature directories |
| PRD workflow | External skills for Amp/Claude plugins | Built-in interactive state machine |
| Self-update | None | `ralph upgrade` from GitHub releases |

The fundamental shift: v1 trusted the AI to manage its own workflow. v2 treats the AI as a pure code-writing tool within a deterministic orchestration framework.

## Configuration (ralph.config.json)

```json
{
  "$schema": "https://raw.githubusercontent.com/scripness/ralph/main/ralph.schema.json",
  "maxRetries": 3,
  "provider": {
    "command": "claude"
  },
  "services": [
    {
      "name": "dev",
      "start": "bun run dev",
      "ready": "http://localhost:3000",
      "readyTimeout": 30,
      "restartBeforeVerify": true
    }
  ],
  "verify": {
    "default": ["bun run typecheck", "bun run lint", "bun run test:unit"],
    "ui": ["bun run test:e2e"],
    "timeout": 300
  },
  "commits": {
    "prdChanges": true
  },
  "logging": {
    "enabled": true,
    "maxRuns": 10,
    "consoleTimestamps": true,
    "consoleDurations": true
  },
  "resources": {
    "enabled": true,
    "cacheDir": "~/.ralph/resources"
  }
}
```

`promptMode`, `promptFlag`, `args`, and `knowledgeFile` are auto-detected from `provider.command` if not set. Only `command` is required. `verify.timeout` sets the per-command timeout in seconds (default: 300). `logging` controls the observability system (all options default to shown values). `resources` configures framework source code caching (enabled by default). Dependencies are auto-resolved from project manifests and cached at version-specific paths (e.g., `~/.ralph/resources/zod@3.24.4/`).

## PRD Schema (v3) — State Separation

PRD data is split into two files: **prd.json** (AI-authored definition, immutable during runs) and **run-state.json** (CLI-managed execution state).

### prd.json (definition only)

```json
{
  "schemaVersion": 3,
  "project": "Name",
  "branchName": "ralph/feature",
  "description": "Feature description",
  "userStories": [{
    "id": "US-001",
    "title": "...",
    "description": "...",
    "acceptanceCriteria": ["..."],
    "tags": ["ui"],
    "priority": 1
  }]
}
```

### run-state.json (execution state only)

```json
{
  "passed": ["US-001", "US-003"],
  "skipped": ["US-005"],
  "retries": { "US-002": 2 },
  "lastFailure": { "US-002": "typecheck failed: ...", "US-005": "max retries exceeded" },
  "learnings": ["accumulated insights"]
}
```

### In-memory types

All functions take a `(*PRDDefinition, *RunState)` pair directly. No adapter layer.

```go
def, err := LoadPRDDefinition(prdJsonPath)
state, err := LoadRunState(statePath)
// Functions take the pair:
story := GetNextStory(def, state)
state.MarkPassed(story.ID)
SaveRunState(statePath, state)
```

Story lifecycle: `pending` -> provider implements -> CLI verifies -> `passed` or `retry (retries++)` or `skipped (auto at maxRetries)`.

## Codebase Patterns

- **Atomic state updates**: All run-state.json and prd.json writes go through `AtomicWriteJSON` (write temp -> validate -> rename). Never write state files directly.
- **State separation**: prd.json (v3) contains AI-authored definition only — immutable during runs. run-state.json contains flat CLI-managed execution state (passed[], skipped[], retries{}, lastFailure{}, learnings[]). Functions take `(*PRDDefinition, *RunState)` pair directly. `LoadPRDDefinition` + `LoadRunState` load both; `SaveRunState` writes only run-state.json.
- **Marker detection**: `processLine()` in loop.go scans each line of provider stdout and stderr. Lines are trimmed and matched as whole lines (not substrings) to prevent marker spoofing via embedded text. Simple marker (DONE) uses `==` comparison; parameterized markers (STUCK:reason, LEARNING:text) use `^...$` anchored regexes. Markers are detected during execution, not after.
- **Provider subprocess**: Spawned via `os/exec` with context timeout. Three modes for prompt delivery (stdin pipe, arg append, temp file).
- **Lock file**: JSON with pid/startedAt/feature/branch. Stale detection via `isLockStale()` which checks both process liveness (`kill(pid, 0)`) and lock age (24h max, guards against PID reuse). Atomic creation via `O_CREATE|O_EXCL`.
- **Feature directories**: `.ralph/YYYY-MM-DD-feature/` or `.ralph/YYYYMMDD-feature/` format. `FindFeatureDir` finds most recent match by suffix.
- **Feature name matching**: `FindFeatureDir` matches feature names case-insensitively via `strings.EqualFold`. `ralph run Auth` and `ralph run auth` find the same feature directory.
- **Default branch detection**: `DefaultBranch()` tries `origin/HEAD` symbolic ref first, then falls back to checking if `main` or `master` branch exists locally, then checks `origin/main` or `origin/master` remote tracking branches. Diff-based functions (`GetChangedFiles`, `HasFileChanged`, `GetDiffSummary`) fall back from three-dot diff to two-dot diff when the merge-base is unavailable.
- **Process group killing**: Services and provider subprocesses are started with `Setpgid: true` so `syscall.Kill(-pid, SIGTERM)` kills the entire process group (including child processes). This prevents orphaned processes on timeout or signal. Provider subprocesses use `cmd.Cancel` + `cmd.WaitDelay` (matching `runCommand()`) to ensure the process group is killed on context timeout and `cmd.Wait()` is always called to prevent zombie processes.
- **CleanupCoordinator**: Signal handlers use a `CleanupCoordinator` that resources register with when created. On SIGINT/SIGTERM, the coordinator kills provider process groups, stops services, logs run end, and releases locks — all before calling `os.Exit(130)`. This ensures cleanup happens even when defer statements would be bypassed.
- **Service ready checks**: HTTP GET polling every 500ms until status < 500, with configurable timeout. ServiceManager reuses a single `http.Client` instance to avoid repeated allocations.
- **Idempotent StopAll**: `ServiceManager.StopAll()` sets `processes` to nil after stopping, making it safe to call multiple times (e.g., from both defer and signal handler).
- **Service output capture**: `capturedOutput` trim buffer (keeps last ~50% on overflow) captures service stdout/stderr. Service output is not printed to the console — only captured for diagnostics via `GetRecentOutput()`.
- **Service health checks**: `CheckServiceHealth()` polls service ready URLs during verification to detect crashed/unresponsive services.
- **Readiness gates**: `CheckReadiness()` in config.go validates: `sh` is in PATH, project is inside a git repository, `.ralph/` directory exists and is writable, QA command binaries exist in PATH (`extractBaseCommand()` + `exec.LookPath`), service `start` commands are available (placeholder service commands are flagged separately). UI stories with no `verify.ui` commands generate a warning. Called before `ralph run` and `ralph verify`. `CheckReadinessWarnings(cfg)` returns soft warnings (e.g., unknown provider defaults).
- **Learning deduplication**: `AddLearning()` normalizes (case-insensitive, trimmed) before checking for duplicates. Learnings are saved on all code paths: success, timeout, error, STUCK, and no-DONE. Prompt delivery caps at 50 most recent learnings to prevent context overflow.
- **Cross-feature learnings**: `CollectCrossFeatureLearnings(projectRoot, excludeFeature)` in feature.go aggregates learnings from other features' run-state.json files at prompt time. Skips the current feature (case-insensitive), features with zero passed stories, and deduplicates using `normalizeLearning()`. Returns most-recent-feature first. Injected into `generateRunPrompt()` only (not verify-fix or refine). Read-only — does not modify any feature's run-state.json.
- **Prompt templates**: Embedded via `//go:embed prompts/*`. Simple `{{var}}` string replacement (not Go templates).
- **Update check**: Background goroutine with 5s timeout, cached to `~/.config/ralph/update-check.json` for 24h (falls back to `/tmp/ralph-update-check.json` if home directory is unavailable). Non-blocking: skipped silently if check hasn't finished by CLI exit. Disabled for `dev` builds and `ralph upgrade`. Cache uses `isNewerVersion()` for proper semver comparison (not just string inequality) to avoid suggesting downgrades after upgrades.
- **Resources module**: `ResourceManager` auto-resolves ALL project dependencies to their source repos, caches them at version-specific paths (`~/.ralph/resources/name@version/`). Resolution flow: `ExtractDependencies()` → `resolveExactVersions()` (from lock files) → `resolveRepoURL()` (from package registries) → `findVersionTag()` → shallow clone at tag. Repo URLs are resolved from npm/PyPI/crates.io/hex.pm registries and Go module paths. `ResolveAll()` runs a 5-worker goroutine pool with per-request 10s timeout. Resolved URLs are cached in `registry.json` to avoid repeated API calls. Unresolvable deps are cached with 7-day expiry. `@types/*` packages, Go indirect deps, and workspace/file/link refs are skipped. `ensureResourceSync()` in resources.go is a reusable helper that creates a ResourceManager, syncs if dependencies are detected, and returns the manager (or nil if resources disabled). Used by `cmdRun`, `cmdPrd`, and `cmdVerify`.
- **Version-specific caching**: Cache paths use `name@version` format (e.g., `~/.ralph/resources/zod@3.24.4/`). When a user bumps a dependency version, the next sync detects the version change and clones the new version. Old versions stay cached for other projects. Tag-based clones are immutable (no sync needed). Default branch clones still check for updates. `resolveExactVersions()` reads lock files (bun.lock, package-lock.json, yarn.lock, pnpm-lock.yaml) for npm, uses go.mod versions for Go, cleans manifest specs for others. Resolved URLs are cached with 30-day expiry (`ResolvedEntry` type with timestamp).
- **Resource consultation**: Instead of pointing agents at raw framework source paths, Ralph spawns lightweight consultation subagents that search cached repos and return focused guidance (200-800 tokens). `ConsultResources()` handles story-level consultation (filters by tags + keywords + name-based matching), `ConsultResourcesForFeature()` handles feature-level consultation (all cached frameworks, capped at 3). Consultation results are injected into prompts via `{{resourceGuidance}}`. Consultants run in parallel (goroutines + WaitGroup). Results are cached to `.ralph/<feature>/consultations/` by SHA256 hash of story/framework/commit. `GUIDANCE_START`/`GUIDANCE_END` markers delimit consultant output. Version info is included in consultation prompts (e.g., "Search the **zod v3.24.4** source code"). Falls back gracefully: failed consultation → file paths with version + search instructions; no cached resources → web search fallback via `buildResourceFallbackInstructions()`.
- **Framework matching**: `relevantFrameworks()` in consultation.go deterministically filters cached resources for a story using: (1) tag matching (`frameworkTagMap`: `ui` → React/Vue/Svelte, `db` → Prisma/Drizzle, `api` → Express/Fastify, etc.), (2) keyword matching (`frameworkKeywords`: 2+ keyword hits required), and (3) name-based matching for auto-resolved deps without keyword entries (`dependencyNameVariants()` splits scoped names). Caps at 3 frameworks per consultation. Feature-level consultation uses `allCachedFrameworks()` which returns all cached resources (capped at 3) without filtering.
- **VerifyReport**: `runVerifyChecks()` runs all verification gates (verify commands, service health, AI deep analysis) and returns a structured `*VerifyReport` with PASS/FAIL/WARN items. `FormatForConsole()` renders a colored summary table; `FormatForPrompt()` renders detailed output for the AI verify-fix session.
- **AI deep verification**: `ralph verify` always runs an AI subagent after mechanical checks. The subagent reads changed files, checks acceptance criteria, verifies best practices, and outputs `VERIFY_PASS` or `VERIFY_FAIL:reason` markers. `runVerifySubagent()` spawns the provider non-interactively and scans for these markers. Multiple `VERIFY_FAIL` markers are collected. NOT run during `ralph run` (per-story verification stays fast).
- **Branch from default branch**: `cmdPrd` passes `git.DefaultBranch()` as startPoint to `EnsureBranch`, so new feature branches always start from main/master instead of the current HEAD. `EnsureBranch(branchName, startPoint ...string)` uses `CreateBranchFrom` when creating a new branch with a startPoint.
- **$schema field**: `RalphConfig` has a `Schema string \`json:"$schema,omitempty"\`` field. `WriteDefaultConfig` includes a `$schema` URL pointing to `ralph.schema.json` for editor autocompletion. The value is ignored at runtime.
- **Git diff in verify prompt**: `GetDiffSummary()` provides `git diff --stat` output from the default branch to HEAD, used in verify-fix and refine prompts.
- **KnowledgeFile change detection**: `HasFileChanged()` in git.go checks if a file was modified on the current branch vs default branch using `git diff --name-only`. Used in verification to report whether the knowledgeFile was updated.
- **Commit-exists gate**: After provider signals DONE, CLI checks HEAD changed from pre-run snapshot (captured AFTER PRD commit to avoid false positives). No new commit = automatic retry counted toward maxRetries.
- **Verify command timeout**: Each verify command has a configurable timeout (`verify.timeout`, default 300s). Prevents hanging test suites from blocking the loop indefinitely.
- **Test file heuristic**: `HasTestFileChanges()` checks if any files matching test patterns (`*_test.*`, `*.test.*`, `*.spec.*`, `__tests__/`) were modified on the branch. Result is included as PASS/WARN in the verification summary.
- **Fresh branch guard**: `HasNonRalphChanges()` returns true if any changed files on the branch are outside `.ralph/`. Used by verify-at-top to skip verification on fresh branches where no implementation work exists — prevents false positives where the existing test suite passes vacuously (it doesn't test the new feature yet).
- **Run logging**: `RunLogger` in logger.go writes JSONL event logs to `.ralph/YYYY-MM-DD-feature/logs/run-NNN.jsonl`. Events include timestamps, durations, story context, and full provider/verification output. Logs are auto-rotated to keep only `maxRuns` most recent. Key methods: `IterationStart(storyID, title, retries)` logs iteration beginning, `IterationEnd(success)` logs iteration completion, `ProviderLine(stream, line)` streams each provider output line in real-time.
- **Console output suppression**: `ralph run` shows a clean dashboard — provider output, verification command output, and service output are NOT printed to the console. Provider output is streamed line-by-line to JSONL (`provider_line` events) for real-time viewing via `ralph logs -f`. Verification commands only show the `→ command` and `✓/✗ (duration)` status lines. Key markers (DONE, STUCK, LEARNING) are printed to console as detected.
- **Timestamped console output**: When `logging.consoleTimestamps` is true, status lines are prefixed with `[HH:MM:SS]`.
- **Event types**: `run_start`, `run_end`, `iteration_start/end`, `provider_start/end`, `provider_output`, `provider_line`, `marker_detected`, `verify_start/end`, `verify_cmd_start/end`, `service_start/ready/restart/health`, `state_change`, `learning`, `warning`, `error`. Note: `provider_output` logs stdout/stderr in bulk after completion; `provider_line` streams each line in real-time.
- **Verify-at-top-of-loop**: Each loop iteration verifies the story BEFORE implementation. If the story already passes all checks, it's marked passed and skipped — no provider spawn needed. This replaces the old pre-verify phase and enables the "infinite loop" pattern: modify PRD → run → verify-at-top catches already-implemented work. **Fresh branch guard**: verify-at-top is skipped when `HasNonRalphChanges()` returns false (no implementation commits on the branch yet), preventing false positives on brand-new feature branches.
- **Codebase discovery**: `DiscoverCodebase()` in discovery.go detects tech stack (go, typescript, python, rust, elixir), package manager (bun, npm, yarn, pnpm, go, cargo, mix), frameworks, and full dependency list from config files. Used in PRD creation and resource syncing. Detection is lightweight (reads config files, doesn't run commands). `ExtractDependencies()` returns `[]Dependency` with name, version, and isDev flag.
- **Dependency extraction**: `extractJSDependencies()` parses package.json, `extractGoDependencies()` parses go.mod, `extractPythonDependencies()` parses pyproject.toml/requirements.txt, `extractRustDependencies()` parses Cargo.toml, `extractElixirDependencies()` parses mix.exs. The `Dependency` struct includes Name, Version, and IsDev fields.
- **Provider selection prompt**: `ralph init` prompts the user to select from `providerChoices` (alphabetically sorted known providers) or enter a custom command. `promptProviderSelection()` accepts a `*bufio.Reader` so tests can inject controlled input. `providerChoices` must stay in sync with `knownProviders` map (enforced by `TestProviderChoices_MatchKnownProviders`).
- **Verify command prompts**: `promptVerifyCommands()` in commands.go prompts for 3 commands (typecheck, lint, test) during `ralph init`. Accepts `*bufio.Reader` and detected defaults `[3]string` for testability. When defaults are detected, pressing Enter accepts them; typing overrides. Non-empty inputs go into `verify.default`; all skipped falls back to placeholder echo commands.
- **Service config prompt**: `promptServiceConfig()` in commands.go prompts for dev server start command and ready URL during `ralph init`. Returns `*ServiceConfig` (nil if skipped, placeholder used instead). Auto-prepends `http://` if no scheme. Services are required — `validateConfig()` enforces at least one entry.
- **Verify command auto-detection**: `DetectVerifyCommands()` in discovery.go reads project config files to pre-fill verify command prompts during `ralph init`. For JS/TS projects, reads `scripts` from package.json (prefers `test:unit` over `test`). For Go, suggests `go vet ./...` and `go test ./...`; detects `golangci-lint run` if `.golangci.yml`/`.golangci.yaml` exists. For Rust, suggests `cargo check` and `cargo test`. For Python, suggests `pytest` if in dependencies. For Elixir, suggests `mix compile --warnings-as-errors` and `mix test`; detects `mix credo` if `:credo` is in mix.exs deps. Only suggests commands that are 100% deterministic from config files — never guesses.
- **`.ralph/` existence check in CheckReadiness**: `CheckReadiness` verifies `.ralph/` exists before checking writability. Missing directory produces "Run 'ralph init' first" message.
- **Services required**: `validateConfig()` enforces at least one service entry. Also validates that `services[].ready` starts with `http://` or `https://`. Catches typos like `localhost:3000` (missing scheme) at config load time. `WriteDefaultConfig()` writes a placeholder service if none provided during init.
- **Unknown provider warning**: `CheckReadinessWarnings()` warns when `provider.command` is not in `knownProviders` map. Non-blocking warning printed before every run/verify.
- **Interactive sessions strip non-interactive flags**: `runProviderInteractive()` removes `--print`/`-p` from provider args via `stripNonInteractiveArgs()` so the provider runs as a conversational CLI session. Used by `ralph prd` (including refine options). The `ralph run` loop (which uses `runProvider()` in loop.go) is unaffected.
- **Interactive commands skip Setpgid**: `Command.Run()` in prd.go does NOT set `Setpgid: true` — the provider must stay in ralph's foreground process group to read from the terminal. `Setpgid` would put it in a background group, causing SIGTTIN on stdin reads (freezing the process). The run loop in loop.go uses `Setpgid: true` since it communicates via pipes, not the terminal.
- **`ralph prd` validates git + ensures branch + warns about placeholder commands**: `cmdPrd()` calls `checkGitAvailable()`, ensures `ralph/<feature>` branch via `EnsureBranch`, and warns (without blocking) if verify.default contains placeholder commands. `commitPrdFile()` has a defense-in-depth check that refuses to commit unless on a `ralph/` branch. The finalized menu offers Refine with AI/Regenerate prd.json/Edit prd.md/Edit prd.json/Quit.
- **Story state helpers**: `RunState` methods: `MarkPassed(id)` appends to passed[], `MarkSkipped(id, reason)` appends to skipped[] + sets lastFailure, `MarkFailed(id, reason, maxRetries)` increments retries and auto-skips at threshold, `UnmarkPassed(id)` removes from passed (for regression detection). `GetNextStory(def, state)` returns first non-passed, non-skipped story by priority. `AllComplete(def, state)` returns true when every story is in passed or skipped.
- **EnsureBranch dirty-tree guard**: `EnsureBranch` refuses to switch to an existing branch with uncommitted changes (returns error). Creating a new branch with dirty tree is allowed (changes carry over safely). Already on the right branch skips the check entirely.
- **`commitPrdOnly` error handling**: `commitPrdOnly(projectRoot, filePath, message)` commits a single file (typically run-state.json during runs). All call sites check the returned error and log a warning via `logger.Warning()`. PRD state commit failures are non-fatal (the loop continues) but are surfaced.
- **Scanner buffer overflow logging**: Both stdout and stderr scanners in `runProvider` check `scanner.Err()` after completion and log warnings for buffer overflows (lines >1MB).
- **Refine context loading**: `generateRefinePrompt()` reads prd.md + prd.json from disk, discovers codebase context, builds git diff summary, includes verify commands + service URLs, and injects resource consultation guidance. The resulting prompt gives the AI comprehensive feature context for free-form interactive work. Called from `prdRefineInteractive()` in prd.go.

## Prompt Template Variables

Each prompt template uses `{{var}}` placeholders replaced by `prompts.go`:

| Template | Variables |
|----------|-----------|
| `run.md` | `storyId`, `storyTitle`, `storyDescription`, `acceptanceCriteria`, `tags`, `retryInfo`, `verifyCommands`, `learnings`, `knowledgeFile`, `project`, `description`, `branchName`, `progress`, `storyMap`, `serviceURLs`, `timeout`, `codebaseContext`, `diffSummary`, `resourceGuidance` |
| `verify-fix.md` | `project`, `description`, `branchName`, `progress`, `storyDetails`, `verifyResults`, `verifyCommands`, `learnings`, `knowledgeFile`, `serviceURLs`, `diffSummary`, `featureDir`, `resourceGuidance` |
| `verify-analyze.md` | `project`, `description`, `branchName`, `criteriaChecklist`, `verifyResults`, `diffSummary`, `resourceGuidance` |
| `prd-create.md` | `feature`, `outputPath`, `codebaseContext`, `resourceGuidance` |
| `prd-finalize.md` | `feature`, `prdContent`, `outputPath`, `resourceGuidance` |
| `refine.md` | `feature`, `prdMdContent`, `prdJsonContent`, `progress`, `storyDetails`, `learnings`, `diffSummary`, `codebaseContext`, `verifyCommands`, `serviceURLs`, `knowledgeFile`, `branchName`, `featureDir`, `resourceGuidance` |
| `consult.md` | `framework`, `frameworkPath`, `storyId`, `storyTitle`, `storyDescription`, `acceptanceCriteria`, `techStack` |
| `consult-feature.md` | `framework`, `frameworkPath`, `feature`, `techStack` |

## Non-obvious Behaviors

- **`AllComplete` treats skipped as complete**: Skipped stories do not prevent the "all stories complete" state — the loop prints a summary and exits when every story is passed or skipped.
- **`runProvider` returns nil error on non-zero exit**: A provider exiting with code != 0 is not treated as an error — it returns `(result, nil)`. The loop checks markers in the result to determine outcome. If `runProvider` returns `(nil, err)` (provider failed to start), the loop guards against nil pointer dereference when logging markers.
- **`ralph verify` ensures correct branch**: `cmdVerify` calls `EnsureBranch` after loading the PRD to ensure verification runs on the feature branch, not whatever branch was checked out.
- **Verification commands run through `sh -c`**: All verification commands are wrapped in `sh -c`, so shell features (pipes, redirects, etc.) are available. `runCommand()` uses `Setpgid: true` and `cmd.Cancel` to kill the entire process group (`syscall.Kill(-pid, SIGKILL)`) on timeout, preventing orphaned test/lint processes.
- **`cmdVerify` acquires the lock**: Not just `cmdRun` — verification also acquires the lock file to prevent concurrent operations.
- **Verification output is captured for retries**: When verification fails, the last 50 lines of command output are stored in `lastFailure` so the retry agent can see what specifically failed.
- **`ralph verify` interactive fix session**: When `ralph verify` detects failures, it offers to open an interactive AI session (using `runProviderInteractive`) pre-loaded with the `VerifyReport` details. The AI can then fix issues interactively.
- **Provider must commit code to pass**: Signaling DONE without making a new git commit is treated as a failed attempt and counts toward maxRetries. The pre-run commit hash is captured AFTER the PRD state commit to avoid false positives.
- **Working tree cleanliness is checked but not enforced**: Uncommitted files after provider finishes generate a warning but don't fail the story. This catches providers that left untracked or modified files.
- **Test file heuristic in VerifyReport**: If no test files (`*_test.*`, `*.test.*`, `*.spec.*`, `__tests__/`) are modified on the branch, a WARN item is included in the `VerifyReport`.
- **`$EDITOR` validation in PRD editing**: `prdEditManual()` validates the editor binary exists before spawning. Falls back from `$EDITOR` to `nano`, but checks either is available. Returns a clear error with instructions to set `$EDITOR`.
- **`ralph prd` warns about placeholder verify commands but does not block**: During `cmdPrd()`, if verify.default contains placeholder commands, a warning is printed to stderr. PRD creation proceeds normally — the user can still create PRDs but needs to fix config before `ralph run`.
- **`prdFinalize` produces v3 prd.json**: `prdFinalize()` converts prd.md to prd.json (v3 schema, no runtime fields). Validates with `LoadPRDDefinition` instead of `LoadPRD`.
- **`prdStateFinalized` offers refine and regenerate**: When returning to a finalized PRD, the menu shows current progress (e.g., "Progress: 3/5 stories complete (1 skipped)") if any stories have been worked on, and offers Refine with AI/Regenerate prd.json/Edit prd.md/Edit prd.json/Quit. Regenerate is safe because run-state.json is separate. If prd.json fails to load (corrupted, wrong schema version), a recovery menu offers Regenerate/Edit prd.json/Edit prd.md/Quit.
- **Cross-feature learnings are branch-local**: `CollectCrossFeatureLearnings` scans the filesystem, so only features whose `.ralph/` directories are on the current filesystem are visible. Feature branches that haven't been merged won't contribute learnings to other features.
- **Refine options don't acquire lock**: Both `prdRefineDraft` (pre-finalization) and `prdRefineInteractive` (post-finalization) open interactive AI sessions without acquiring a lock. These are interactive sessions, not automated runs.
- **Cross-feature learnings are ephemeral**: `CollectCrossFeatureLearnings` computes learnings at prompt time by scanning other features' run-state.json files. They are never copied into the current feature's run-state.json — purely read-only aggregation. Only injected into `generateRunPrompt` (the story implementation prompt), not into verify-fix or refine prompts.
- **Feature completion prints learnings and knowledge file warning**: When `AllComplete` triggers in the run loop, captured learnings are printed as a numbered list and a warning is shown if the knowledge file was not modified on the branch.
- **Resource consultation is always-on**: Consultation runs automatically whenever cached resources exist — not configurable separately from `resources.enabled`. When resources are disabled or no dependencies are cached, `FormatGuidance` returns a web search fallback instruction.
- **Consultation caching prevents redundant LLM calls**: Consultation results are cached to `.ralph/<feature>/consultations/<hash>.md`. Cache keys are SHA256 hashes of (story ID + framework name + commit hash + story description) for story-level, or (feature name + framework name + commit hash) for feature-level. On retry, cached results are reused without spawning subagents.
- **Consultation uses GUIDANCE_START/GUIDANCE_END markers**: Similar to VERIFY_PASS/VERIFY_FAIL, consultation subagents output guidance between `<ralph>GUIDANCE_START</ralph>` and `<ralph>GUIDANCE_END</ralph>` markers. Missing markers = failed consultation (falls back to file path).
- **Consultation validates source citations**: Successful consultations must contain `Source:` lines (evidence the subagent read actual framework source). Missing citations = treated as failed (likely hallucinated). Falls back to file path with search instructions.
- **Framework keyword matching requires 2+ hits**: `relevantFrameworks()` requires at least 2 keyword matches from `frameworkKeywords` to associate a framework with a story. This prevents false positives from single keyword coincidences. Tag matching via `frameworkTagMap` is exact (1 tag hit suffices).

## Testing

Tests are in `*_test.go` files alongside source. Key test files:
- `commands_test.go` - provider selection prompt, verify command prompts (with detected defaults), service config prompt, provider choices validation, gitignore creation
- `config_test.go` - config loading, validation, provider defaults, readiness checks (including placeholder service detection), command validation, verify timeout defaults, services required validation, $schema field handling
- `schema_test.go` - PRD definition validation, RunState methods (MarkPassed/MarkFailed/MarkSkipped/UnmarkPassed), GetNextStory, AllComplete, GetPendingStories, learning deduplication, LoadPRDDefinition, ValidatePRDDefinition, LoadRunState, SaveRunState, v2 error, IsUIStory
- `discovery_test.go` - tech stack detection (including Elixir), framework detection (including Phoenix), codebase context formatting, verify command auto-detection, Elixir dependency extraction
- `services_test.go` - output capture, service manager, health checks
- `prompts_test.go` - prompt generation, variable substitution, provider-agnosticism, generateRefinePrompt, buildRefinementStoryDetails, generateVerifyAnalyzePrompt, buildCriteriaChecklist, cross-feature learnings in run prompt, resourceGuidance injection
- `loop_test.go` - marker detection (DONE, STUCK with note, LEARNING, whole-line matching, embedded marker rejection, whitespace tolerance), nil ProviderResult guard, provider result parsing, command timeout, runCommand, VERIFY_PASS/VERIFY_FAIL marker detection
- `feature_test.go` - directory parsing, timestamp formats, case-insensitive matching, cross-feature learnings (dedup, exclusion, skip-no-passed, empty, case-insensitive)
- `lock_test.go` - lock acquisition, stale detection (including age-based isLockStale)
- `cleanup_test.go` - CleanupCoordinator idempotency, nil resource handling
- `atomic_test.go` - atomic writes, JSON validation
- `git_test.go` - git operations (branch, commit, checkout, EnsureBranch, EnsureBranch dirty-tree guard, CreateBranchFrom, EnsureBranch with startPoint, DefaultBranch fallback chain including origin/main and origin/master, GetDiffSummary, HasNewCommitSince, IsWorkingTreeClean, GetChangedFiles two-dot fallback, HasTestFileChanges, HasNonRalphChanges)
- `update_check_test.go` - update check cache path, isNewerVersion semver comparison
- `logger_test.go` - event logging, run numbering, log rotation, event filtering, run summaries
- `resources_test.go` - ResourceManager, ResourcesConfig, cache operations, ensureResourceSync, GetCachedResources, CachedResource version field
- `resolve_test.go` - URL normalization, shouldResolve filtering, cleanVersion, ecosystemFromTechStack, splitAtVersion, resolveGo (GitHub/GitLab path truncation), resolveExactVersions (Go mod, bun.lock, package-lock.json, yarn.lock v1/berry, pnpm-lock.yaml v6/v9, fallback clean), parseYarnLockHeader, stripJSONComments, dependencyNameVariants, unresolvable cache (mark/check/expiry), ResolveAll edge cases (empty, @types skip, unresolvable skip)
- `consultation_test.go` - relevantFrameworks (tag matching, keyword matching, 2-hit threshold, name-based matching, cap, nil story), FormatGuidance (success, failed, mixed, empty, nil), extractBetweenMarkers, consultation cache keys (deterministic, save/load), allCachedFrameworks cap, buildResourceFallbackInstructions, consult/consult-feature prompt templates, resourceGuidance in prd-create/refine/prd-finalize prompts
- `resourcereg_test.go` - Resource type fields, ResourceRegistry resolved URL cache (set/get/persist/30-day expiry), unresolvable cache (mark/check/persist/expiry), CachedRepo version field persistence
- `loop_test.go` also includes VerifyReport tests (FormatForConsole, FormatForPrompt, Finalize, AddWarn)
- `prd_test.go` - editor validation in prdEditManual, stripNonInteractiveArgs
- `external_git_test.go` - ExternalGitOps (ref field), Exists, GetRepoSize
- `e2e_test.go` - Full end-to-end test (`//go:build e2e`) exercising init → prd → run → verify on real project with real Claude

Run with `go test ./...` or `go test -v ./...` for verbose output.

### E2E Tests

`e2e_test.go` contains a full end-to-end test (`//go:build e2e`) that exercises the complete user journey on a real TypeScript project (warrantycert) with real Claude CLI. No mocking, no pre-crafted data.

**Prerequisites:** `claude` CLI (configured with API key), `bun`, `git`, internet access.

**Run:** `make test-e2e` (or `go test -tags e2e -timeout 60m -v -run TestE2E ./...`)

**What it tests (15 sequential phases):**
0. **Smoke Tests** — `--help`, `--version`, unknown command (exit codes + output patterns)
1. **Project Setup** — Clone warrantycert, `bun install`, database setup, Playwright install, baseline typecheck
2. **ralph init** — Interactive provider selection (claude) + verify command acceptance (detected defaults)
3. **Config Enhancement** — Add services, verify.ui programmatically
4. **ralph doctor** — Validate all environment checks pass (config, provider, dirs, tools, git)
5. **ralph prd** — Create + finalize PRD for "certificate-search" feature (writes v3 PRDDefinition directly, real Claude brainstorming)
6. **Pre-Run Checks** — status (no-arg + feature)
7. **ralph run** — First implementation run (25 min timeout, real Claude coding)
8. **Post-Run Analysis** — Parse PRD state, JSONL event assertions (run_start, provider_start, verify_start, iteration_start, service_start, marker_detected), git history
9. **PRD Refinement** — Conditional: resets skipped stories in run-state.json if not all passed
10. **Second Run** — Conditional: re-run with refined PRD (20 min timeout)
11. **Status + Logs** — status (no-arg), logs --list/--summary/--json/--run/--type/--story
12. **ralph verify** — Conditional: verification if all stories passed
13. **Post-Run Doctor** — Verify doctor still passes after full run (no stale locks, etc.)
14. **Report** — Comprehensive summary with per-story breakdown

**Key design:** Uses `promptResponder` (expect-style stdin interaction) to drive interactive sessions — watches stdout for prompt patterns and responds just-in-time. Process tracker (`processTracker`) ensures all ralph subprocesses are killed on test abort via `t.Cleanup`. JSONL event assertions validate the internal event stream (run_start, provider_start, verify_start, etc.). The test validates Ralph's full orchestration pipeline including service management and e2e test execution.

**Artifact directory:** Each run saves structured output to `e2e-runs/<timestamp>/` (override with `RALPH_E2E_ARTIFACT_DIR` env var). Structure:
```
e2e-runs/2026-02-10T15-04-05/
  report.md              # Human-readable report organized by story/idea
  result.txt             # PASS, PARTIAL, FAIL, or INCOMPLETE
  summary.json           # Machine-readable: stories, branch, run_number, config
  config.json            # ralph.config.json used
  prd.md / prd.json      # Original PRD
  run-state.json         # Execution state
  prd-final.json         # Final PRD state after all runs
  prd-after-run1.json    # PRD snapshot after first run
  prd-refined.json       # PRD after refinement (if applicable)
  git-log.txt            # Commits on the ralph/ branch
  git-diff.txt           # Full diff from main
  git-diff-stat.txt      # Diff stat summary
  git-status.txt         # Working tree status after runs
  learnings.txt          # All learnings captured
  phases/                # Stdout+stderr from each ralph invocation
    00-smoke.txt
    02-init.txt
    04-doctor.txt
    05-prd-create.txt
    07-first-run.txt
    09-SKIPPED.txt       # Skipped phase indicator (if applicable)
    13-post-doctor.txt
    ...
  stories/               # Per-story breakdown (organized by "idea")
    us-001/
      summary.md         # Description, criteria, status, failure details
      failure.txt        # Raw verification failure output (if failed)
    us-002/
      ...
  logs/                  # Copied JSONL run logs
    run-001.jsonl
    ...
```

**Duration:** ~20-45 minutes depending on Claude's implementation speed.

## Releasing

Releases are automated via [GoReleaser](https://goreleaser.com/) and GitHub Actions (`workflow_dispatch`).

```bash
make release    # triggers GitHub workflow via gh CLI, auto patch-bumps
```

Or from GitHub UI: Actions -> Release -> Run workflow -> pick patch/minor/major.

The workflow: reads latest git tag -> bumps version -> creates new tag -> runs tests -> GoReleaser builds 4 binaries (linux/darwin x amd64/arm64) -> creates GitHub Release with auto-generated notes.

Version is injected at build time via `-ldflags -X main.version`. The `version` variable in `main.go` defaults to `"dev"` for local builds. No version is hardcoded in source — the git tag is the source of truth.

Users are notified of new versions via a background check (24h cache in `~/.config/ralph/update-check.json`) that prints a notice on CLI exit. `ralph upgrade` uses `go-selfupdate` for secure binary replacement.

Config: `.goreleaser.yaml`. Release workflow: `.github/workflows/release.yml`. CI (push/PR): `.github/workflows/ci.yml`.

## Post-Task Checklist

After completing any task or set of tasks that changes behavior, adds features, or modifies interfaces, you MUST verify that documentation and tests reflect the current state of the codebase. Nothing should be outdated.

### Documentation sync

1. **CLAUDE.md** — Check every section that could be affected by your changes:
   - Architecture table (file descriptions)
   - Codebase Patterns (any new or changed patterns)
   - Prompt Template Variables table (if you added/removed/renamed a `{{var}}`)
   - Non-obvious Behaviors (if your change introduces subtle behavior)
   - Testing section (if you added a new `*_test.go` file or significantly expanded an existing one)
   - Common Development Tasks (if the process for a task type changed)
2. **README.md** — Check user-facing descriptions: command usage, signal protocol, configuration examples.
3. **Prompt templates** (`prompts/*.md`) — If you changed how the CLI interacts with the provider (new markers, changed verification flow, new context passed), update the relevant prompt.

### Test sync

1. Every new exported function or method must have at least one test.
2. Every changed function signature or behavior must have its existing tests updated (not just passing — actually testing the new behavior).
3. Run `go test ./...` and `go vet ./...` to confirm nothing is broken.

### How to check

After your code changes are done, re-read the sections of CLAUDE.md, README.md, and prompt templates that relate to what you changed. Diff your mental model of what the docs say against what the code now does. Fix any drift before considering the task complete.

## Common Development Tasks

- **Add a new provider**: Add entry to `knownProviders` map in config.go and add to `providerChoices` slice in commands.go (keep alphabetical)
- **Add a new marker**: Add regex pattern and field to `ProviderResult` in loop.go, handle in `processLine()`
- **Add a new command**: Add case to `main()` switch in main.go, implement handler in commands.go
- **Modify prompt**: Edit the `.md` file in `prompts/` directory (embedded at compile time)
- **Change PRD schema**: Update `PRDDefinition`/`StoryDefinition` in schema.go for definition fields, `RunState` for execution state. Bump schemaVersion, update validation and `prd-finalize.md` template
