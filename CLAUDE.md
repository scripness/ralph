# CLAUDE.md

## Project Overview

Ralph v2 is a Go CLI that orchestrates AI coding agents in an infinite loop to autonomously implement software features defined in a PRD (Product Requirements Document). It is a complete rewrite of the original [snarktank/ralph](https://github.com/snarktank/ralph) bash script.

The core idea ("the Ralph Pattern"): break work into small user stories, spawn fresh AI instances to implement them one at a time, verify each with automated tests, persist learnings for future iterations, and repeat until all stories pass.

## Architecture

```
main.go           CLI entry point, command dispatch
commands.go       Command handlers (init, run, verify, prd, status, next, validate, doctor); doctor checks lock status
config.go         Configuration loading, validation, provider defaults, readiness checks
schema.go         PRD data structures, story state machine, validation, PRD quality checks
prompts.go        Prompt template loading + variable substitution (//go:embed prompts/*)
loop.go           Main agent loop, provider subprocess management, verification orchestration
browser.go        Browser automation via rod (Chrome DevTools Protocol)
prd.go            Interactive PRD state machine (create/refine/finalize)
feature.go        Date-prefixed feature directory management (.ralph/YYYY-MM-DD-feature/)
services.go       Dev server lifecycle, output capture, health checks
git.go            Git operations (branch, commit, status)
lock.go           Concurrency lock file (.ralph/ralph.lock)
atomic.go         Atomic file writes (temp + rename)
upgrade.go        Self-update via go-selfupdate (scripness/ralph)
update_check.go   Background update check with 24h cache
utils.go          fileExists helper
```

Prompt templates live in `prompts/` and are embedded at compile time:
- `run.md` - story implementation instructions sent to provider
- `verify.md` - final verification instructions
- `prd-create.md` - PRD brainstorming prompt
- `prd-refine.md` - PRD refinement prompt
- `prd-finalize.md` - PRD to JSON conversion prompt

## Build and Test

```bash
make build    # go build -ldflags="-s -w" -o ralph .
make test     # go test ./...
```

Go version: 1.25.6. Key dependencies: `github.com/go-rod/rod` for browser automation, `github.com/creativeprojects/go-selfupdate` for self-update.

## How the CLI Works (End-to-End)

1. `ralph init` creates `ralph.config.json` and `.ralph/` directory (with `.ralph/.gitignore`). Use `--force` to overwrite existing config.
2. `ralph prd <feature>` runs an interactive state machine with a menu-driven flow: create prd.md -> refine (edit/regenerate) -> finalize to prd.json. Each phase presents lettered options (a/b/c/q) via stdin.
3. `ralph run <feature>` enters the main loop:
   - Readiness gate: refuses to run if QA commands are missing/placeholder or command binaries aren't in PATH
   - Signal handling: SIGINT/SIGTERM releases the lock and exits with code 130
   - Loads config + PRD, acquires lock, ensures git branch, starts services
   - Each iteration: picks next story (highest priority, not passed/blocked), generates prompt, spawns provider subprocess, captures output with marker detection
   - After provider signals `<ralph>DONE</ralph>`, CLI runs verification commands + browser checks (console errors are hard failures) + service health checks
   - Pass -> mark story complete; Fail -> retry (up to maxRetries, then block)
   - When all stories pass -> final verification (verify commands + browser verification for all UI stories + service health) -> provider reviews -> `VERIFIED` or `RESET`
   - Learnings are saved on every path (including timeout/error)
4. `ralph verify <feature>` runs verification only (same readiness gates, starts services for browser verification)

## Responsibility Split: CLI vs AI Provider

This is the most important architectural decision. In the original v1, the AI agent did almost everything (picked stories, ran tests, updated prd.json, committed). In v2, the CLI is the orchestrator and the AI is a pure implementer.

### CLI handles (the provider must NOT do these):
- **Readiness enforcement**: Hard-fails before run/verify if QA commands are missing, placeholder, or not in PATH
- **Story selection**: CLI picks the next story by priority, skipping passed/blocked
- **Branch management**: CLI creates/switches to the `ralph/<feature>` branch
- **State updates**: CLI marks stories passed/failed/blocked based on verification results
- **Verification**: CLI runs `verify.default` and `verify.ui` commands, not the provider
- **Browser testing**: CLI executes browserSteps via rod; console errors are hard failures
- **Service management**: CLI starts/stops/restarts dev servers; captures output; checks health during verification
- **Learning management**: CLI deduplicates learnings and saves them on every code path (including timeout/error)
- **Crash recovery**: CLI tracks `currentStoryId` in prd.json to resume interrupted work
- **Concurrency control**: CLI uses lock file to prevent parallel runs
- **PRD persistence**: CLI commits prd.json changes atomically (including STUCK/timeout/no-DONE paths)

### Provider handles (told via prompts in prompts/run.md):
- **Code implementation**: Write the code for the assigned story
- **Writing tests**: Create tests for the implementation
- **Local checks**: Run linters/tests before committing (as a sanity check)
- **Git commits**: Commit implementation with `feat: US-XXX - Title` format
- **Signal markers**: Output `<ralph>DONE</ralph>`, `<ralph>STUCK</ralph>`, etc.
- **Knowledge updates**: Update AGENTS.md/CLAUDE.md with discovered patterns
- **Learnings**: Output `<ralph>LEARNING:...</ralph>` for cross-iteration memory
- **Documentation verification**: Use btca to check implementations against current docs (when available)

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

For interactive PRD sessions (`ralph prd`), `stdin` prompt mode is automatically overridden to `arg` since the provider needs stdin for interactive input.

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
    "command": "amp"
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
    "ui": ["bun run test:e2e"]
  },
  "browser": {
    "enabled": true,
    "headless": true,
    "executablePath": "/usr/bin/chromium",
    "screenshotDir": ".ralph/screenshots"
  },
  "commits": {
    "prdChanges": true,
    "message": "chore: update prd.json"
  }
}
```

`promptMode`, `promptFlag`, `args`, and `knowledgeFile` are auto-detected from `provider.command` if not set. Only `command` is required.

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
- **Marker detection**: `processLine()` in loop.go scans each line of provider stdout and stderr with regex. Markers are detected during execution, not after.
- **Provider subprocess**: Spawned via `os/exec` with context timeout. Three modes for prompt delivery (stdin pipe, arg append, temp file).
- **Lock file**: JSON with pid/startedAt/feature/branch. Stale detection via `kill(pid, 0)`. Atomic creation via `O_CREATE|O_EXCL`.
- **Feature directories**: `.ralph/YYYY-MM-DD-feature/` or `.ralph/YYYYMMDD-feature/` format. `FindFeatureDir` finds most recent match by suffix.
- **Browser steps**: Defined in prd.json per story. Executed by rod in headless Chrome. Screenshots saved to `.ralph/screenshots/`.
- **Console error enforcement**: Browser console errors are hard verification failures (not warnings). Checked in both per-story and final verification.
- **Default branch detection**: `DefaultBranch()` tries `origin/HEAD` symbolic ref first, then falls back to checking if `main` or `master` branch exists locally.
- **Process group killing**: Services are started with `Setpgid: true` so `syscall.Kill(-pid, SIGTERM)` kills the entire process group (including child processes).
- **Service ready checks**: HTTP GET polling every 500ms until status < 500, with configurable timeout.
- **Service output capture**: `capturedOutput` trim buffer (keeps last ~50% on overflow) captures service stdout/stderr via `io.MultiWriter`. Available via `GetRecentOutput()` for diagnostics.
- **Service health checks**: `CheckServiceHealth()` polls service ready URLs during verification to detect crashed/unresponsive services.
- **Readiness gates**: `CheckReadiness()` in config.go validates QA command binaries exist in PATH (`extractBaseCommand()` + `exec.LookPath`) before allowing `ralph run` or `ralph verify`. Covers `verify.default`, `verify.ui`, and service `start` commands. Placeholder echo commands are detected separately.
- **Learning deduplication**: `AddLearning()` normalizes (case-insensitive, trimmed) before checking for duplicates. Learnings are saved on all code paths: success, timeout, error, STUCK, and no-DONE. Prompt delivery caps at 50 most recent learnings to prevent context overflow.
- **PRD quality warnings**: `WarnPRDQuality()` checks stories for missing "Typecheck passes" acceptance criteria (soft warning, does not block).
- **Prompt templates**: Embedded via `//go:embed prompts/*`. Simple `{{var}}` string replacement (not Go templates).
- **Update check**: Background goroutine with 5s timeout, cached to `~/.config/ralph/update-check.json` for 24h. Non-blocking: skipped silently if check hasn't finished by CLI exit. Disabled for `dev` builds and `ralph upgrade`.
- **btca detection**: `CheckBtcaAvailable()` checks if btca is in PATH. Prompts always include a documentation verification section: btca instructions when available, web search fallback when not. `CheckReadinessWarnings()` returns soft warnings (btca missing) that don't block execution.
- **Verification summary**: `runFinalVerification` accumulates structured PASS/FAIL lines for each verification step and passes them to the verify prompt as `verifySummary`.
- **Git diff in verify prompt**: `GetDiffSummary()` provides `git diff --stat` output from the default branch to HEAD, injected into the verify prompt as `diffSummary`.

