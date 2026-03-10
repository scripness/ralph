# Ralph Roadmap

This document collects research, analysis, and design proposals for future Ralph improvements. Each section is a self-contained investigation into a specific capability area — including evidence gathered, trade-offs evaluated, and concrete implementation plans.

Sections are appended as new ideas are explored. They represent vetted proposals, not commitments — each can be implemented independently and in any order unless noted otherwise.

---

## Table of Contents

- [Security Audit System](#security-audit-system)
- [Synthesis: Ralph vs the Ralph Ecosystem](#synthesis-ralph-vs-the-ralph-ecosystem)
- [Refine Session Summary: Capturing Non-Code Progress](#refine-session-summary-capturing-non-code-progress)
- [Accuracy Improvement: Data Flow Gaps](#accuracy-improvement-data-flow-gaps)

---

## Security Audit System

**Status:** Proposed
**Date:** 2026-03-10
**Motivation:** Ralph currently has near-zero security coverage for AI-generated code. The verification prompt (`verify-analyze.md`) contains two bullet points about security. PRD creation has no security questions. Implementation prompts have no security guidance. No SAST tooling is auto-detected or integrated.

### Analysis

#### Current State

| Component | Security Coverage |
|-----------|------------------|
| `prd-create.md` | None. Five clarifying questions, none about data sensitivity, auth, or threat surface. |
| `prd-finalize.md` | None. Validates story sizing and acceptance criteria verifiability, not security completeness. |
| `run.md` | None. The implementing agent receives zero security instructions. |
| `verify-analyze.md` | Two mentions: "Are there security issues (XSS, injection, auth bypass)?" and "Security vulnerabilities ARE failures." No structured checklist. |
| `consult.md` | None explicit. Fallback guidance has one line: "Check security patterns (input validation, auth, etc.) are up to date." |
| `discovery.go` | Auto-detects typecheck, lint, and test commands. No SAST tool detection (gosec, semgrep, bandit, etc.). |
| `schema.go` | No security metadata fields. |

A user can define a feature like "add user login" with acceptance criteria limited to "form submits to /login" and Ralph will implement it without considering password hashing, CSRF, rate limiting, input sanitization, or session security.

#### SAST vs AI-Driven Security: Where Each Excels

Research across current tooling (Semgrep, CodeQL, gosec, Bandit, eslint-plugin-security) and AI-based review reveals they are complementary, not competing:

**SAST tools excel at:**
- Pattern-based vulnerabilities: SQL injection, XSS, hardcoded secrets, weak crypto, unsafe deserialization
- Deterministic, reproducible results across runs
- High precision on known vulnerability patterns
- Fast execution (seconds to minutes)

**AI review excels at:**
- Business logic flaws: auth bypass through logic errors, IDOR, broken access control
- Missing security controls: no rate limiting, no CSRF, no input validation where needed
- Context-dependent issues: how auth state propagates, implicit trust assumptions between components
- Architecture-level patterns: inconsistent middleware application, data crossing trust boundaries unvalidated
- Triaging SAST false positives: understanding whether a flagged pattern is actually exploitable in context

**Neither catches:**
- Runtime behavior, actual cloud configuration vs declared
- Third-party service misconfigurations
- Dependency vulnerabilities in transitive deps (requires SCA tools like `npm audit`, `cargo audit`)
- Performance under adversarial load (DDoS resilience)

**Key finding:** AI-generated code has a disproportionately high rate of logic-level security flaws (missing authorization, workflow manipulation) rather than pattern-level bugs. This means AI review is more important than SAST for Ralph's specific use case, but SAST provides a deterministic baseline that AI review alone cannot guarantee.

**Cross-ecosystem SAST:** Semgrep is the strongest single-tool option — covers Go, JS/TS, Python, Rust, and 30+ other languages from one CLI with community rules. No per-language configuration required. Installation: `pip install semgrep` or Docker. ~12% false positive rate (addressable via AI triage).

#### Separate Command vs Integrated

Real-world patterns from security tooling:

- **Separate command** (Snyk test, Trivy, cargo-audit): Each command is focused, composable in CI/CD. But adoption drops when security is optional — `cargo audit` has ~60% adoption vs `cargo clippy`'s near-100% because clippy is integrated into the standard build.
- **Integrated** (clippy lints in cargo build): Security is non-negotiable, always runs. But couples security depth to the parent command's scope.
- **Tiered** (go test vs go test -race): Same entry point, different depth. Best of both when the base level is fast and the deep level is opt-in.

For Ralph, the risk of a standalone `ralph secure` being forgotten is high. But architecture-level auditing (Dockerfiles, CI/CD, IaC, auth flow consistency) doesn't fit naturally into per-story verification. This argues for both: lightweight security in verify (always-on) and deep audit as a separate command (on-demand).

#### Infrastructure Audit Feasibility

An AI agent with only git repository access can audit declared infrastructure, not deployed infrastructure:

**High-confidence (auditable from repo):**
- Dockerfile: base image currency, running as root, exposed ports, hardcoded secrets in layers
- CI/CD configs: overly permissive permissions (`permissions: write-all`), missing security scanning steps, secrets exposure
- IaC (Terraform/Pulumi): public S3 buckets, open security groups, missing encryption at rest, IAM over-permissions
- K8s manifests: privileged containers, missing resource limits, missing network policies, secrets in manifests
- Auth middleware: consistent application across routes, token validation, session config, CORS
- Database migrations: privilege levels, audit logging, data exposure
- Error handling: information leakage, stack traces in responses, secrets in logs

**Not auditable from repo (must be explicit about limits):**
- Runtime cloud configuration vs declared IaC
- Actual network topology, firewall rules not in code
- Third-party service configurations (Auth0, Stripe, etc.)
- Whether secrets are valid or rotated
- Performance/resilience under load
- Compliance enforcement beyond code (organizational practices)

#### PRD→Implement→Verify Lifecycle Injection

The shift-left vs gate-right tension:

- **Shift-left** (security in PRD + implementation prompts) prevents vulnerabilities from being written. In Ralph's autonomous agent model, this is highly effective because fixing a security flaw means re-running an entire story.
- **Gate-right** (security in verify) catches what prompts missed. Essential because AI agents can miss nuances even with good instructions.
- **The cost asymmetry is clear:** preventing a vulnerability in the PRD/prompt stage costs nothing; catching it in verify costs a full retry cycle.

Anti-patterns to avoid:
- Generic OWASP checklists on every story (alert fatigue — agents become desensitized)
- Treating all SAST warnings as failures (12% false positive rate creates noise)
- Vague criteria like "handles security" (unmeasurable, creates compliance theater)
- Claiming infrastructure coverage that doesn't exist (security theater)

### Synthesis

The design is four layers, each independently useful, each building on the previous:

#### Layer 1: PRD Security Awareness (shift-left)

**`prd-create.md`** — Add one conditional question block. Only applies when the feature involves data, authentication, or API access:

> - What data is involved? (user PII, payment info, internal configs, public data)
> - Who can access it? (authenticated users, public, specific roles)
> - Are there compliance requirements? (GDPR, PCI, HIPAA, SOC2)

**`schema.go`** — Add optional `SecurityTags []string` field to `StoryDefinition`. Valid tags: `auth`, `data-access`, `api`, `input-validation`. Not required — most stories (UI tweaks, refactors, display changes) won't have them.

**`prd-finalize.md`** — If a story has security tags, validate that its acceptance criteria include specific, testable security criteria. Reject vague criteria. Examples of good criteria:

> - "Invalid credentials return 401 (no detail about which field failed)"
> - "User can only view tasks owned by themselves"
> - "Passwords stored as bcrypt hashes"
> - "API rejects requests >1MB with 400"

#### Layer 2: Implementation Guidance (targeted, not bloated)

**`run.md`** — Add `{{securityFocus}}` template variable, populated **only** for stories with security tags. 3-5 bullet points per tag, framework-aware where possible. Stories without security tags get zero additional prompt weight.

Example for an `auth`-tagged story:

> This story involves authentication. Ensure:
> - Token/session validation follows framework patterns (check existing middleware)
> - Invalid credentials return 401 (don't distinguish "user not found" vs "wrong password")
> - Never log passwords, tokens, or sensitive data
> - Session expiry is configured

**`consult.md`** — For security-tagged stories, extend consultation to also search cached framework source for security APIs, auth patterns, and input validation utilities.

#### Layer 3: Structured Verification (gate-right)

**`verify-analyze.md`** — Replace the vague "are there security issues?" with story-type-specific checklists driven by security tags:

| Tag | Verification checklist |
|-----|----------------------|
| `auth` | Token validation on protected routes, 401 on invalid creds, no creds in logs, session expiry configured |
| `data-access` | Row-level access control, parameterized queries, error messages don't leak data existence |
| `api` | Input validation, request size limits, rate limiting, no stack traces in error responses |
| `input-validation` | All inputs validated (type, length, format), no eval() with user input, output properly escaped |

Stories without security tags continue to receive the existing general review unchanged.

**`discovery.go`** — Auto-detect `semgrep` (covers all Ralph ecosystems from one tool). If present, add to verify commands. Pass SAST output to AI analysis as `{{sastFindings}}` for triage — the AI determines which findings are real vs false positives.

#### Layer 4: `ralph secure` (deep architecture audit)

New standalone command for comprehensive security auditing beyond per-story scope.

**Characteristics:**
- Reads the entire codebase + infrastructure artifacts (Dockerfile, CI/CD, IaC, K8s, nginx, env files, migrations, auth middleware)
- Does NOT acquire the lock (read-only analysis, can run in parallel with `ralph run`)
- Outputs structured findings to `.ralph/<feature>/security-report.md` with severity levels (CRITICAL / HIGH / MEDIUM / LOW)
- Explicitly declares its scope limitations ("auditing declared infrastructure, not deployed state")

**Three tiers of depth:**

| Tier | Duration | Scope |
|------|----------|-------|
| Quick scan | ~5 min | Dependencies, Dockerfile basics, CI/CD permissions, .env handling, hardcoded secrets pattern matching |
| Focused audit | ~15 min | Auth middleware consistency, API endpoint protection, error handling patterns, database migrations, config files (nginx, CORS) |
| Architecture review | ~30 min | End-to-end data flow analysis, trust boundary mapping, secrets management strategy, logging/observability audit, dependency tree risk assessment |

**Prompt design principles:**
- Scope declaration upfront: "You are auditing what's in the repo, not what's deployed"
- Structured output: severity + file location + remediation suggestion per finding
- Confidence levels: HIGH (act on this), MEDIUM (flag for review), LOW (informational)
- Honest limits: explicitly list what cannot be assessed

### Implementation Order

Each phase is independently useful. Phase 1 alone closes the biggest gap.

1. **Phase 1 — PRD security awareness:** `SecurityTags` in schema, conditional questions in `prd-create.md`, criteria validation in `prd-finalize.md`
2. **Phase 2 — Implementation guidance:** `{{securityFocus}}` in `run.md`, security patterns in `consult.md`
3. **Phase 3 — Structured verification:** Story-type checklists in `verify-analyze.md`, Semgrep auto-detection in `discovery.go`, SAST output as AI triage input
4. **Phase 4 — Deep audit command:** `ralph secure` with tiered prompts, security report output, architecture-level analysis

---

## Synthesis: Ralph vs the Ralph Ecosystem

**Status:** Research complete
**Date:** 2026-03-10
**Motivation:** Understand where Ralph v2 sits relative to the original technique (ghuntley.com/ralph), the reference playbook (ghuntley/how-to-ralph-wiggum), and the popular bash implementation (snarktank/ralph) — and identify gaps worth closing.

### Background

The "Ralph Wiggum" technique originated as a blog post by Geoffrey Huntley describing a methodology for autonomous AI-driven software development. The core insight: a bash while-loop that feeds a prompt to an AI agent, one task per fresh context window, with backpressure from tests as the only quality gate. Two implementations followed — Huntley's own reference playbook and snarktank's bash wrapper — before our Go rewrite.

### The Four Implementations

#### Original technique (ghuntley.com/ralph)

A philosophy, not a tool. The entire orchestrator is `while :; do cat PROMPT.md | claude ; done`. The AI owns everything: it reads the plan, picks the next task, runs tests, updates state, and commits. Quality comes from two sources: well-crafted specs (`specs/*.md`) and backpressure (tests/typecheck/lint baked into the prompt). The plan (`IMPLEMENTATION_PLAN.md`) is a living document the AI rewrites each iteration — disposable and regenerable. Greenfield-only by the author's own admission. Trust in eventual consistency is the core philosophy.

#### Reference playbook (ghuntley/how-to-ralph-wiggum)

A copy-paste starting point with concrete files under `files/`. The `loop.sh` (~50 lines) supports two modes: `./loop.sh plan` (gap analysis only) and `./loop.sh` (build from plan). Pushes after every iteration. Uses `claude -p --dangerously-skip-permissions --output-format=stream-json --model opus`. The prompts introduce the "numbered guardrails" pattern (99999-999999999999999 escalating importance) and explicit subagent parallelism directives ("500 Sonnet subagents for search, 1 for build/tests, Opus subagents for complex reasoning"). `AGENTS.md` is kept lean (~60 lines) as an operational-only document; all status/progress lives in `IMPLEMENTATION_PLAN.md`. No completion detection — runs until iteration limit or Ctrl+C.

#### snarktank/ralph (previous bash tool)

Added structure without changing the trust model. `ralph.sh` (~120 lines) introduces structured PRDs (`prd.json` with user stories, acceptance criteria, priority, `passes: boolean`), one-story-per-iteration discipline, a learning journal (`progress.txt` with a "Codebase Patterns" section), and a `<promise>COMPLETE</promise>` completion marker. Supports amp and claude via `--tool` flag. Archives previous runs on branch change. But critically: the AI self-manages all state — reads `prd.json`, picks the story, sets `passes: true`, runs whatever tests it thinks appropriate, and commits. The script just loops and checks for the COMPLETE marker. If the AI lies about passing or skips verification, nothing catches it.

#### Ralph v2 (our Go CLI)

Inverts the control model. The CLI is the orchestrator; the AI is a stateless code-writing subprocess. The CLI selects stories, manages state (immutable `prd.json` + separate `run-state.json`), runs verification commands independently, manages services, handles retries/skips, and determines completion. The AI communicates through a narrow marker protocol (`DONE`, `STUCK`, `LEARNING`) and the CLI independently verifies every claim — including checking that a commit actually exists after DONE. Full feature lifecycle: `ralph prd` (AI brainstorm + finalize) → `ralph run` (autonomous implementation loop) → `ralph verify` (mechanical + AI deep analysis) → `ralph refine` (interactive post-verification sessions). Adds service orchestration, resource consultation, PID-based locking, JSONL structured logging, process group isolation, readiness gates, and six-provider support.

### Side-by-Side Comparison

| Dimension | ghuntley (original) | ghuntley (playbook) | snarktank | Ralph v2 |
|---|---|---|---|---|
| Implementation | Philosophy | Bash ~50 LOC + 2 prompts | Bash ~120 LOC | Go ~8000+ LOC |
| Who picks next task | AI reads TODO | AI reads TODO | AI reads prd.json | CLI by priority |
| Who tracks completion | AI updates TODO | AI updates TODO | AI sets `passes: true` | CLI via run-state.json |
| Who runs tests | AI (inline, prompt-directed) | AI (inline, prompt-directed) | AI (inline, self-directed) | CLI (independent verify commands) |
| Can AI self-report done | Yes | Yes | Yes | No — CLI verifies commit + tests |
| PRD/plan mutability | AI rewrites plan each iteration | AI rewrites plan each iteration | AI mutates prd.json passes field | Immutable prd.json; separate state |
| Planning phase | Separate PROMPT_plan.md | Separate PROMPT_plan.md | None (manual PRD creation) | `ralph prd` with AI brainstorm |
| Learning persistence | AI writes AGENTS.md | AI writes AGENTS.md + IMPL_PLAN | AI writes progress.txt | CLI extracts LEARNING markers, deduplicates, caps at 50 |
| Subagent parallelism | 500 search / 1 build (prompt-directed) | 500 Sonnet search / 1 Sonnet build / Opus for reasoning | Not specified | Not prescribed to provider |
| Service management | None | None | None | Full lifecycle with health checks |
| Resource consultation | Manual specs | Manual specs | None | Auto-resolve deps, clone source, subagent guidance |
| Concurrency control | None | None | None | PID-based lock with stale detection |
| Branch management | Manual | Script pushes after each iteration | AI creates branch | CLI manages ralph/feature branches |
| Retry/skip | Regenerate plan / git reset --hard | Restart loop | AI self-manages | Automatic retry → auto-skip at threshold |
| Regression detection | None | None | None | Verify-at-top before each implementation |
| Completion detection | None (infinite loop) | Iteration limit | `<promise>COMPLETE</promise>` marker | DONE/STUCK/LEARNING markers + independent verification |
| Post-run lifecycle | None | None | Archive on branch change | Archive (summary) → refine sessions |
| Process isolation | None | None | None | Process groups + SIGINT/SIGTERM cleanup |
| Observability | Console output | Console + stream-json | Console + progress.txt | JSONL structured logs with filter/follow/summary |
| Readiness gates | None | None | None | Git, sh, dirs, commands, services |
| Provider support | claude | claude | amp, claude | amp, claude, opencode, aider, codex, custom |
| Target projects | Greenfield only (author's stated position) | Greenfield only | Small features | Any project with test infrastructure |

### What We Preserved from the Original

These ideas from Huntley's technique are load-bearing and survived into v2 unchanged:

- **Fresh context per task.** The foundational insight. Context pollution across stories degrades output quality. Each provider invocation gets a clean window with only the relevant story, learnings, and consultation guidance.
- **Git commits as proof of work.** Not self-reported flags, not JSON field mutations — a real commit in the history is the only evidence of implementation. Our v2 enforces this: DONE without a new commit is a failed attempt.
- **Backpressure via tests.** The only reliable quality gate for AI-generated code. Huntley embeds this in the prompt; we externalize it to CLI-managed verify commands. Same principle, different trust boundary.
- **Learning persistence across iterations.** Knowledge must survive context resets. Huntley uses AGENTS.md (AI-written); we use LEARNING markers (CLI-extracted). The mechanism differs but the purpose is identical: avoid repeating mistakes.
- **One task per iteration.** Prevents context exhaustion and maintains decision coherence. Huntley derives this from context window economics; we enforce it structurally via CLI-driven story selection.

### What We Changed

These are deliberate departures from the original technique, not accidental omissions:

- **Untrusted provider model.** The original technique trusts the AI completely — to pick the right task, run the right tests, report honestly, and manage its own state. This works when the human is watching. Our v2 assumes unattended operation where the AI is an untrusted subprocess communicating through a narrow marker protocol. The CLI independently verifies every claim.
- **Immutable plans.** Huntley's `IMPLEMENTATION_PLAN.md` is a living document the AI rewrites every iteration. This is elegant for adaptability but means the AI can silently drop tasks, change priorities, or lose track of requirements. Our `prd.json` is immutable during runs; the CLI tracks progress externally. The AI can signal STUCK but cannot reshape the plan.
- **CLI-owned state.** In all three prior implementations, the AI writes to state files directly (`IMPLEMENTATION_PLAN.md`, `prd.json` passes field, `progress.txt`). Our v2 never lets the provider touch state. All state writes go through `AtomicWriteJSON` in the CLI process. This eliminates an entire class of bugs (corrupt state, partial writes, race conditions).
- **Externalized verification.** Huntley's backpressure is inside the provider's context window — the prompt says "run tests" and the AI decides which tests to run and how to interpret results. Our v2 runs configured verify commands as separate processes after the provider exits. The provider cannot skip, fake, or selectively interpret verification.

### Gaps Worth Investigating

These are capabilities present in other implementations that we deliberately omitted or haven't yet addressed. Not all are worth adding — some conflict with our design principles — but each deserves consideration.

#### 1. Plan-then-build separation

**What ghuntley does:** Explicit planning mode (`PROMPT_plan.md`) runs gap analysis across the entire codebase with hundreds of subagents before any implementation begins. The planning phase produces a prioritized task list that the build phase consumes.

**What we do:** Planning is folded into `ralph prd`. The AI brainstorms a PRD with the user, finalizes it into stories, and then `ralph run` immediately starts implementing. There's no "study the entire codebase with 500 subagents and create a prioritized gap analysis" step between finalization and implementation.

**Why this matters:** The planning phase serves two purposes: (1) catching missing requirements before implementation begins, and (2) giving the AI a comprehensive codebase understanding that informs task ordering. Our PRD creation does (1) but relies on per-story codebase analysis during implementation for (2).

**Potential approach:** A `ralph plan` command or `ralph run --plan-first` flag that runs one provider iteration in analysis-only mode before entering the implementation loop. The prompt would instruct the AI to study the codebase against all stories and output warnings, dependency ordering suggestions, or missing acceptance criteria — but not modify the prd.json.

#### 2. Provider-internal parallelism hints

**What ghuntley does:** Prompts explicitly control subagent behavior: "up to 500 parallel Sonnet subagents for searches/reads and only 1 Sonnet subagent for build/tests. Use Opus subagents when complex reasoning is needed."

**What we do:** Our `run.md` prompt doesn't prescribe provider-internal parallelism. We leave it to the provider's own judgment.

**Why this matters:** The constraint "1 subagent for build/tests" prevents backpressure flooding — if multiple subagents run tests concurrently, the noise-to-signal ratio degrades. The "Opus for reasoning, Sonnet for search" tiering is a cost/quality optimization. These aren't arbitrary — they're tuned from observed failure modes.

**Potential approach:** Add optional `{{parallelismHints}}` to `run.md`, populated based on provider type. For Claude: subagent tiering guidance. For other providers: omit (they manage their own parallelism differently). This is low-priority — provider-internal behavior is opaque to us and providers are improving their own orchestration.

#### 3. Living implementation plan

**What ghuntley does:** `IMPLEMENTATION_PLAN.md` is rewritten by the AI every iteration. Tasks are added when bugs are discovered, removed when completed, reprioritized based on new understanding. The plan evolves with the codebase.

**What we do:** `prd.json` is immutable. If the AI discovers that a story is impossible, misdefined, or needs splitting, it signals STUCK and we retry/skip. The stories don't evolve mid-run.

**Why this matters:** In practice, implementation reveals requirements. A story might need splitting ("this is actually two changes"), reordering ("story 3 depends on story 5, not the reverse"), or amending ("the acceptance criteria assume an API that doesn't exist"). Our current model handles this through STUCK + skip, which loses the nuance of what the AI discovered.

**Potential approach:** Allow the provider to emit a new marker like `AMEND:story_id:reason` that the CLI logs but doesn't act on automatically. After a run completes (or on STUCK), `ralph run` could offer to re-enter `ralph prd` with the amendments as context. This preserves immutability during the run while capturing mid-implementation discoveries for the next planning cycle.

#### 4. Spec-level granularity

**What ghuntley does:** Multiple `specs/*.md` files, one per "topic of concern" (JTBD breakdown). Each spec is loaded by subagents as needed, not all at once.

**What we do:** A single `prd.md` → `prd.json` per feature. All stories live in one document.

**Why this matters:** For large features, a single PRD can consume significant context. Ghuntley's approach lets the AI load only the relevant spec for the current task. This is a context efficiency optimization — with 176K usable tokens, loading a 3K spec instead of a 15K PRD leaves more room for codebase analysis and implementation.

**Potential approach:** This would be a significant architecture change (multiple spec files per feature, story-to-spec mapping). The ROI is unclear for typical Ralph features (5-15 stories). Worth revisiting if users report context exhaustion on larger features.

#### 5. Explicit "capture the why" culture

**What ghuntley does:** Guardrail 99999: "When authoring documentation, capture the why — tests and implementation importance." The prompt explicitly tells the AI to document not just what it did, but why tests matter and why implementation decisions were made.

**What we do:** LEARNING markers capture facts ("This codebase uses Prisma for database access", "The auth middleware is in src/middleware/auth.ts") but don't structurally incentivize capturing reasoning.

**Why this matters:** "Why" learnings are more durable than "what" learnings. "Always validate JWT expiry because the session middleware doesn't check it" is more valuable than "JWT validation is in auth.ts". The reasoning helps future iterations make better decisions, not just avoid the same file paths.

**Potential approach:** Update `run.md` to add a sentence in the LEARNING marker instructions: "Good learnings explain WHY something matters, not just WHAT exists. Include the reasoning behind patterns you discover." Zero implementation cost — purely a prompt change.

#### 6. Self-correcting plan / discovery integration

**What ghuntley does:** The build prompt includes: "For any bugs you notice, resolve them or document them in IMPLEMENTATION_PLAN.md even if unrelated to the current piece of work." Discoveries feed back into the plan immediately.

**What we do:** The provider can emit LEARNING markers (which persist as guidance for future stories) and STUCK markers (which trigger retry/skip). But there's no mechanism for the provider to say "I found a bug in story 4's assumptions while implementing story 2" without going through the blunt STUCK channel.

**Potential approach:** This overlaps with gap 3 (living plan). The LEARNING mechanism already partially serves this purpose — a learning like "The users table has no email column, which story US-004 assumes exists" would be injected into US-004's prompt. The gap is that learnings are advisory, not structural — they don't change story ordering or skip decisions.

### Non-Gaps (Deliberate Omissions)

Some differences between our approach and ghuntley's are features, not bugs:

- **No `git reset --hard` recovery.** Ghuntley's escape hatch for broken codebases. We use verify-at-top + retry + skip instead. Destructive recovery is the user's choice, not the tool's.
- **No `git add -A`.** Ghuntley's prompts use `git add -A`. Our provider prompt uses `git add` with specific files. Blanket staging risks committing generated artifacts, `.env` files, or large binaries.
- **No `git push` after every iteration.** Ghuntley's loop.sh pushes after each iteration. We leave pushing to the user. Force-pushing on every iteration creates noise in shared repos and can't be undone.
- **No unlimited iterations by default.** Ghuntley's loop runs forever until Ctrl+C. Our run loop exits when `AllComplete()` is true. Infinite loops without completion detection waste compute when stories are done.
- **No AI-written AGENTS.md.** Ghuntley lets the AI update AGENTS.md with operational learnings. We use CLI-managed LEARNING markers instead. Letting the AI write to its own instruction file creates a feedback loop where bad learnings compound — the AI writes something wrong, reads it next iteration, and reinforces the error.

### Summary

Ralph v2 is a strict superset of the original technique's capabilities, with one fundamental architectural departure: the AI is untrusted. Everything else — fresh context per task, backpressure via tests, learning persistence, git as proof of work — is preserved. The gaps worth investigating (plan-then-build separation, living plans, "capture the why" prompting) are additive and don't require changing the trust model. The largest philosophical tension is between Huntley's "let Ralph Ralph" (AI self-directs, plan evolves organically) and our "CLI orchestrates" (deterministic control, immutable plans). Both work; they optimize for different failure modes. Huntley's approach is more adaptive but fragile to AI errors in state management. Ours is more rigid but robust to AI misbehavior.

---

## Refine Session Summary: Capturing Non-Code Progress

**Status:** Proposed
**Date:** 2026-03-10
**Motivation:** After an interactive `ralph refine` session, `generateRefineSummary` only receives `git log --oneline` + `git diff --stat`. The conversation itself — debugging insights, rejected approaches, architectural decisions, constraint discovery — is lost. If a session produces zero commits, nothing is persisted at all.

### Current State

`generateRefineSummary` in `refine.go` captures pre-session commit hash, then after the interactive session ends:

1. Runs `git log --oneline <preCommit>..HEAD` — commit messages only
2. Runs `git diff --stat <preCommit>..HEAD` — file names and line counts
3. Feeds both to a non-interactive summarizer subagent (via `prompts/refine-summarize.md`)
4. Extracts summary between `SUMMARY_START`/`SUMMARY_END` markers
5. Appends to `summary.md`

If zero commits exist, `generateRefineSummary` returns nil immediately — no summary written.

The interactive session connects provider stdin/stdout/stderr directly to the terminal (`cmd.Stdout = os.Stdout` in `prd.go`). No output is captured by ralph. This is intentional: `Setpgid` must be false for the provider to read from the controlling terminal, and piping would break terminal control sequences.

### What's Lost

**Zero-commit sessions vanish entirely.** These scenarios produce no record:
- Debugging sessions that identify root causes but require no code fix
- Architecture evaluations where an approach is investigated and rejected
- Constraint discovery (e.g., "localStorage has a 5MB limit, current state is 3MB, offline mode infeasible")
- Performance profiling that concludes the bottleneck is elsewhere

**Commit messages are lossy.** "fix: handle edge case in auth" doesn't capture why that approach was chosen over alternatives, what was tried first, or what constraints were discovered during investigation.

**Refine is where human-guided investigation happens** — exactly where reasoned conclusions about system properties would be most valuable for future agents to know.

### Provider Conversation History Accessibility

Every supported provider stores session history locally, in provider-specific formats:

| Provider | Location | Format |
|----------|----------|--------|
| Claude Code | `~/.claude/projects/` | JSONL |
| Aider | `.aider.chat.history.md` | Markdown |
| Codex | `~/.codex/sessions/` | Proprietary |
| OpenCode | `~/.local/share/opencode/storage/` | SQLite + JSON |
| Amp | Cloud (GCP) | Not locally accessible |

The conversation exists on disk after the session — but in 5 different formats at 5 different locations. Ralph doesn't know how to read any of them.

### Options Evaluated

**Option A: Parse provider conversation logs post-session.**
Feed provider-specific history files to the summarizer. Rejected — 5 different formats, fragile coupling to provider internals, privacy concerns, massive token cost (conversations can be 50k+ tokens). Would break whenever a provider changes its storage format.

**Option B: Tee interactive output through `io.MultiWriter`.**
Capture stdout while still displaying to terminal. Rejected — breaks terminal control sequences (ANSI escapes, cursor movement), fragile across providers, doesn't capture user input side. Would degrade the interactive experience.

**Option C: Feed full `git diff` to the summarizer (not just `--stat`).**
Currently uses `git diff --stat` which only shows file names and line counts. Switching to `git diff` (truncated to a reasonable size) gives the summarizer actual code context to reason about what changed and why. Low effort (~10 lines changed), no architectural change, significantly better summaries for code-change sessions. Doesn't solve zero-commit sessions.

**Option D: Post-session notes prompt.**
After the interactive session exits, prompt the user: "Any notes to record from this session? (empty to skip)". Append notes to `summary.md` even if zero commits. Lightweight, provider-agnostic, captures exactly what the human thinks is important. Could also accept notes via flag: `ralph refine --note "rejected websockets due to browser compat"`.

**Option E: Extend LEARNING markers to interactive sessions.**
Currently refine sessions don't do marker detection (interactive mode bypasses it entirely). Not feasible without fundamentally changing interactive session architecture — would require piping output, which breaks terminal interactivity.

### Recommendation

**Implement C + D together.** They're complementary, both low-effort, and address different failure modes:

#### Change 1: Enrich summarizer input (Option C)

In `generateRefineSummary`, replace `git diff --stat` with a truncated `git diff` (cap at ~8000 chars to stay within reasonable prompt size). The summarizer AI can then reason about actual code changes — what patterns were introduced, what was refactored and why — instead of just "3 files changed, 42 insertions".

**Files affected:** `refine.go` (~10 lines)

#### Change 2: Post-session notes (Option D)

After `runProviderInteractive` returns, before checking for commits:
1. Prompt: "Record any notes from this session? (empty to skip)"
2. If notes provided and commits exist: include notes in the summarizer prompt as additional context alongside the git log/diff
3. If notes provided and zero commits: write notes directly to `summary.md` under a `## Notes (date)` heading
4. If no notes and zero commits: no change (current behavior)

The user is the best judge of what's worth persisting. This respects that while being trivially implementable. The notes flow through the same `summary.md` path that future refine sessions already consume via `{{summary}}`.

**Files affected:** `commands.go` (cmdRefine, ~15 lines), `refine.go` (pass notes to summarizer, ~10 lines), `prompts/refine-summarize.md` (add `{{sessionNotes}}` section, ~5 lines)

### Why Not Raw Conversation Logs

Research on tiered memory systems for AI agents ([arxiv.org/html/2602.17913v1](https://arxiv.org/html/2602.17913v1)) shows that raw logs achieve 0.873 accuracy but at extreme token cost, while pure summaries achieve 0.851 accuracy with 54% token reduction. The sweet spot is structured summaries with selective detail — which is exactly what enriched diff + human-curated notes provides.

Ralph's design principle is that the CLI orchestrates and the AI is a stateless subprocess. Parsing provider conversation logs would couple Ralph to provider internals, violating this boundary. Human-provided notes maintain the clean separation: the user observed the conversation and distills what matters.

---

## Accuracy Improvement: Data Flow Gaps

**Status:** Verified (32/32 claims confirmed against codebase)
**Date:** 2026-03-10
**Motivation:** Ralph's sole metric is accuracy — stories implemented correctly without human intervention, fewer retries, less babysitting. Three rounds of comparative analysis (Stripe Minions, OpenAI Symphony, Tidewave) revealed that Ralph already captures most of the information needed for higher accuracy but has concrete data-flow gaps where that information never reaches the AI provider.

### Comparative Analysis Synthesis

Three frameworks were analyzed for accuracy-relevant features that Ralph could adopt:

**Stripe Minions** (internal, not available): Reference architecture for large-scale orchestration. Key finding: structured retry context and failure classification are the highest-leverage improvements, identified independently by this analysis and the Minions architecture pattern. Blueprint/DAG engines are over-engineering at Ralph's scale.

**OpenAI Symphony** (open source, Codex-only): Explicitly labeled "prototype software intended for evaluation only" in its README. Zero published benchmarks, success rates, or accuracy data. Zero production deployments documented. Locked to Codex + Linear + Elixir runtime. Three accuracy-relevant features worth adopting: procedural skills (prompt text, 0 LOC), failure classification (~60 LOC), and mid-execution feedback (~200 LOC, deferred). Everything else Symphony offers either doesn't affect accuracy or conflicts with Ralph's provider-agnostic model. Migration cost: 6-8 weeks to rewrite PRD format, prompts, state management, verification — for gains achievable by importing 3 features in ~2 weeks.

**Tidewave** (open source, Elixir MCP server): Different layer entirely — runtime intelligence tools, not orchestration. Key finding: the biggest win isn't MCP integration; it's that Ralph already captures 256KB of service logs per service via `ServiceManager.GetRecentOutput` and never shows them to the AI. Full MCP integration is a month-2 item; service log injection is a 15-line fix.

**Cross-analysis convergence:** All three analyses independently identified the same #1 priority — structured retry context. The provider currently gets only a truncated string reason on retry (`prompts.go:117-125`). It doesn't see what code it wrote, what tests actually output, or what type of failure occurred. Every retry is partially guessing.

**Accuracy estimates:** ~70-80% first-attempt success rate today, ~95-96% after retries. With Phase 1+2 improvements: ~75-85% first attempt, ~96-97% after retries. Theoretical ceiling with current architecture: ~92-95% first attempt, ~98-99% after retries. The gap between improved and ceiling is closable via mid-execution feedback (CHECKPOINT markers) — a future phase.

### The Core Problem: Data Flow Gaps

Ralph captures information that the AI never sees. These are the specific gaps:

| Data | Captured Where | Shown to AI? | Gap |
|------|---------------|-------------|-----|
| Git diff of failed attempt | Available via `git diff <preRunCommit>..HEAD` | No | Provider can't see what it already tried |
| Full test output | `truncateOutput` at 50 lines in `loop.go:685` | Only 50-line truncation in `lastFailure` | Error details lost |
| Failure type (compile vs test vs lint) | Not classified | No | Provider can't prioritize fix strategy |
| Service logs (ERROR/PANIC) | `ServiceManager.GetRecentOutput` in `services.go:253` | Only in verification reason string | Provider misses startup/migration failures |
| Pre-existing test failures | Not captured | No | Provider chases failures it didn't cause |
| All verification failures | Stops at first failure (`loop.go:686` early return) | Only first failure | Provider fixes one issue per retry cycle |
| Changed file list | Available via `git diff --name-only` | Not in `verify-analyze.md` | Verify AI told to "read the code" with no file list |

### Phase 1: Structured Retry Context (Highest ROI)

The #1 failure mode is retrying blindly. Currently the AI gets only a string reason on retry (`prompts.go:117-125`). It doesn't see what code it wrote, what tests output, or what type of failure occurred.

**1.1 Add retry context fields to RunState**

`schema.go` — Add to `RunState` struct (line 35-42):
- `LastDiff map[string]string` — git diff of provider's changes from the failed attempt
- `LastTestOutput map[string]string` — full verification command output (not just 50 lines)
- `FailureClass map[string]string` — classified failure type

All `json:",omitempty"` maps — backward compatible (nil = not present in old files). Add getter/setter methods following existing `GetLastFailure`/`MarkFailed` patterns.

**1.2 Capture retry context after failed verification**

`loop.go` — After `runStoryVerification` returns failure (~line 396):
- `captureRetryDiff(git, preRunCommit)` — new func, runs `git diff <preRunCommit>..HEAD`, truncates to 200 lines
- Store full test output from `StoryVerifyResult` (expand struct to carry `fullOutput`)
- `classifyFailure(output) string` — new func, returns `"compile"`, `"test"`, `"lint"`, `"timeout"`, `"service"`, `"no-commit"`, or `"unknown"` via simple pattern matching

**1.3 Inject structured context into prompts**

`prompts.go` — Expand retry section in `generateRunPrompt` (line 117-125) to include failure class, previous diff, and test output.

`prompts/run.md` — Add after retry info section:
- Explicit retry guidance: "Review the diff from your previous attempt. Try a DIFFERENT approach. Do NOT output DONE if verification fails."
- Template vars: `{{retryDiff}}`, `{{retryTestOutput}}`, `{{retryClassification}}`

**Tests:** `schema_test.go` (getter/setter round-trips, backward compat), `loop_test.go` (`TestClassifyFailure` for each classification), `prompts_test.go` (retry context appears in generated prompt).

### Phase 2: Service Health + Log Injection

Prevents cascading failures and gives AI visibility into server errors. Independent of Phase 1.

**2.1 Health check before provider spawn**

`services.go` — Add `EnsureHealthy()` method: iterates started services, checks `isReady(url)`, restarts unhealthy ones.

`loop.go` — Call `svcMgr.EnsureHealthy()` before `runProvider()` (~line 269). Log warning on failure, don't block.

**2.2 Service log injection**

`prompts.go` — Add `buildServiceLogs(svcMgr, services) string` helper. Returns formatted last 30 lines per service, or "" if no services.

Pass `serviceLogs` string to `generateRunPrompt`, `generateVerifyFixPrompt`, `generateVerifyAnalyzePrompt`. This adds a parameter to each function signature — update all callers in `loop.go`.

`prompts/run.md`, `prompts/verify-fix.md`, `prompts/verify-analyze.md` — Add `{{serviceLogs}}` template variable.

**Tests:** `services_test.go` (`TestEnsureHealthy`), `prompts_test.go` (`TestBuildServiceLogs` empty/populated).

### Phase 3: Collect All Failures + Pre-flight Baseline

Depends on Phase 1 (uses `FailureClass` and `LastTestOutput` fields).

**3.1 Collect ALL verification failures**

`loop.go` — Modify `StoryVerifyResult` (line 665): replace `reason string` with `failures []string`. In `runStoryVerification` (line 672-751), remove early returns on failure — continue loop, append each failure. Add `reason() string` method that joins failures.

NOT doing parallel verification — commands share state (dev server, database, filesystem). Concurrent execution causes flaky failures that decrease accuracy.

**3.2 Pre-flight baseline**

`schema.go` — Add `BaselineFailures []string` to RunState (omitempty, nil = not captured).

`loop.go` — Before main story loop (~line 142): if `state.BaselineFailures == nil`, run `runStoryVerification` with nil story (skip UI commands), store results. Only run once.

`prompts.go` — Add baseline info to `generateRunPrompt`: list pre-existing failures so AI doesn't chase them.

`prompts/run.md` — Add `{{baselineInfo}}` section.

**Gotcha**: `runStoryVerification` currently assumes non-nil story for `IsUIStory()` check — add nil guard.

**Tests:** `loop_test.go` (`TestStoryVerifyResult_CollectsAllFailures`), `schema_test.go` (`TestRunState_BaselineFailures`), `prompts_test.go` (baseline info in prompt).

### Phase 4: MCP Config Generation + Prompt Polish

Independent of all other phases. Enables Tidewave and other MCP tools.

**4.1 MCP config support**

`config.go` — Add `MCPServerConfig` struct (`Name`, `Command`, `Args`, `Env`) and `MCPConfig` struct. Add `MCP *MCPConfig` to `RalphConfig` (omitempty).

`mcp.go` (new, ~100 lines) — `generateMCPJson(projectRoot, mcpCfg)` writes `.mcp.json` in Claude Code format. `cleanupMCPJson(projectRoot)` removes it. Uses `AtomicWriteJSON`.

`loop.go` — Before `runProvider`: generate `.mcp.json` if MCP configured. After provider exits: clean up. Add to `CleanupCoordinator` for signal handling.

**4.2 Prompt template improvements**

`prompts/run.md` — Add explicit "Do NOT output DONE if any verification command fails."

`prompts/verify-analyze.md` — Add `{{changedFiles}}` (list from `git diff --name-only`). Currently Step 1 tells AI to "read the code" with no file list.

`prompts.go` — In `generateVerifyAnalyzePrompt`, compute changed files list and pass as template var.

**Tests:** `mcp_test.go` (new: generate/cleanup/path tests), `config_test.go` (load config with/without MCP), `prompts_test.go` (changed files in verify-analyze prompt).

### Phase Sequencing

```
Phase 1 ──────────────> Phase 3
(retry context)         (all failures + baseline)
                        [depends on Phase 1]

Phase 2 ──────────────> (independent)
(service health/logs)

Phase 4 ──────────────> (independent)
(MCP + prompt polish)
```

Phases 1, 2, 4 can be done in parallel. Phase 3 requires Phase 1 complete.

### Key Files Modified

| File | Phases | Changes |
|------|--------|---------|
| `schema.go` | 1, 3 | RunState fields: LastDiff, LastTestOutput, FailureClass, BaselineFailures |
| `loop.go` | 1, 2, 3, 4 | Retry capture, health check, all-failures collection, baseline, MCP lifecycle |
| `prompts.go` | 1, 2, 3, 4 | All prompt generators get new context parameters |
| `services.go` | 2 | EnsureHealthy method |
| `mcp.go` (new) | 4 | MCP config generation/cleanup |
| `config.go` | 4 | MCPConfig types |
| `prompts/run.md` | 1, 2, 3 | Retry guidance, service logs, baseline info |
| `prompts/verify-fix.md` | 2 | Service logs |
| `prompts/verify-analyze.md` | 2, 4 | Service logs, changed files |

### Existing Code to Reuse

- `AtomicWriteJSON` (schema.go) — all state and config writes
- `truncateOutput` (loop.go) — truncating diffs and test output
- `ServiceManager.GetRecentOutput` (services.go) — already captures service logs
- `ServiceManager.isReady` (services.go) — health check logic
- `CleanupCoordinator` (cleanup.go) — signal handling for MCP cleanup
- Template var replacement pattern in prompts.go — `{{var}}` string replacement

### Verification Strategy

After each phase:
1. `go vet ./...` — no static analysis issues
2. `go test ./...` — all unit tests pass
3. Manual test on a real project:
   - Phase 1: Force a test failure, verify retry prompt contains diff + test output + classification
   - Phase 2: Kill dev server mid-run, verify Ralph restarts it; verify service logs appear in prompts
   - Phase 3: Start with pre-existing test failures, verify AI doesn't try to fix them; force multiple verification failures, verify all shown
   - Phase 4: Configure MCP server, verify `.mcp.json` generated before provider and cleaned up after

### Claim Verification

All 32 code-level claims in this plan were adversarially verified against the actual codebase (commit `4135b80`). Every file reference, function name, struct field, line number, and architectural claim was confirmed. Two line numbers were off by 1 (trivial). No hallucinations detected.
