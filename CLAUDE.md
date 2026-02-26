# CLAUDE.md

## Maintenance Rule

Do not bloat this file. Only add non-obvious invariants and gotchas that cannot be inferred by reading the code. If the code explains it, it doesn't belong here. Prefer reading source files over expanding this document.

## Project Overview

Ralph v2 is a Go CLI that orchestrates AI coding agents to autonomously implement software features defined in a PRD. It spawns fresh AI instances per user story, verifies each with automated tests, persists learnings, and repeats until all stories pass. Complete rewrite of [snarktank/ralph](https://github.com/snarktank/ralph).

## Build and Test

```bash
make build    # go build -ldflags="-s -w" -o ralph .
make test     # go test ./...
make test-e2e # go test -tags e2e -timeout 60m -v -run TestE2E ./...
make release  # triggers GitHub Actions release workflow via gh CLI
```

Go 1.25.6. Key dependency: `github.com/creativeprojects/go-selfupdate`. Version injected at build via `-ldflags -X main.version` (defaults to `"dev"`). Prompt templates live in `prompts/` and are embedded at compile time via `//go:embed prompts/*`.

## Responsibility Split: CLI vs AI Provider

The CLI is the orchestrator; the AI provider is a pure code implementer. This is the most important architectural decision.

### CLI handles (provider must NOT do these):
- Story selection, branch management, state updates, verification
- Service management (start/stop/restart), concurrency control (lock file)
- Learning management (dedup + save on all code paths including timeout/error)
- Resource consultation (subagents → guidance injected into prompts)
- PRD persistence (commits prd.md, prd.json, run-state.json)
- Readiness gates (git repo, `sh`, `.ralph/` writable, commands in PATH)

### Provider handles (via prompts/run.md):
- Code implementation + tests + git commits (`feat: US-XXX - Title`)
- Signal markers: `<ralph>DONE</ralph>`, `<ralph>STUCK:reason</ralph>`, `<ralph>LEARNING:text</ralph>`

### Provider auto-detection (from `provider.command`):
| Provider | promptMode | promptFlag | defaultArgs | knowledgeFile |
|----------|-----------|------------|-------------|---------------|
| `amp` | stdin | | `--dangerously-allow-all` | AGENTS.md |
| `claude` | stdin | | `--print --dangerously-skip-permissions` | CLAUDE.md |
| `opencode` | arg | | `run` | AGENTS.md |
| `aider` | arg | `--message` | `--yes-always` | AGENTS.md |
| `codex` | arg | | `exec --full-auto` | AGENTS.md |
| other | stdin | | | AGENTS.md |

Prompt delivery modes: `stdin` (pipe), `arg` (CLI argument), `file` (temp file). `defaultArgs` apply only when `args` key absent from config.

## Critical Invariants & Gotchas

Non-obvious behaviors that cause bugs if misunderstood:

**State management:**
- All state file writes MUST use `AtomicWriteJSON` (temp → validate → rename). Never write directly.
- prd.json is immutable during runs. run-state.json is separate CLI-managed state. Functions take `(*PRDDefinition, *RunState)` pair.
- `AllComplete` treats skipped as complete — loop exits when every story is passed OR skipped.

**Provider subprocess:**
- `runProvider` returns `(result, nil)` on non-zero exit — markers determine outcome, not exit code. `(nil, err)` means provider failed to start.
- Marker detection is **whole-line** matching (not substring) to prevent spoofing. DONE uses `==`; STUCK/LEARNING use anchored regexes.
- Provider MUST commit to pass — no new commit after DONE = automatic retry (pre-run hash captured AFTER PRD commit to avoid false positives).
- Process groups: run loop uses `Setpgid: true` for group killing. Interactive sessions (prd.go) must NOT set `Setpgid` — provider needs foreground group for terminal stdin, or gets SIGTTIN.
- `ralph run` suppresses provider output from console — streams to JSONL logs only. Use `ralph logs -f` to follow live.
- Scanner buffer overflow at 1MB lines; warnings logged.

**Verification:**
- Commands run through `sh -c` with `Setpgid: true` + process group killing on timeout.
- AI deep verification (`runVerifySubagent`) runs only in `ralph verify`, NOT during `ralph run` per-story verification.
- `cmdVerify` acquires the lock (not just `cmdRun`).
- Verify-at-top only runs for stories where `state.IsAttempted(story.ID)` is true — prevents false positives from vacuous test passes on unattempted stories.
- Last 50 lines of failed command output stored in `lastFailure` for retry agent.

**Interactive sessions:**
- `stripNonInteractiveArgs()` removes `--print`/`-p`; `stdin` mode overridden to `arg`.
- `ralph refine` doesn't acquire the lock (interactive, user is present).

**Branch & git:**
- `EnsureBranch` refuses to switch to existing branch with dirty tree (error). New branch with dirty tree is allowed. Already on the right branch skips the check.
- `DefaultBranch()` fallback chain: `origin/HEAD` → local `main`/`master` → remote `origin/main`/`origin/master`. Diff functions fall back three-dot → two-dot when merge-base unavailable.
- Feature names match case-insensitively (`strings.EqualFold`).
- Feature directories use `.ralph/YYYY-MM-DD-feature/` format. `FindFeatureDir` finds most recent match by suffix.

**Resources & consultation:**
- Consultation is always-on when cached resources exist. Falls back to web search instructions when none cached.
- Consultation results must contain `Source:` lines or are treated as failed (likely hallucinated), falling back to file path.
- Version-specific cache paths (`name@version`). Versioned repos skip sync (immutable). `DetectDefaultBranch()` via `git ls-remote --symref` avoids assuming `main`.

**Miscellaneous:**
- Services required — `validateConfig()` enforces ≥1 entry. `ready` URL must have `http(s)://` scheme.
- Learning cap: 50 most recent in prompts. Previous work context comes from `.ralph/summary.md` (project-level, injected into `generateRunPrompt` via `{{previousWork}}`).
- **Archive flow**: After `ralph verify` succeeds, user can archive the feature → AI generates summary → appended to `.ralph/summary.md` → prd.md/prd.json/run-state.json deleted → committed. Summary written BEFORE files deleted (fail-safe).
- `extractSummary` returns `false` for empty content between markers (same pattern as guidance markers).
- `promptYesNo`/`promptChoice` return false/empty on EOF (prevents infinite loops in non-interactive contexts).
- `CleanupCoordinator` handles SIGINT/SIGTERM: kills provider groups, stops services, releases locks before `os.Exit(130)`. Ensures cleanup when defers are bypassed.
- Prompt templates use `{{var}}` string replacement (not Go templates).

## Common Development Tasks

- **Add provider**: `knownProviders` in config.go + `providerChoices` in commands.go (alphabetical, enforced by test)
- **Add marker**: Regex + field in `ProviderResult` (loop.go), handle in `processLine()`
- **Add command**: Case in `main()` (main.go), handler in commands.go
- **Modify prompt**: Edit `prompts/*.md` (embedded at compile time)
- **Change PRD schema**: Types in schema.go, bump schemaVersion, update `prd-finalize.md`

## Post-Task Checklist

After changes that modify behavior, features, or interfaces:

### Documentation sync
1. **CLAUDE.md** — Update only if your change introduces a non-obvious invariant or changes a common dev task. Do not add information discoverable from code.
2. **README.md** — Update if user-facing commands, config, or protocol changed.
3. **Prompt templates** (`prompts/*.md`) — Update if CLI↔provider interaction changed.

### Test sync
1. Every new exported function/method must have at least one test.
2. Changed behavior must have updated tests (not just passing — testing the new behavior).
3. Run `go test ./...` and `go vet ./...` to confirm.