## Prompt Template Variables

Each prompt template uses `{{var}}` placeholders replaced by `prompts.go`:

| Template | Variables |
|----------|-----------|
| `run.md` | `storyId`, `storyTitle`, `storyDescription`, `acceptanceCriteria`, `tags`, `retryInfo`, `verifyCommands`, `learnings`, `knowledgeFile`, `project`, `description`, `branchName`, `progress`, `storyMap`, `browserSteps`, `serviceURLs`, `timeout`, `btcaInstructions` |
| `verify.md` | `project`, `description`, `storySummaries`, `verifyCommands`, `learnings`, `knowledgeFile`, `prdPath`, `branchName`, `serviceURLs`, `diffSummary`, `btcaInstructions`, `verifySummary` |
| `prd-create.md` | `feature`, `outputPath` |
| `prd-refine.md` | `feature`, `prdContent`, `outputPath` |
| `prd-finalize.md` | `feature`, `prdContent`, `outputPath` |

## Non-obvious Behaviors

- **`AllStoriesComplete` treats blocked as complete**: Blocked stories do not prevent the "all stories complete" state — they are skipped and the loop proceeds to final verification.
- **`runProvider` returns nil error on non-zero exit**: A provider exiting with code != 0 is not treated as an error — it returns `(result, nil)`. The loop checks markers in the result to determine outcome.
- **Verification commands run through `sh -c`**: All verification commands are wrapped in `sh -c`, so shell features (pipes, redirects, etc.) are available.
- **`cmdVerify` acquires the lock**: Not just `cmdRun` — verification also acquires the lock file to prevent concurrent operations.
- **Only one REASON marker is kept**: If multiple `<ralph>REASON:...</ralph>` markers are emitted, each overwrites the previous. Only the last one survives.
- **SUGGEST_NEXT is advisory only**: The marker is captured but `GetNextStory` selects purely by priority. The suggestion is not currently acted upon.
- **Verification output is captured for retries**: When verification fails, the last 50 lines of command output are stored in `story.Notes` so the retry agent can see what specifically failed.
- **Final VERIFIED is gated on all verification**: If any `verify.default`, `verify.ui` command, browser step, or service health check failed during final verification, the CLI overrides a provider's `VERIFIED` marker and returns not-verified. The provider cannot skip past failing checks.

