# CLAUDE.md

## Project Overview

Ralph v2 is a Go CLI that orchestrates AI coding agents in an infinite loop to autonomously implement software features defined in a PRD (Product Requirements Document). It is a complete rewrite of the original [snarktank/ralph](https://github.com/snarktank/ralph) bash script.

The core idea ("the Ralph Pattern"): break work into small user stories, spawn fresh AI instances to implement them one at a time, verify each with automated tests, persist learnings for future iterations, and repeat until all stories pass.

## Architecture

```
main.go           CLI entry point, command dispatch
commands.go       Command handlers (init, run, verify, prd, refine, status, next, validate, doctor, logs, resources)
config.go         Configuration loading, validation, provider defaults, readiness checks
schema.go         PRD data structures, story state machine, validation, PRD quality checks
prompts.go        Prompt template loading + variable substitution (//go:embed prompts/*)
loop.go           Main agent loop, provider subprocess management, verification orchestration, pre-verify phase
logger.go         JSONL event logging, run history, timestamped console output
browser.go        Browser automation via rod (Chrome DevTools Protocol)
prd.go            Interactive PRD state machine (create/finalize)
feature.go        Date-prefixed feature directory management (.ralph/YYYY-MM-DD-feature/)
services.go       Dev server lifecycle, output capture, health checks
git.go            Git operations (branch, commit, status, working tree checks, test file detection)
lock.go           Concurrency lock file (.ralph/ralph.lock)
cleanup.go        CleanupCoordinator for graceful signal handling and resource cleanup
atomic.go         Atomic file writes (temp + rename)
upgrade.go        Self-update via go-selfupdate (scripness/ralph)
update_check.go   Background update check with 24h cache
discovery.go      Codebase context discovery (tech stack, frameworks, package manager, dependencies)
resources.go      Framework source code caching and management (ResourceManager)
resourcereg.go    Default resource registry mapping dependencies to source repos
resourceregistry.go  Cache metadata (registry.json) management
external_git.go   Git operations for external repos (clone, fetch, pull)
utils.go          fileExists helper
```

Prompt templates live in `prompts/` and are embedded at compile time:
- `run.md` - story implementation instructions sent to provider
- `verify.md` - final verification instructions
- `prd-create.md` - PRD brainstorming prompt
- `prd-finalize.md` - PRD to JSON conversion prompt
- `refine.md` - interactive refine session context prompt

## Build and Test

```bash
make build    # go build -ldflags="-s -w" -o ralph .
make test     # go test ./...
make test-e2e # go test -tags e2e -timeout 60m -v -run TestE2E ./...
```

Go version: 1.25.6. Key dependencies: `github.com/go-rod/rod` for browser automation, `github.com/creativeprojects/go-selfupdate` for self-update.

## How the CLI Works (End-to-End)

1. `ralph init` detects the project's tech stack, prompts for provider selection, verify commands (with auto-detected defaults from package.json/go.mod/Cargo.toml/pyproject.toml/mix.exs/requirements.txt), and dev server config (services are required), then creates `ralph.config.json` and `.ralph/` directory (with `.ralph/.gitignore`). Use `--force` to overwrite existing config.
2. `ralph prd <feature>` runs an interactive state machine with a menu-driven flow: create prd.md -> finalize to prd.json. When prd.md exists but no prd.json, offers Finalize/Edit/Quit. When both exist, offers Edit prd.md/Edit prd.json/Start execution/Quit with a tip about `ralph refine`. When creating a new PRD, discovery runs first to detect the codebase context (tech stack, frameworks, verify commands) and includes it in the prompt.
3. `ralph refine <feature>` opens an interactive AI session pre-loaded with comprehensive feature context (prd.md, prd.json, progress, story status, learnings, git diff, codebase context, verify commands, service URLs). Requires prd.json to exist. The user works freely with the AI — no post-processing by the CLI. After refinement, `ralph run` handles safety via pre-verify.
4. `ralph run <feature>` enters the main loop:
   - Readiness gate: refuses to run if not inside a git repo, `sh` is missing, `.ralph/` is not writable, QA commands are missing/placeholder, or command binaries aren't in PATH
   - Signal handling: SIGINT/SIGTERM releases the lock and exits with code 130
   - Loads config + PRD, acquires lock, ensures git branch, starts services
   - **Pre-verify phase**: runs verification on ALL non-blocked stories before implementation
     - Stories that pass are marked as passed (catches already-implemented work)
     - Stories that fail but were previously marked passed are reset to pending (catches PRD changes that invalidate previous work)
     - This uses `ResetStoryForPreVerify` which does NOT increment retry count
   - Each iteration: picks next story (highest priority, not passed/blocked), generates prompt, spawns provider subprocess, captures output with marker detection
   - After provider signals `<ralph>DONE</ralph>`, CLI runs verification commands + browser checks (console errors are hard failures) + service health checks
   - Pass -> mark story complete; Fail -> retry (up to maxRetries, then block)
   - When all stories pass -> final verification (verify commands + browser verification for all UI stories + service health) -> provider reviews -> `VERIFIED` or `RESET`
   - Learnings are saved on every path (including timeout/error)
5. `ralph verify <feature>` runs verification only (same readiness gates, starts services for browser verification)
6. `ralph logs <feature>` views run history and logs:
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
- **Story selection**: CLI picks the next story by priority, skipping passed/blocked
- **Branch management**: CLI creates/switches to the `ralph/<feature>` branch (both `ralph prd` and `ralph run` call `EnsureBranch` before any commits)
- **State updates**: CLI marks stories passed/failed/blocked based on verification results
- **Verification**: CLI runs `verify.default` and `verify.ui` commands, not the provider
- **Browser testing**: CLI executes browserSteps via rod; console errors are hard failures
- **Service management**: CLI starts/stops/restarts dev servers; captures output; checks health during verification
- **Learning management**: CLI deduplicates learnings and saves them on every code path (including timeout/error)
- **Crash recovery**: CLI tracks `currentStoryId` in prd.json to resume interrupted work
- **Concurrency control**: CLI uses lock file to prevent parallel runs
- **PRD persistence**: CLI commits both prd.md and prd.json. `ralph prd` commits prd.md after creation and both files after finalization. `ralph run` commits prd.json state changes atomically (including STUCK/timeout/no-DONE paths)

### Provider handles (told via prompts in prompts/run.md):
- **Code implementation**: Write the code for the assigned story
- **Writing tests**: Create tests for the implementation
- **Local checks**: Run linters/tests before committing (as a sanity check)
- **Git commits**: Commit implementation with `feat: US-XXX - Title` format
- **Signal markers**: Output `<ralph>DONE</ralph>`, `<ralph>STUCK</ralph>`, etc.
- **Knowledge updates**: Update AGENTS.md/CLAUDE.md with discovered patterns
- **Learnings**: Output `<ralph>LEARNING:...</ralph>` for cross-iteration memory
- **Documentation verification**: Check implementations against cached framework source code or web search

### User Contract

**User must provide (prompted during `ralph init`):**
- `provider.command` — which AI CLI to use
- `verify.default` — typecheck/lint/test commands
- `services` — dev server config (start command + ready URL)

**User must provide (manual config, only if needed):**
- `verify.ui` — e2e test commands (only if UI stories)
- `browser.executablePath` — if auto-download won't work

**Ralph handles automatically:**
- Provider args, promptMode, knowledgeFile (auto-detected from known providers)
- Verify command suggestions (auto-detected from package.json scripts, go.mod, Cargo.toml, pyproject.toml, mix.exs, requirements.txt)
- Browser binary (auto-downloaded if needed for UI stories)
- Resource caching (auto-detected from dependencies)
- Git branch management, story selection, verification orchestration
- All PRD state management

### Provider communication protocol (markers in stdout or stderr):
| Marker | Meaning |
|--------|---------|
| `<ralph>DONE</ralph>` | Story implementation complete, ready for CLI verification |
| `<ralph>STUCK</ralph>` | Cannot proceed, counts as failed attempt |
| `<ralph>BLOCK:US-XXX</ralph>` | Story is impossible, skip permanently (comma-separated for multiple: `BLOCK:US-001,US-003`) |
| `<ralph>LEARNING:text</ralph>` | Save insight for future iterations |
| `<ralph>SUGGEST_NEXT:US-XXX</ralph>` | Advisory hint for story ordering |
| `<ralph>VERIFIED</ralph>` | Final verification passed (verify phase only) |
| `<ralph>RESET:US-001,US-003</ralph>` | Reset stories for rework (verify phase only) |
| `<ralph>REASON:text</ralph>` | Explanation for STUCK/BLOCK/RESET |

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

For interactive sessions (`ralph prd`, `ralph refine`), `stdin` prompt mode is automatically overridden to `arg` since the provider needs stdin for interactive input. Non-interactive flags (`--print`, `-p`) are also stripped so the provider runs as a full interactive CLI session where the user can converse with the AI.

## Key Differences from Original Ralph (snarktank/ralph v1)

The original was a ~90-line bash script (`ralph.sh`) with a simple for loop:

| Aspect | v1 (bash) | v2 (this repo) |
|--------|-----------|----------------|
| Language | Bash script | Go binary |
| Providers | Amp or Claude (hard-coded if/else) | Any AI CLI (provider-agnostic) |
| Story selection | AI agent decides | CLI decides (deterministic) |
| State management | AI updates prd.json directly | CLI manages all state |
| Verification | AI runs checks itself | CLI runs verification commands |
| Completion signal | `<promise>COMPLETE</promise>` | Rich marker protocol (DONE, STUCK, BLOCK, etc.) |
| Memory | `progress.txt` (append-only file) | `prd.json.run.learnings[]` |
| Loop limit | Fixed iterations (default 10) | Infinite until verified |
| Browser testing | Delegated to provider skills (dev-browser) | Built-in rod automation |
| Service management | None | Start/ready/restart lifecycle |
| Crash recovery | None | `currentStoryId` tracking |
| Concurrency safety | None | Lock file |
| Multi-feature | Manual archive/switch | Date-prefixed feature directories |
| PRD workflow | External skills for Amp/Claude plugins | Built-in interactive state machine |
| Self-update | None | `ralph upgrade` from GitHub releases |

The fundamental shift: v1 trusted the AI to manage its own workflow. v2 treats the AI as a pure code-writing tool within a deterministic orchestration framework.

## Configuration (ralph.config.json)

```json
{
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
  "browser": {
    "enabled": true,
    "headless": true,
    "executablePath": "/usr/bin/chromium",
    "screenshotDir": ".ralph/screenshots"
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
    "cacheDir": "~/.ralph/resources",
    "custom": []
  }
}
```

`promptMode`, `promptFlag`, `args`, and `knowledgeFile` are auto-detected from `provider.command` if not set. Only `command` is required. `verify.timeout` sets the per-command timeout in seconds (default: 300). `logging` controls the observability system (all options default to shown values). `resources` configures framework source code caching (enabled by default).

## PRD Schema (v2)

```json
{
  "schemaVersion": 2,
  "project": "Name",
  "branchName": "ralph/feature",
  "description": "Feature description",
  "run": {
    "startedAt": "RFC3339 timestamp",
    "currentStoryId": "US-001 or null",
    "learnings": ["accumulated insights"]
  },
  "userStories": [{
    "id": "US-001",
    "title": "...",
    "description": "...",
    "acceptanceCriteria": ["..."],
    "tags": ["ui"],
    "priority": 1,
    "passes": false,
    "retries": 0,
    "blocked": false,
    "lastResult": { "completedAt": "...", "commit": "abc123", "summary": "..." },
    "notes": "",
    "browserSteps": [{ "action": "navigate", "url": "/path" }]
  }]
}
```

Story lifecycle: `pending (passes=false)` -> provider implements -> CLI verifies -> `passed (passes=true)` or `retry (retries++)` or `blocked (blocked=true)`.

## Codebase Patterns

- **Atomic state updates**: All prd.json writes go through `AtomicWriteJSON` (write temp -> validate -> rename). Never write prd.json directly.
- **Marker detection**: `processLine()` in loop.go scans each line of provider stdout and stderr. Lines are trimmed and matched as whole lines (not substrings) to prevent marker spoofing via embedded text. Simple markers (DONE, STUCK, VERIFIED) use `==` comparison; parameterized markers use `^...$` anchored regexes. Markers are detected during execution, not after.
- **Provider subprocess**: Spawned via `os/exec` with context timeout. Three modes for prompt delivery (stdin pipe, arg append, temp file).
- **Lock file**: JSON with pid/startedAt/feature/branch. Stale detection via `isLockStale()` which checks both process liveness (`kill(pid, 0)`) and lock age (24h max, guards against PID reuse). Atomic creation via `O_CREATE|O_EXCL`.
- **Feature directories**: `.ralph/YYYY-MM-DD-feature/` or `.ralph/YYYYMMDD-feature/` format. `FindFeatureDir` finds most recent match by suffix.
- **Feature name matching**: `FindFeatureDir` matches feature names case-insensitively via `strings.EqualFold`. `ralph run Auth` and `ralph run auth` find the same feature directory.
- **Browser steps**: Defined in prd.json per story. Executed by rod in headless Chrome. Screenshots saved to `.ralph/screenshots/`.
- **Console error capture**: BrowserRunner captures both `RuntimeExceptionThrown` (uncaught exceptions) and `RuntimeConsoleAPICalled` (`console.error()`) events. Access to `consoleErrors` is protected by `sync.Mutex` since event handlers run in goroutines. Both `RunSteps` and `checkURL` wait 1 second before collecting console errors to allow async JS errors to propagate.
- **Browser launcher cleanup**: BrowserRunner stores the `launcher.Launcher` reference and calls `launcher.Kill()` in `close()` to ensure the Chrome process is terminated. Error paths in `init()` also call `launcher.Kill()` to prevent orphaned processes.
- **Console error enforcement**: Browser console errors are hard verification failures (not warnings). Checked in both per-story and final verification.
- **Browser pre-download**: `EnsureBrowser()` pre-resolves the Chromium binary before the main loop. Uses `os.Stat` (not `Validate()`) for fast path (~1μs vs ~2s). Mutates `config.Browser.ExecutablePath` in-place on success; sets `Enabled=false` on download failure to prevent silent per-story retry. Gated on UI stories — skips entirely if no stories have the "ui" tag. `CheckBrowserStatus()` provides read-only status for `cmdDoctor`.
- **Default branch detection**: `DefaultBranch()` tries `origin/HEAD` symbolic ref first, then falls back to checking if `main` or `master` branch exists locally, then checks `origin/main` or `origin/master` remote tracking branches. Diff-based functions (`GetChangedFiles`, `HasFileChanged`, `GetDiffSummary`) fall back from three-dot diff to two-dot diff when the merge-base is unavailable.
- **Process group killing**: Services and provider subprocesses are started with `Setpgid: true` so `syscall.Kill(-pid, SIGTERM)` kills the entire process group (including child processes). This prevents orphaned processes on timeout or signal. Provider subprocesses use `cmd.Cancel` + `cmd.WaitDelay` (matching `runCommand()`) to ensure the process group is killed on context timeout and `cmd.Wait()` is always called to prevent zombie processes.
- **CleanupCoordinator**: Signal handlers use a `CleanupCoordinator` that resources register with when created. On SIGINT/SIGTERM, the coordinator kills provider process groups, stops services, logs run end, and releases locks — all before calling `os.Exit(130)`. This ensures cleanup happens even when defer statements would be bypassed.
- **Service ready checks**: HTTP GET polling every 500ms until status < 500, with configurable timeout. ServiceManager reuses a single `http.Client` instance to avoid repeated allocations.
- **Idempotent StopAll**: `ServiceManager.StopAll()` sets `processes` to nil after stopping, making it safe to call multiple times (e.g., from both defer and signal handler).
- **Service output capture**: `capturedOutput` trim buffer (keeps last ~50% on overflow) captures service stdout/stderr. Service output is not printed to the console — only captured for diagnostics via `GetRecentOutput()`.
- **Service health checks**: `CheckServiceHealth()` polls service ready URLs during verification to detect crashed/unresponsive services.
- **Readiness gates**: `CheckReadiness()` in config.go validates: `sh` is in PATH, project is inside a git repository, `.ralph/` directory exists and is writable, `browser.executablePath` exists if explicitly set, QA command binaries exist in PATH (`extractBaseCommand()` + `exec.LookPath`), service `start` commands are available (placeholder service commands are flagged separately), and PRD browserSteps require browser enabled. Called before `ralph run` and `ralph verify`. `CheckReadinessWarnings(cfg)` returns soft warnings (e.g., unknown provider defaults).
- **Learning deduplication**: `AddLearning()` normalizes (case-insensitive, trimmed) before checking for duplicates. Learnings are saved on all code paths: success, timeout, error, STUCK, and no-DONE. Prompt delivery caps at 50 most recent learnings to prevent context overflow.
- **PRD quality warnings**: `WarnPRDQuality()` checks stories for missing "Typecheck passes" acceptance criteria (soft warning, does not block).
- **Prompt templates**: Embedded via `//go:embed prompts/*`. Simple `{{var}}` string replacement (not Go templates).
- **Update check**: Background goroutine with 5s timeout, cached to `~/.config/ralph/update-check.json` for 24h (falls back to `/tmp/ralph-update-check.json` if home directory is unavailable). Non-blocking: skipped silently if check hasn't finished by CLI exit. Disabled for `dev` builds and `ralph upgrade`. Cache uses `isNewerVersion()` for proper semver comparison (not just string inequality) to avoid suggesting downgrades after upgrades.
- **Resources module**: `ResourceManager` auto-detects project dependencies via `ExtractDependencies()`, maps them to source repos via `MapDependencyToResource()`, and caches full repos in `~/.ralph/resources/`. Shallow clones (`--depth 1 --single-branch`) minimize disk usage. `EnsureResources()` is called before the story loop to sync detected frameworks. Cache metadata is stored in `registry.json`. Use `ralph resources list/sync/clear` for manual management. `ralph doctor` shows cache status.
- **Resource verification instructions**: Prompts include `{{resourceVerificationInstructions}}` which tells the agent about cached framework source code. When resources are cached, agents can read actual source to verify implementations. Falls back to web search instructions when no resources are cached.
- **Verification summary**: `runFinalVerification` accumulates structured PASS/FAIL/WARN lines for each verification step (including truncated command output for failures) and passes them to the verify prompt as `verifySummary`.
- **Git diff in verify prompt**: `GetDiffSummary()` provides `git diff --stat` output from the default branch to HEAD, injected into the verify prompt as `diffSummary`.
- **KnowledgeFile change detection**: `HasFileChanged()` in git.go checks if a file was modified on the current branch vs default branch using `git diff --name-only`. Used in final verification to report whether the knowledgeFile was updated.
- **Criteria checklist**: `buildCriteriaChecklist()` in prompts.go generates a structured checkbox list of all non-blocked stories' acceptance criteria for the verify prompt.
- **Commit-exists gate**: After provider signals DONE, CLI checks HEAD changed from pre-run snapshot (captured AFTER PRD commit to avoid false positives). No new commit = automatic retry counted toward maxRetries.
- **Verify command timeout**: Each verify command has a configurable timeout (`verify.timeout`, default 300s). Prevents hanging test suites from blocking the loop indefinitely.
- **Test file heuristic**: `HasTestFileChanges()` checks if any files matching test patterns (`*_test.*`, `*.test.*`, `*.spec.*`, `__tests__/`) were modified on the branch. Result is included as PASS/WARN in the verification summary.
- **Run logging**: `RunLogger` in logger.go writes JSONL event logs to `.ralph/YYYY-MM-DD-feature/logs/run-NNN.jsonl`. Events include timestamps, durations, story context, and full provider/verification output. Logs are auto-rotated to keep only `maxRuns` most recent. Key methods: `StoryStart(storyID, title)` logs story iteration beginning, `StoryEnd(storyID, success)` logs story iteration completion, `BrowserStep(action, success, details)` logs individual browser step execution, `ProviderLine(stream, line)` streams each provider output line in real-time.
- **Console output suppression**: `ralph run` shows a clean dashboard — provider output, verification command output, and service output are NOT printed to the console. Provider output is streamed line-by-line to JSONL (`provider_line` events) for real-time viewing via `ralph logs -f`. Verification commands only show the `→ command` and `✓/✗ (duration)` status lines. Key markers (DONE, STUCK, BLOCK, LEARNING) are printed to console as detected.
- **Timestamped console output**: When `logging.consoleTimestamps` is true, status lines are prefixed with `[HH:MM:SS]`.
- **Event types**: `run_start`, `run_end`, `iteration_start/end`, `story_start/end`, `provider_start/end`, `provider_output`, `provider_line`, `marker_detected`, `verify_start/end`, `verify_cmd_start/end`, `browser_start/end`, `browser_step`, `service_start/ready/restart/health`, `state_change`, `learning`, `warning`, `error`. Note: `provider_output` logs stdout/stderr in bulk after completion; `provider_line` streams each line in real-time.
- **Pre-verify phase**: `preVerifyStories()` runs verification on ALL non-blocked stories before the implementation loop. Stories that pass are marked as passed (catches already-implemented work). Stories that fail but were previously marked passed are reset to pending using `ResetStoryForPreVerify()`, which does NOT increment retry count. This enables the "infinite loop" pattern: modify PRD → run → pre-verify detects invalid stories → re-implement.
- **Pre-verify code change guard**: `preVerifyStories()` calls `HasNonRalphChanges()` before running any verification. If only `.ralph/` files changed on the branch (e.g., prd.md, prd.json), pre-verify is skipped entirely — global verify commands pass vacuously on fresh branches with no code, producing false positives.
- **Codebase discovery**: `DiscoverCodebase()` in discovery.go detects tech stack (go, typescript, python, rust, elixir), package manager (bun, npm, yarn, pnpm, go, cargo, mix), frameworks, and full dependency list from config files. Used in PRD creation and resource syncing. Detection is lightweight (reads config files, doesn't run commands). `ExtractDependencies()` returns `[]Dependency` with name, version, and isDev flag.
- **Dependency extraction**: `extractJSDependencies()` parses package.json, `extractGoDependencies()` parses go.mod, `extractPythonDependencies()` parses pyproject.toml/requirements.txt, `extractRustDependencies()` parses Cargo.toml, `extractElixirDependencies()` parses mix.exs. The `Dependency` struct includes Name, Version, and IsDev fields.
- **Provider selection prompt**: `ralph init` prompts the user to select from `providerChoices` (alphabetically sorted known providers) or enter a custom command. `promptProviderSelection()` accepts a `*bufio.Reader` so tests can inject controlled input. `providerChoices` must stay in sync with `knownProviders` map (enforced by `TestProviderChoices_MatchKnownProviders`).
- **Verify command prompts**: `promptVerifyCommands()` in commands.go prompts for 3 commands (typecheck, lint, test) during `ralph init`. Accepts `*bufio.Reader` and detected defaults `[3]string` for testability. When defaults are detected, pressing Enter accepts them; typing overrides. Non-empty inputs go into `verify.default`; all skipped falls back to placeholder echo commands.
- **Service config prompt**: `promptServiceConfig()` in commands.go prompts for dev server start command and ready URL during `ralph init`. Returns `*ServiceConfig` (nil if skipped, placeholder used instead). Auto-prepends `http://` if no scheme. Services are required — `validateConfig()` enforces at least one entry.
- **Browser skip warnings**: When browser verification is skipped for UI stories (browser disabled or no service ready URL), warnings are logged in both `runStoryVerification()` and `runFinalVerification()`. In final verification, WARN lines are added to `summaryLines` so the verify agent sees them.
- **Verify command auto-detection**: `DetectVerifyCommands()` in discovery.go reads project config files to pre-fill verify command prompts during `ralph init`. For JS/TS projects, reads `scripts` from package.json (prefers `test:unit` over `test`). For Go, suggests `go vet ./...` and `go test ./...`; detects `golangci-lint run` if `.golangci.yml`/`.golangci.yaml` exists. For Rust, suggests `cargo check` and `cargo test`. For Python, suggests `pytest` if in dependencies. For Elixir, suggests `mix compile --warnings-as-errors` and `mix test`; detects `mix credo` if `:credo` is in mix.exs deps. Only suggests commands that are 100% deterministic from config files — never guesses.
- **`.ralph/` existence check in CheckReadiness**: `CheckReadiness` verifies `.ralph/` exists before checking writability. Missing directory produces "Run 'ralph init' first" message.
- **Services required**: `validateConfig()` enforces at least one service entry. Also validates that `services[].ready` starts with `http://` or `https://`. Catches typos like `localhost:3000` (missing scheme) at config load time. `WriteDefaultConfig()` writes a placeholder service if none provided during init.
- **Browser executable path validation**: `CheckReadiness` validates `browser.executablePath` exists on disk if explicitly set and browser is enabled.
- **Unknown provider warning**: `CheckReadinessWarnings()` warns when `provider.command` is not in `knownProviders` map. Non-blocking warning printed before every run/verify.
- **Interactive sessions strip non-interactive flags**: `runProviderInteractive()` removes `--print`/`-p` from provider args via `stripNonInteractiveArgs()` so the provider runs as a conversational CLI session. Used by `ralph prd` and `ralph refine`. The `ralph run` loop (which uses `runProvider()` in loop.go) is unaffected.
- **Interactive commands skip Setpgid**: `Command.Run()` in prd.go does NOT set `Setpgid: true` — the provider must stay in ralph's foreground process group to read from the terminal. `Setpgid` would put it in a background group, causing SIGTTIN on stdin reads (freezing the process). The run loop in loop.go uses `Setpgid: true` since it communicates via pipes, not the terminal.
- **`ralph prd` validates git + ensures branch + warns about placeholder commands**: `cmdPrd()` calls `checkGitAvailable()`, ensures `ralph/<feature>` branch via `EnsureBranch`, and warns (without blocking) if verify.default contains placeholder commands. `commitPrdFile()` has a defense-in-depth check that refuses to commit unless on a `ralph/` branch. The simplified finalized menu offers Edit prd.md/Edit prd.json/Start execution/Quit (no regeneration or AI refine — that's `ralph refine`).
- **Story state transitions are total**: `MarkStoryPassed` clears `Blocked`, `MarkStoryFailed` clears `Passes` and `LastResult`, `MarkStoryBlocked` clears `Passes` and `LastResult`. This prevents conflicting state (e.g., a story that is both passed and blocked).
- **Crash recovery via CurrentStoryID**: `GetNextStory` checks `prd.Run.CurrentStoryID` first and returns that story if it's still eligible (not passed, not blocked). Falls through to normal priority selection if the story is ineligible or nonexistent.
- **EnsureBranch dirty-tree guard**: `EnsureBranch` refuses to switch to an existing branch with uncommitted changes (returns error). Creating a new branch with dirty tree is allowed (changes carry over safely). Already on the right branch skips the check entirely.
- **commitPrdOnly error handling**: All `commitPrdOnly` call sites check the returned error and log a warning via `logger.Warning()`. PRD state commit failures are non-fatal (the loop continues) but are surfaced.
- **`aggregateBrowserResults` helper**: `RunSteps` fallback to `RunChecks` aggregates all URL check results into a single `BrowserCheckResult` with merged console errors and the first error propagated.
- **Scanner buffer overflow logging**: Both stdout and stderr scanners in `runProvider` check `scanner.Err()` after completion and log warnings for buffer overflows (lines >1MB).
- **ResetStoryForPreVerify vs ResetStory**: `ResetStory` (called from verify phase RESET marker) increments retries and can block the story. `ResetStoryForPreVerify` (called during pre-verify) resets without incrementing retries — the story wasn't re-attempted, it just became invalid due to PRD changes.
- **`ralph refine` context loading**: `generateRefinePrompt()` reads prd.md + prd.json from disk, discovers codebase context, builds git diff summary, and includes verify commands + service URLs. The resulting prompt gives the AI comprehensive feature context for free-form interactive work. No post-processing — `ralph run`'s pre-verify handles safety.
- **Story ID stability in finalize prompt**: `prd-finalize.md` instructs the provider to preserve existing `US-XXX` IDs from prd.md, only assigning new sequential IDs to genuinely new stories.

## Prompt Template Variables

Each prompt template uses `{{var}}` placeholders replaced by `prompts.go`:

| Template | Variables |
|----------|-----------|
| `run.md` | `storyId`, `storyTitle`, `storyDescription`, `acceptanceCriteria`, `tags`, `retryInfo`, `verifyCommands`, `learnings`, `knowledgeFile`, `project`, `description`, `branchName`, `progress`, `storyMap`, `browserSteps`, `serviceURLs`, `timeout`, `resourceVerificationInstructions` |
| `verify.md` | `project`, `description`, `storySummaries`, `verifyCommands`, `learnings`, `knowledgeFile`, `prdPath`, `branchName`, `serviceURLs`, `diffSummary`, `resourceVerificationInstructions`, `verifySummary`, `criteriaChecklist` |
| `prd-create.md` | `feature`, `outputPath`, `codebaseContext` |
| `prd-finalize.md` | `feature`, `prdContent`, `outputPath` |
| `refine.md` | `feature`, `prdMdContent`, `prdJsonContent`, `progress`, `storyDetails`, `learnings`, `diffSummary`, `codebaseContext`, `verifyCommands`, `serviceURLs`, `knowledgeFile`, `branchName`, `featureDir` |

## Non-obvious Behaviors

- **`AllStoriesComplete` treats blocked as complete**: Blocked stories do not prevent the "all stories complete" state — they are skipped and the loop proceeds to final verification.
- **`runProvider` returns nil error on non-zero exit**: A provider exiting with code != 0 is not treated as an error — it returns `(result, nil)`. The loop checks markers in the result to determine outcome. If `runProvider` returns `(nil, err)` (provider failed to start), the loop and final verification guard against nil pointer dereference when logging markers.
- **`runVerify` ensures correct branch**: `runVerify` calls `EnsureBranch` after loading the PRD to ensure verification runs on the feature branch, not whatever branch was checked out.
- **`ralph verify` refuses incomplete stories**: `runVerify` checks `AllStoriesComplete()` before running final verification. If stories are still pending, it prints which ones and returns an error.
- **Final verification restarts services once**: `runFinalVerification` restarts services once before the browser verification loop, not per-story. If the restart fails, all browser checks for that run are marked as failed.
- **Verification commands run through `sh -c`**: All verification commands are wrapped in `sh -c`, so shell features (pipes, redirects, etc.) are available. `runCommand()` uses `Setpgid: true` and `cmd.Cancel` to kill the entire process group (`syscall.Kill(-pid, SIGKILL)`) on timeout, preventing orphaned test/lint processes.
- **`cmdVerify` acquires the lock**: Not just `cmdRun` — verification also acquires the lock file to prevent concurrent operations.
- **Only one REASON marker is kept**: If multiple `<ralph>REASON:...</ralph>` markers are emitted, each overwrites the previous. Only the last one survives.
- **SUGGEST_NEXT is advisory only**: The marker is captured but `GetNextStory` selects purely by priority. The suggestion is not currently acted upon.
- **Verification output is captured for retries**: When verification fails, the last 50 lines of command output are stored in `story.Notes` so the retry agent can see what specifically failed.
- **Final VERIFIED is gated on all verification**: If any `verify.default`, `verify.ui` command, browser step, or service health check failed during final verification, the CLI overrides a provider's `VERIFIED` marker and returns not-verified. The provider cannot skip past failing checks.
- **FAIL summary lines include command output**: When verification commands fail in `runFinalVerification`, the truncated output (last 50 lines) is appended to the FAIL line in `verifySummary`, so the verify agent can see what failed without re-running commands.
- **Browser fallback in final verify**: Final verification runs browser checks for ALL UI stories, not just those with explicit `browserSteps`. Stories without steps get the `RunChecks` fallback (URL-based page load + console error detection).
- **KnowledgeFile modification check**: Final verification checks whether the configured `knowledgeFile` (AGENTS.md/CLAUDE.md) was modified on the branch and reports WARN/PASS in the summary.
- **Acceptance criteria checklist**: The verify prompt includes a structured checkbox checklist (`criteriaChecklist`) of every non-blocked story's acceptance criteria, making it harder for the verify agent to skip criteria checks.
- **Browser init/restart failures fail verification**: Both per-story and final verification treat browser initialization errors and service restart failures as hard failures. Previously these were logged as warnings and silently continued.
- **Provider must commit code to pass**: Signaling DONE without making a new git commit is treated as a failed attempt and counts toward maxRetries. The pre-run commit hash is captured AFTER the PRD state commit to avoid false positives.
- **Working tree cleanliness is checked but not enforced**: Uncommitted files after provider finishes generate a warning but don't fail the story. This catches providers that left untracked or modified files.
- **Test file heuristic in verify summary**: If no test files (`*_test.*`, `*.test.*`, `*.spec.*`, `__tests__/`) are modified on the branch, a WARN is included in the `verifySummary` for the verify agent to act on.
- **`$EDITOR` validation in PRD editing**: `prdEditManual()` validates the editor binary exists before spawning. Falls back from `$EDITOR` to `nano`, but checks either is available. Returns a clear error with instructions to set `$EDITOR`.
- **`ralph prd` warns about placeholder verify commands but does not block**: During `cmdPrd()`, if verify.default contains placeholder commands, a warning is printed to stderr. PRD creation proceeds normally — the user can still create PRDs but needs to fix config before `ralph run`.
- **Browser URL extraction from acceptance criteria**: `extractURLs()` in browser.go scans acceptance criteria text for path patterns ("navigate to /path", "visit /page", "accessible at /url") and explicit `http(s)://` URLs. If no URLs are found and the story is tagged "ui", the base URL from services config is used as fallback. This means UI stories get basic browser verification (page load + console error check) even without explicit `browserSteps`.
- **EnsureBrowser skips without UI stories**: If no story in the PRD has a "ui" tag, `EnsureBrowser` returns immediately — no `os.Stat`, no download attempt. Failed browser download disables browser for the entire run (`Enabled=false`), preventing repeated download retries on each story.
- **Pre-verify skips on fresh branches**: If no files outside `.ralph/` changed on the branch (checked via `HasNonRalphChanges()`), `preVerifyStories()` returns immediately. On a fresh branch with only PRD files, global verify commands (typecheck, lint, test) pass vacuously, falsely marking all stories as "already implemented."
- **`prdFinalize` is a one-shot operation**: `prdFinalize()` converts prd.md to prd.json. There is no state merging — once prd.json exists, the finalized menu doesn't offer regeneration. For post-finalization work, use `ralph refine`.
- **`ralph refine` requires prd.json**: `cmdRefine()` requires prd.json to exist (the feature must be finalized). It loads the PRD, ensures the feature branch, and opens an interactive session. No lock is acquired (interactive session, not automated).
- **`prdStateFinalized` shows progress and refine hint**: When returning to a finalized PRD, the menu shows current progress (e.g., "Progress: 3/5 stories complete (1 blocked)") if any stories have been worked on, and prints a tip about `ralph refine`.

## Testing

Tests are in `*_test.go` files alongside source. Key test files:
- `commands_test.go` - provider selection prompt, verify command prompts (with detected defaults), service config prompt, provider choices validation, gitignore creation
- `config_test.go` - config loading, validation, provider defaults, readiness checks (including placeholder service detection, browserSteps cross-check), command validation, verify timeout defaults, services required validation
- `schema_test.go` - PRD validation, story state transitions (including flag clearing), browser steps, learning deduplication, PRD quality, ResetStoryForPreVerify, GetPendingStories, GetNextStory crash recovery via CurrentStoryID
- `discovery_test.go` - tech stack detection (including Elixir), framework detection (including Phoenix), codebase context formatting, verify command auto-detection, Elixir dependency extraction
- `services_test.go` - output capture, service manager, health checks
- `prompts_test.go` - prompt generation, variable substitution, provider-agnosticism, generateRefinePrompt, buildRefinementStoryDetails
- `loop_test.go` - marker detection (including REASON overwrite, whole-line matching, embedded marker rejection, whitespace tolerance), nil ProviderResult guard, provider result parsing, command timeout, runCommand
- `feature_test.go` - directory parsing, timestamp formats, case-insensitive matching
- `lock_test.go` - lock acquisition, stale detection (including age-based isLockStale)
- `browser_test.go` - runner init, screenshot saving, console error mutex safety, formatConsoleArgs, aggregateBrowserResults
- `cleanup_test.go` - CleanupCoordinator idempotency, nil resource handling
- `atomic_test.go` - atomic writes, JSON validation
- `git_test.go` - git operations (branch, commit, checkout, EnsureBranch, EnsureBranch dirty-tree guard, DefaultBranch fallback chain including origin/main and origin/master, GetDiffSummary, HasNewCommitSince, IsWorkingTreeClean, GetChangedFiles two-dot fallback, HasTestFileChanges, HasNonRalphChanges)
- `update_check_test.go` - update check cache path, isNewerVersion semver comparison
- `logger_test.go` - event logging, run numbering, log rotation, event filtering, run summaries
- `resources_test.go` - ResourceManager, ResourcesConfig, cache operations, FormatSize
- `resourcereg_test.go` - MapDependencyToResource, MergeWithCustom, DefaultResources validation (including Phoenix/Elixir ecosystem entries)
- `prd_test.go` - editor validation in prdEditManual, stripNonInteractiveArgs
- `external_git_test.go` - ExternalGitOps, Exists, GetRepoSize
- `e2e_test.go` - Full end-to-end test (`//go:build e2e`) exercising init → prd → run → verify on real project with real Claude

Run with `go test ./...` or `go test -v ./...` for verbose output.

### E2E Tests

`e2e_test.go` contains a full end-to-end test (`//go:build e2e`) that exercises the complete user journey on a real TypeScript project (warrantycert) with real Claude CLI. No mocking, no pre-crafted data.

**Prerequisites:** `claude` CLI (configured with API key), `bun`, `git`, internet access, system Chromium or auto-download capability.

**Run:** `make test-e2e` (or `go test -tags e2e -timeout 60m -v -run TestE2E ./...`)

**What it tests (15 sequential phases):**
0. **Smoke Tests** — `--help`, `--version`, unknown command (exit codes + output patterns)
1. **Project Setup** — Clone warrantycert, `bun install`, database setup, Playwright install, baseline typecheck
2. **ralph init** — Interactive provider selection (claude) + verify command acceptance (detected defaults)
3. **Config Enhancement** — Add services, verify.ui, browser config programmatically
4. **ralph doctor** — Validate all environment checks pass (config, provider, dirs, tools, git)
5. **ralph prd** — Create + finalize PRD for "certificate-search" feature (real Claude brainstorming)
6. **Pre-Run Checks** — validate (schema + story count), status (no-arg + feature), next commands
7. **ralph run** — First implementation run (25 min timeout, real Claude coding)
8. **Post-Run Analysis** — Parse PRD state, JSONL event assertions (run_start, provider_start, verify_start, story_start, service_start, marker_detected), git history
9. **PRD Refinement** — Conditional: refine if not all stories passed, re-validate after
10. **Second Run** — Conditional: re-run with refined PRD (20 min timeout)
11. **Status + Logs + Resources** — status (no-arg), logs --list/--summary/--json/--run/--type/--story, resources list/path
12. **ralph verify** — Conditional: final verification if all stories passed
13. **Post-Run Doctor** — Verify doctor still passes after full run (no stale locks, etc.)
14. **Report** — Comprehensive summary with per-story breakdown

**Key design:** Uses `promptResponder` (expect-style stdin interaction) to drive interactive sessions — watches stdout for prompt patterns and responds just-in-time. Process tracker (`processTracker`) ensures all ralph subprocesses are killed on test abort via `t.Cleanup`. JSONL event assertions validate the internal event stream (run_start, provider_start, verify_start, etc.). The test validates Ralph's full orchestration pipeline including service management, browser verification, and e2e test execution.

**Artifact directory:** Each run saves structured output to `e2e-runs/<timestamp>/` (override with `RALPH_E2E_ARTIFACT_DIR` env var). Structure:
```
e2e-runs/2026-02-10T15-04-05/
  report.md              # Human-readable report organized by story/idea
  result.txt             # PASS, PARTIAL, FAIL, or INCOMPLETE
  summary.json           # Machine-readable: stories, branch, run_number, config
  config.json            # ralph.config.json used
  prd.md / prd.json      # Original PRD
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
      browser-steps.txt  # Browser step events from JSONL (if UI story)
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
- **Add a new browser action**: Add case to `executeStep()` switch in browser.go
- **Add a new marker**: Add regex pattern and field to `ProviderResult` in loop.go, handle in `processLine()`
- **Add a new command**: Add case to `main()` switch in main.go, implement handler in commands.go
- **Modify prompt**: Edit the `.md` file in `prompts/` directory (embedded at compile time)
- **Change PRD schema**: Update structs in schema.go, bump schemaVersion, update validation
