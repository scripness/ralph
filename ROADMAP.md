# Roadmap: Ralph → Scrip

**Last updated:** 2026-03-10

This document is the single source of truth for Ralph's evolution into scrip. Every item here is concrete and implementable — vague aspirations have been cut. Items are organized by priority based on convergent evidence from multiple analyses.

**Research sources distilled into this document:**
- Ralph codebase audit (20K LOC, 47 source files)
- Symphony SPEC.md pattern extraction (16 reusable patterns evaluated)
- Symphony comparative analysis (daemon orchestrator, Codex-only)
- Stripe Minions analysis (shift-left verification, structured retry)
- Tidewave analysis (MCP runtime intelligence, service log surfacing)
- T3 Code analysis (worktree isolation, visual diff review)
- JetBrains Air analysis (ACP protocol, execution isolation tiers)
- ROADMAP accuracy improvement research (32 verified code claims)
- Security audit system design (4-layer architecture)

---

## Table of Contents

- [Phase 1: Accuracy](#phase-1-accuracy)
- [Phase 2: Security](#phase-2-security)
- [Phase 3: Extensibility & Integration](#phase-3-extensibility--integration)
- [Phase 4: Quality of Life](#phase-4-quality-of-life)
- [Deferred](#deferred)

---

## Phase 1: Accuracy

Three independent analyses (Symphony failure classification, Stripe Minions structured feedback, ROADMAP accuracy gaps) converge on the same conclusion: Ralph's retry loop works but wastes attempts because providers get insufficient context about *why* the previous attempt failed and *what* the current codebase state is. These improvements target convergence speed — fewer retries to reach passing.

**Estimated total: ~600 LOC new, ~200 LOC modified**

### 1.1 Structured Retry Context with Failure Classification

**Sources:** ROADMAP accuracy Phase 1, Symphony SPEC Section 14.1, Minions structured feedback
**Priority:** Highest — directly improves retry success rate

Ralph currently passes `lastFailure` (last 50 lines of failed command output) and `retryInfo` ("Attempt 2 of 3") on retries. Providers don't know whether the failure was a compile error, test failure, lint violation, or timeout — and can't see what code changed in the failed attempt.

**Implementation:**

Add to `schema.go`:
```go
// RunState additions
LastDiff        map[string]string  // story ID → git diff from failed attempt
LastTestOutput  map[string]string  // story ID → last test output (truncated)
FailureClass    map[string]string  // story ID → classified failure type
```

Add to `loop.go` (~80 LOC):
- `classifyFailure(output string) string` — returns one of: `"compile"`, `"test"`, `"lint"`, `"timeout"`, `"service"`, `"no-commit"`, `"stuck"`, `"unknown"`
- After failed verification: capture `git diff` from pre-run commit hash
- Store classification + diff + test output in RunState

Add to `prompts.go` (~40 LOC):
- New template variables: `{{retryDiff}}`, `{{retryTestOutput}}`, `{{retryClassification}}`
- Populate from RunState on retry iterations

Update `prompts/run.md` (~20 LOC):
- Add structured retry section: "This is attempt {{attempt}} of {{maxRetries}}. Previous attempt failed with: {{retryClassification}}. Changes from failed attempt: {{retryDiff}}. Error output: {{retryTestOutput}}"

**~170 LOC total.** Test: `TestClassifyFailure` (classification logic), `TestRunPrompt_RetryContext` (template renders correctly).

### 1.2 Pre-Flight Baseline Capture

**Sources:** Minions shift-left verification, ROADMAP accuracy Phase 3.2

Providers currently don't know the baseline state. If 42 tests pass before implementation, the provider should know that — and know which tests are relevant to their story. After DONE, comparing against baseline detects regressions the provider introduced.

**Implementation:**

Add to `schema.go` (~10 LOC):
```go
BaselineFailures []string  // verification failures captured before first story attempt
```

Add to `loop.go` (~50 LOC):
- Before main story loop (after services start): if `state.BaselineFailures == nil`, run verification commands with no story context
- Store any pre-existing failures in `BaselineFailures`
- On retry, include baseline in prompt: "These failures existed before your attempt — do not fix them"

Update `prompts/run.md` (~10 LOC):
- Add `{{baselineInfo}}` section when baseline failures exist

**~70 LOC total.** Test: `TestBaselineCapture` (pre-existing failures stored), `TestBaselineExcludedFromRetry` (baseline failures not blamed on provider).

### 1.3 Collect All Verification Failures

**Sources:** Minions parallel feedback, ROADMAP accuracy Phase 3.1

`runStoryVerification` currently returns on first failure. Provider only sees "test failed" but not that lint also failed. Full error picture enables targeted fixes.

**Implementation:**

Modify `loop.go` (~50 LOC):
- Change `StoryVerifyResult` to collect `failures []string` instead of single `reason string`
- Remove early returns in verification loop — run all commands, collect all results
- Format all failures into `lastFailure` for retry prompt

**~50 LOC modified.** Test: `TestStoryVerification_CollectsAllFailures`.

### 1.4 Service Log Injection

**Sources:** Tidewave `get_logs` pattern, ROADMAP accuracy Phase 2

Ralph's `ServiceManager.GetRecentOutput()` already captures service output (256KB buffer). This data is never shown to the AI. When a service crashes or logs errors during implementation, the provider should know.

**Implementation:**

Add to `prompts.go` (~30 LOC):
- `buildServiceLogs(svcMgr, services) string` — extract ERROR/PANIC/WARN lines from service output

Add `{{serviceLogs}}` variable to `prompts/run.md`, `prompts/verify-fix.md`, `prompts/verify-analyze.md` (~20 LOC each, ~50 LOC total plumbing in prompts.go)

**~80 LOC total.** Test: `TestBuildServiceLogs` (filters errors from output).

### 1.5 Service Health Check Before Provider Spawn

**Sources:** ROADMAP accuracy Phase 2.1

Services can crash between story iterations. Provider spawns into a broken environment, wastes an attempt on "connection refused" errors, gets classified as test failure.

**Implementation:**

Add to `services.go` (~20 LOC):
- `ServiceManager.EnsureHealthy()` — check ready URLs for all started services, restart any that are down

Add to `loop.go` (~20 LOC):
- Call `EnsureHealthy()` before `runProvider()` (line ~269)

**~40 LOC total.** Test: `TestEnsureHealthy_RestartsDown`.

### 1.6 Procedural Recipes in Prompts

**Sources:** Symphony SKILL.md pattern (12-step commit procedure vs Ralph's 4-line instruction)

Provider improvisation on mechanical tasks (staging, committing, test execution order) causes avoidable retries. Explicit step-by-step procedures reduce variance.

**Implementation:**

Update `prompts/run.md` (~50 lines):
- Explicit staging procedure: verify scope before `git add`, avoid `git add .`
- Verification sequence: run all verify commands, confirm pass, then signal DONE
- Test writing patterns: match existing test framework and naming conventions
- Commit message format enforcement

**0 Go code, ~50 lines of prompt text.** No test needed (behavioral, measured via retry rates).

### 1.7 Two-Timer Timeout Model

**Sources:** Symphony SPEC Section 5.2.3/10.4

Ralph has a single `provider.timeout` (30min hard limit). No detection for alive-but-idle providers (stuck in reasoning loop, waiting for input). A provider could produce no output for 29 minutes and only get killed at the hard limit.

**Implementation:**

Add to `config.go` (~10 LOC):
- `provider.stallTimeout` field (default 300s / 5min)

Add to `loop.go` (~40 LOC):
- Track `lastOutputTime` in `processLine()`
- Second timer: if `time.Since(lastOutputTime) > stallTimeout`, kill provider process group
- Log stall event with duration

**~50 LOC total.** Test: `TestStallDetection` (mock provider with no output triggers kill).

---

## Phase 2: Security

AI-generated code has disproportionately high rates of logic-level security flaws (missing authorization, workflow manipulation) rather than pattern-level bugs. Ralph currently has near-zero security coverage: two bullet points in `verify-analyze.md`, no SAST detection, no security metadata in PRD schema.

This is a layered design — each layer adds value independently.

**Estimated total: ~1,400 LOC new, ~200 LOC modified**

### 2.1 Layer 1: Security Awareness in PRD

Add conditional security questions to `prd-create.md` that trigger for features involving user data, authentication, external APIs, or financial transactions. Add `SecurityTags []string` field to `StoryDefinition` in `schema.go`.

`prd-finalize.md` validates that security-tagged stories have specific, testable security criteria (not just "be secure" — must be "Invalid credentials return 401", "Passwords stored as bcrypt hash").

**~100 LOC** (schema field, prompt additions, validation).

### 2.2 Layer 2: Security Guidance in Implementation

Add `{{securityFocus}}` template variable to `prompts/run.md`. For stories with SecurityTags, populate with framework-aware security patterns (CSRF middleware for Phoenix, parameterized queries for Ecto, bcrypt for password hashing).

Extend `consult.md` to search cached framework source for security APIs, auth patterns, input validation utilities.

**~200 LOC** (prompts.go population logic, consultation security search, template additions).

### 2.3 Layer 3: Security in Verification

Extend `discovery.go` to detect SAST tools: Semgrep (cross-language), gosec (Go), Sobelow (Elixir), Bandit (Python). Pipe SAST output to AI for triage (filter false positives).

Replace vague "are there security issues?" in `verify-analyze.md` with tag-specific checklists:
- **auth** stories: session management, password hashing, rate limiting, CSRF
- **data-access** stories: parameterized queries, authorization checks, IDOR prevention
- **api** stories: input validation, authentication, rate limiting, error information leakage
- **input-validation** stories: XSS prevention, injection, file upload restrictions

**~240 LOC** (discovery.go extension, verify-analyze.md checklists).

### 2.4 Layer 4: `scrip secure` Command

Standalone deep security audit with 4 components:

| Component | Duration | What It Does |
|-----------|----------|-------------|
| SAST baseline | ~2min | Run detected SAST tools, capture structured output |
| Dependency audit | ~1min | `mix deps.audit` / `npm audit` / `cargo audit` for known CVEs |
| AI deep analysis | ~15min | Architecture-level review: auth flow consistency, trust boundaries, middleware gaps |
| AI triage | ~5min | Review SAST results for false positives, prioritize real issues |

For Elixir/Phoenix specifically: Sobelow (`mix sobelow --format json`), `mix deps.audit`, `mix hex.audit`.

Output: `SecurityReport` type with findings, severity, recommendations. Formatted for console and optionally JSON.

**~800 LOC** (secure.go command handler, prompts/secure-analyze.md, SecurityReport type, discovery.go SAST detection).

### 2.5 Lightweight Security in Verify

Integration point: run SAST + dependency audit as part of normal `ralph verify` (always-on, fast). Deep AI audit remains `scrip secure` only (on-demand, slow).

**~60 LOC** (wire SAST commands into verify pipeline).

---

## Phase 3: Extensibility & Integration

These features make Ralph usable in more contexts without changing the core orchestration model.

**Estimated total: ~900 LOC new, ~150 LOC modified**

### 3.1 Lifecycle Hooks

**Sources:** Symphony SPEC Section 5.3.4/9.4

Ralph hardcodes all lifecycle operations in Go. Users can't inject dependency installation, DB migrations, cache warming, or cleanup between stories. This blocks real projects that need pre-flight setup.

**Implementation:**

Config addition:
```json
{
  "hooks": {
    "beforeStory": "npm ci && prisma migrate deploy",
    "afterStory": "rm -rf .next/.cache",
    "beforeVerify": "mix deps.compile",
    "timeout": 60
  }
}
```

New `hooks.go` (~80 LOC):
- `RunHook(name, command, timeout, failureMode)` — shell exec with timeout, process group kill, error classification
- `failureMode`: `"fatal"` (abort story attempt) or `"warn"` (log and continue)

Config extension (~40 LOC):
- `HooksConfig` struct with optional string fields + timeout

Loop integration (~40 LOC):
- 4 call sites: before provider spawn, after provider exit, before verification, before archive

**~160 LOC total.** Test: `TestRunHook_Timeout`, `TestRunHook_FatalFailure`, `TestRunHook_WarnContinues`.

### 3.2 MCP Server Management

**Sources:** Tidewave MCP architecture, ROADMAP accuracy Phase 4.1

Providers that support MCP (Claude Code, Codex) can connect to MCP servers for runtime intelligence (Tidewave, custom tools). Ralph should manage MCP configuration so users don't manually configure each provider.

**Implementation:**

Config addition:
```json
{
  "mcp": {
    "tidewave": {
      "url": "http://localhost:4000/tidewave/mcp"
    }
  }
}
```

New logic in services/mcp area (~150 LOC):
- `MCPConfig` and `MCPServerConfig` types in config.go
- `generateMCPJson(projectRoot, mcpCfg)` — write `.mcp.json` for Claude Code auto-discovery
- `cleanupMCPJson(projectRoot)` — remove after provider exits
- Add to `CleanupCoordinator` for crash cleanup

Integration (~50 LOC):
- After services ready + before provider spawn: write `.mcp.json`
- After provider exit: cleanup `.mcp.json`

**~200 LOC total.** Test: `TestGenerateMCPJson`, `TestCleanupMCPJson`, `TestMCPConfig_Validation`.

### 3.3 Linear Push-Only Integration

**Sources:** SCRIP-PLAN integration analysis

Lightweight dashboard sync — scrip pushes story status to Linear for visibility. Linear is a viewport, not the orchestrator. PRD remains source of truth.

**Implementation:**

New `linear.go` (~200 LOC):
- GraphQL client with `IssueCreate` and `IssueUpdate` mutations
- After `scrip prd` finalize: create Linear issues for each story
- After each story pass/skip in `scrip run`: update Linear issue status
- After `scrip verify` success: close all Linear issues

Config addition:
```json
{
  "linear": {
    "apiKey": "$LINEAR_API_KEY",
    "teamId": "TEAM-ID"
  }
}
```

Constraints:
- Push-only (no polling, no daemon, no state sync)
- API key from env var expansion (never stored in config)
- All failures logged and ignored (informational, not critical path)

**~250 LOC total.** Test: `TestLinearClient_IssueCreate`, `TestLinearClient_EnvVarExpansion`, `TestLinearClient_FailureIgnored`.

### 3.4 Machine-Readable Status

**Sources:** Symphony observability Section 13

`ralph status` is human-readable only. Machine-readable output enables CI/CD integration, Slack notifications, dashboards.

**Implementation:**

Add `--json` flag to `ralph status` (~40 LOC):
```json
{
  "feature": "user-auth",
  "branch": "ralph/user-auth",
  "status": "in-progress",
  "stories": {"passed": 3, "skipped": 1, "pending": 2},
  "retries": {"US-001": 0, "US-002": 1},
  "learnings": 5
}
```

**~40 LOC.** Test: `TestStatusJSON_Format`.

---

## Phase 4: Quality of Life

Smaller improvements that reduce friction. Each is independently implementable.

**Estimated total: ~300 LOC**

### 4.1 Dependency Enforcement at Runtime

`StoryDefinition` supports `depends_on` field but `GetNextStory()` ignores it. Stories blocked on unmet dependencies waste attempts.

Add dependency check in `GetNextStory()`: skip stories whose `depends_on` IDs are not all in `state.Passed`. Log when a story is blocked. (~30 LOC in story selection logic)

### 4.2 Enriched Refine Summaries

Replace `git diff --stat` with truncated full `git diff` (capped at ~8000 chars) in post-refine summarization input. Gives summarizer actual code changes, not just file names. (~10 LOC in refine.go)

### 4.3 Post-Session Notes Capture

After interactive refine session, prompt user for notes. If provided with zero commits, write notes directly to summary.md. If with commits, include in summarizer context. Captures decisions and rationale from interactive work. (~40 LOC in commands.go + refine.go)

### 4.4 Prompt Polish

- `prompts/run.md`: Add "Do NOT signal DONE if any verification command fails" (prevents false-positive DONE markers)
- `prompts/verify-analyze.md`: Add `{{changedFiles}}` variable (from `git diff --name-only`) so AI analyzes only relevant files
- `prompts/run.md`: Add "Capture the why" sentence in LEARNING instructions — emphasize reasoning over facts

(~40 LOC in prompts.go + template edits)

### 4.5 Strict Template Rendering

Ralph's `{{var}}` replacement silently produces empty strings for undefined variables. A silent consultation failure results in empty `{{resourceGuidance}}` with no error. Add post-render check for remaining `{{...}}` patterns — fail with clear error. (~30 LOC in prompts.go)

### 4.6 Workspace Path Validation

Validate feature directory paths are under `.scrip/`. Prevent path traversal via `../` in feature names. Check in `CreateFeatureDir()` and before provider launch. (~20 LOC in feature.go)

### 4.7 Stale Feature Directory Cleanup

On `scrip run` start, scan `.scrip/` for feature dirs with no summary.md, no active run-state.json, and older than 30 days. Log warnings (don't auto-delete). (~40 LOC in startup path)

### 4.8 Verification Report Artifacts

After `runVerifyChecks()`, write structured JSON to `featureDir/verify-report.json`:
```json
{
  "timestamp": "2026-03-05T14:30:00Z",
  "feature": "user-auth",
  "passed": true,
  "checks": [
    {"name": "typecheck", "passed": true, "duration_ms": 2300},
    {"name": "test", "passed": true, "duration_ms": 5100}
  ]
}
```

Enables CI/CD integration and audit trails. (~80 LOC — mostly serialization, `VerifyReport` type already exists)

---

## Deferred

Items evaluated and explicitly deferred — not forgotten, but lacking evidence of current need or requiring architectural changes beyond current scope.

| Item | Reason Deferred |
|------|----------------|
| **Dynamic config reload** | No pain point — `scrip run` exits between features. Reload only matters for daemon mode, which scrip doesn't have. |
| **Reconciliation (manual code detection)** | Verify-at-top already catches regressions. Detecting "someone else touched the code" adds complexity for a solo-developer tool. |
| **Non-code task model** | No concrete schema or workflow designed. "Deployment as a task" needs its own design session with real use cases. |
| **Token/cost tracking** | Requires provider cooperation (many don't emit usage stats). Implement when provider output parsing is standardized. |
| **Worktree isolation** | Architectural change enabling parallel execution. Ralph is sequential by design. Defer until parallelism is designed. |
| **ACP protocol** | Ralph's marker protocol (DONE/STUCK/LEARNING) works with any CLI agent. ACP adds JSON-RPC complexity for marginal gain in unattended context. |
| **Plan-then-build separation** | Vague — no clear output schema, trigger condition, or re-planning flow. Needs design session. |
| **AMEND markers** | Overlaps with structured retry context. Unclear how amendments persist or trigger re-planning. Needs design session. |
| **Session IDs** | JSONL events already include story ID and iteration count. UUID per invocation is nice-to-have for debugging but not blocking. |
| **Exponential backoff retries** | Immediate retry works for logic errors (most common failure). No evidence Ralph hits rate limits in practice. |

---

## Rename: Ralph → Scrip

When the above features are ready to ship as a cohesive release, rename:
- Binary: `ralph` → `scrip`
- Config: `ralph.config.json` → `scrip.config.json`
- Feature dir: `.ralph/` → `.scrip/`
- Schema URL: update `$schema` field
- Lock file: `.ralph/ralph.lock` → `.scrip/scrip.lock`
- All internal references, prompts, error messages

**~250 LOC modified** (find/replace across codebase). Include migration logic to detect and rename `.ralph/` → `.scrip/` for existing projects.

---

## Total Effort Summary

| Phase | New LOC | Modified LOC | Items |
|-------|---------|-------------|-------|
| **1: Accuracy** | ~460 | ~140 | 7 items |
| **2: Security** | ~1,400 | ~200 | 5 items |
| **3: Extensibility** | ~650 | ~150 | 4 items |
| **4: Quality of Life** | ~250 | ~50 | 8 items |
| **Rename** | — | ~250 | 1 item |
| **Total** | **~2,760** | **~790** | **25 items** |

Phase 1 should be implemented first — it directly improves the core value proposition (accuracy and convergence speed) with the lowest risk and highest ROI per line of code.