## Testing

Tests are in `*_test.go` files alongside source. Key test files:
- `config_test.go` - config loading, validation, provider defaults, readiness checks, command validation, btca availability
- `schema_test.go` - PRD validation, story state transitions, browser steps, learning deduplication, PRD quality
- `services_test.go` - output capture, service manager, health checks
- `prompts_test.go` - prompt generation, variable substitution, provider-agnosticism
- `loop_test.go` - marker detection, provider result parsing
- `feature_test.go` - directory parsing, timestamp formats
- `lock_test.go` - lock acquisition, stale detection
- `browser_test.go` - runner init, screenshot saving
- `atomic_test.go` - atomic writes, JSON validation
- `git_test.go` - git operations (branch, commit, checkout, EnsureBranch, DefaultBranch, GetDiffSummary)
- `update_check_test.go` - update check cache path

Run with `go test ./...` or `go test -v ./...` for verbose output.

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

- **Add a new provider**: Add entry to `knownProviders` map in config.go, no other changes needed
- **Add a new browser action**: Add case to `executeStep()` switch in browser.go
- **Add a new marker**: Add regex pattern and field to `ProviderResult` in loop.go, handle in `processLine()`
- **Add a new command**: Add case to `main()` switch in main.go, implement handler in commands.go
- **Modify prompt**: Edit the `.md` file in `prompts/` directory (embedded at compile time)
- **Change PRD schema**: Update structs in schema.go, bump schemaVersion, update validation
