# Phase 0 Refinement Specification (Round 01)

**Purpose:** Fix verified gaps between PHASE0-SPEC.md and the current implementation. Every task below was confirmed by adversarial verification agents reading the actual source code. No speculative items.

**Source of truth:** PHASE0-SPEC.md remains the design document. This refinement spec addresses implementation gaps only.

**Lineage:** The scrip v1 core implementation is complete (all 6 sessions pass, build/test/vet clean). This refinement pass addresses 18 verified issues organized by priority.

---

## Table of Contents

- [Task Inventory](#task-inventory)
- [Tier 1: Must Fix](#tier-1-must-fix)
- [Tier 2: Should Fix](#tier-2-should-fix)
- [Tier 3: Cleanup](#tier-3-cleanup)
- [Tier 4: Test Coverage](#tier-4-test-coverage)
- [Implementation Contract](#implementation-contract)

---

## Task Inventory

18 tasks across 4 priority tiers. Each task is independent — no inter-task dependencies. Work top-to-bottom by tier.

| # | Task | Tier | Files | Est. LOC |
|---|------|------|-------|----------|
| 1 | Make timeout retryable | 1 | cmd_exec.go | ~20 |
| 2 | Rename binary to scrip | 1 | Makefile, .goreleaser.yaml | ~5 |
| 3 | Wire SetProvider(pid) | 2 | cmd_exec.go | ~15 |
| 4 | Add subagent instructions to consult-item.md | 2 | prompts/consult-item.md | ~20 |
| 5 | Wire consult-feature.md in plan and land | 2 | cmd_plan.go, cmd_land.go, consultation.go | ~60 |
| 6 | Implement stall timeout | 2 | cmd_exec.go | ~40 |
| 7 | Rename infrastructure files | 2 | README.md, install.sh, ralph.schema.json, go.mod | ~200 |
| 8 | Wire exec-verify.md after DONE | 2 | cmd_exec.go | ~30 |
| 9 | Populate PlanRound.Consultation | 3 | cmd_plan.go | ~5 |
| 10 | Delete unused progress-narrative.md | 3 | prompts/progress-narrative.md | -30 |
| 11 | Delete dead buildProviderArgs | 3 | loop.go, loop_test.go | -130 |
| 12 | Rename Story→Item terminology | 3 | loop.go, logger.go, cmd_exec.go, consultation.go, prompts/consult-item.md | ~80 |
| 13 | Fix stale PRD comments | 3 | consultation.go, discovery.go | ~5 |
| 14 | Add retry diff to context | 3 | cmd_exec.go, git.go | ~25 |
| 15 | Add TestMergeWithExisting | 4 | cmd_prep_test.go | ~40 |
| 16 | Add retry/stuck integration test scenarios | 4 | integration_test.go | ~80 |
| 17 | Add TestStrictRendering | 4 | prompts_test.go | ~40 |
| 18 | Add TestLandFailPlanLoop | 4 | integration_test.go | ~100 |

---

## Tier 1: Must Fix

These cause real runtime failures or ship-blocking issues.

### Task 1: Make provider timeout retryable instead of fatal

**Problem:** `cmd_exec.go:407-413` treats ALL `provErr != nil` as fatal, including timeouts. A provider that times out kills the entire exec loop instead of retrying.

**Spec reference:** PHASE0-SPEC.md line 1017: "No marker + timeout → Kill process group. Log timeout. Retry as stall."

**Root cause:** `scripSpawnProvider` returns `(result, err)` on timeout (line 631) where `result.TimedOut == true`. The caller at line 407 catches all `provErr != nil` uniformly.

**Fix:** In `cmd_exec.go`, after `scripSpawnProvider` returns, distinguish timeout from pre-start failures:

```go
// BEFORE the existing provErr != nil block:
if provErr != nil && result != nil && result.TimedOut {
    // Timeout — retry as stall (same pattern as STUCK handler)
    reason := fmt.Sprintf("Provider timed out after %d seconds", cfg.Config.Provider.Timeout)
    fmt.Printf("  ⏱ %s\n", reason)
    _ = AppendProgressEvent(progressPath, &ProgressEvent{
        Event:   ProgressItemStuck,
        Item:    item.Title,
        Attempt: itemState.Attempts + 1,
        Reason:  reason,
    })
    if itemState.Attempts+1 >= scripMaxRetries {
        _ = AppendProgressEvent(progressPath, &ProgressEvent{
            Event:  ProgressItemDone,
            Item:   item.Title,
            Status: "skipped",
        })
    }
    sessState.ClearProvider()
    _ = SaveSessionState(statePath, sessState)
    continue
}
// Keep existing provErr != nil block for pre-start failures (result == nil)
```

**Critical constraint:** Pre-start failures (`result == nil`, e.g., "claude" binary not found) MUST remain fatal. Only timeout (`result.TimedOut == true`) becomes retryable.

**Exit criteria:** A provider timeout no longer kills the exec loop. The item is retried up to `scripMaxRetries`, then skipped. Pre-start failures still exit immediately.

**Tests:** Add test case to `TestScripProcessLine` or a new test verifying timeout retry behavior.

---

### Task 2: Rename binary output in Makefile and .goreleaser.yaml

**Problem:** `Makefile:7` builds `-o ralph`. `.goreleaser.yaml` has `binary: ralph` and `name_template: "ralph-..."`.

**Fix:**
- `Makefile:7` — change `-o ralph` to `-o scrip`
- `.goreleaser.yaml:14` — change `binary: ralph` to `binary: scrip`
- `.goreleaser.yaml:18` — change `name_template: "ralph-{{ .Os }}-{{ .Arch }}"` to `name_template: "scrip-{{ .Os }}-{{ .Arch }}"`

**Do NOT change** `.goreleaser.yaml:23` (`name: ralph` under `release.github`) — this must match the actual GitHub repository name. Only change it if/when the repo is renamed.

**Exit criteria:** `make build` produces a binary named `scrip`. `goreleaser` would produce release artifacts named `scrip-*`.

---

## Tier 2: Should Fix

Quality and spec compliance gaps. The system works without these but is degraded.

### Task 3: Wire SetProvider(pid) in cmd_exec.go

**Problem:** `state.go:35` defines `SetProvider(pid)` but `cmd_exec.go` never calls it. `ProviderPID` and `ProviderStartedAt` are always 0 in state.json, making crash recovery PID detection non-functional.

**Root cause:** `cleanup.SetProvider(cmd)` at `cmd_exec.go:571` sets the CleanupCoordinator's provider (for signal handling), NOT the SessionState's provider. These are different types on different structs.

**Fix:** Pass `sessState` and `statePath` into `scripSpawnProvider`. After `cmd.Start()` succeeds (around line 568), call:
```go
sessState.SetProvider(cmd.Process.Pid)
_ = SaveSessionState(statePath, sessState)
```

This must happen BEFORE the blocking I/O loop begins, while the provider is still alive.

**Alternative (simpler):** Have `scripSpawnProvider` return the PID via a callback or channel right after `cmd.Start()`. However, the blocking nature of the function makes this awkward — passing sessState directly is cleaner.

**Exit criteria:** state.json contains non-zero `provider_pid` and `provider_started_at` while a provider is running. `IsProviderAlive` returns `true` for a live provider.

---

### Task 4: Add subagent instructions to consult-item.md

**Problem:** `prompts/consult-item.md` is missing the spec-required subagent control instructions (PHASE0-SPEC.md:474-480). The template works at runtime (variable names are internally consistent with consultation.go), but the spawned agent self-limits to 3-5 subagents instead of 250-500.

**What to add** (do NOT rename variables — they match the Go code):
1. "Study the cached framework source code using up to 500 parallel Sonnet subagents" — add to the research instructions
2. "Use Opus subagents when evaluating architectural patterns or resolving conflicting approaches" — add after the search instruction
3. Change citation format from `Source: path/to/file.ts` to `Source: file:line` (with line numbers)
4. Add "Uncited guidance is treated as hallucination and will be discarded by the CLI"

**Do NOT change** the template variable names (`{{storyId}}`, `{{storyTitle}}`, `{{acceptanceCriteria}}`, etc.). They are internally consistent with `consultation.go:337-344`. Renaming them is Task 12 (Story→Item rename).

**Exit criteria:** Template includes subagent parallelism instructions, Opus instructions, line-number citation format, and hallucination warning.

---

### Task 5: Wire consult-feature.md in cmd_plan.go and cmd_land.go

**Problem:** `consult-feature.md` template exists and is tested, but no production code invokes it. `cmd_plan.go:283` just lists resource paths. `cmd_land.go:136` discards the resource manager (`_ = rm`).

**Fix:** Write a `consultForFeature` function in `consultation.go` (analogous to `consultForItem`):
1. Use `allCachedFrameworks` (consultation.go:191) to get relevant frameworks
2. Use `featureConsultCacheKey` (consultation.go:206) for caching
3. Use `getPrompt("consult-feature", ...)` to render the template
4. Spawn non-autonomous claude (`ScripProviderArgs(false)`) per framework
5. Validate citations, format results

Then wire it:
- `cmd_plan.go` — replace `buildPlanConsultation` body with `consultForFeature` call (or call consultForFeature and format the result)
- `cmd_land.go` — replace `_ = rm` / `buildResourceFallbackInstructions()` with actual `consultForFeature` call using the resource manager

**Exit criteria:** `scrip plan` and `scrip land` produce framework-specific consultation guidance (with citations) when cached resources exist, falling back to web search instructions when they don't.

---

### Task 6: Implement stall timeout in scripSpawnProvider

**Problem:** `config.go:59` defines `StallTimeout` (default 300s) but `scripSpawnProvider` has no idle-output detection. A silent provider hangs for 30 minutes (hard timeout) instead of 5 minutes (stall timeout).

**Fix:** Add a stall timer to `scripSpawnProvider`:

1. Accept `stallTimeoutSec int` as a new parameter
2. Create a `time.Timer` for `stallTimeout` duration
3. In the stdout scanner loop (line 609-618), reset the timer on each line: `stallTimer.Reset(stallDuration)`
4. In the stderr goroutine (line 591-604), also reset the timer on each line
5. Add a goroutine that waits on `stallTimer.C` and calls `cancel()` when it fires
6. Set `result.TimedOut = true` when stall timer fires (same as hard timeout)

**Synchronization:** Use a channel-based approach — both scanner loops send on an `activityCh` channel, a watcher goroutine selects between `activityCh` and `stallTimer.C`.

**Update all call sites** to pass `cfg.Config.Provider.StallTimeout` as the new parameter:
- `cmd_exec.go:370`
- `cmd_land.go:143, 172, 245`

**Exit criteria:** A provider that produces no output for `stallTimeout` seconds is killed. Active providers (producing output) are not affected. Hard timeout remains as the outer bound.

---

### Task 7: Rename infrastructure files from ralph to scrip

**Problem:** Go source is clean, but build/release/docs infrastructure still says "ralph".

**Files to change:**

1. **README.md** — Full rewrite. All command examples (`ralph init` → `scrip prep`, `ralph prd` → `scrip plan`, `ralph run` → `scrip exec`, `ralph verify` → `scrip land`), paths (`.ralph/` → `.scrip/`), markers (`<ralph>` → `<scrip>`), config (`ralph.config.json` → `.scrip/config.json`). ~81 occurrences.

2. **install.sh** — Rename function `install_ralph` → `install_scrip`, `REPO` value, `ASSET_NAME`, binary name in `mv` command, success message. ~8 occurrences. Note: `REPO` should only change if the GitHub repo is actually renamed.

3. **ralph.schema.json** — Rename file to `scrip.schema.json`. Update title, description, and fix stale `"default": "~/.ralph/resources"` → `"~/.scrip/resources"` (line 164 — this is a real bug, the code already uses `~/.scrip/resources`). Update any code references to the schema filename.

4. **go.mod** — Change `module github.com/scripness/ralph` to `module github.com/scripness/scrip`. This is safe — single-package module, zero internal imports. Only one line changes.

**Exit criteria:** No "ralph" references in README.md, install.sh, schema file, or go.mod (except intentional repo URL references that must match GitHub).

---

### Task 8: Wire exec-verify.md AI deep analysis after DONE

**Problem:** `generateExecVerifyPrompt` (cmd_exec.go:741) and `prompts/exec-verify.md` exist but are never called. After DONE, only mechanical verification runs.

**Spec reference:** PHASE0-SPEC.md:482-484: "AI deep analysis after the provider signals DONE. Runs as a non-autonomous subagent."

**Fix:** After mechanical verification passes (around cmd_exec.go:492-516), add:
1. Build prompt via `generateExecVerifyPrompt(item, criteria, diff, testOutput)`
2. Spawn non-autonomous claude: `scripSpawnProvider(projectRoot, prompt, timeout, false, logger, cleanup)`
3. Parse output with `landParseAnalysis` (reuse from cmd_land.go, or extract shared function)
4. On VERIFY_PASS → advance as normal
5. On VERIFY_FAIL → treat as verification failure (same retry path as mechanical failure)

**Performance consideration:** This adds one API call per item. For a 10-item plan, that doubles claude invocations. Consider making this opt-in via a config flag or `--deep-verify` CLI flag. The spec requires it, but pragmatism suggests it should be toggleable.

**Exit criteria:** After a provider signals DONE and mechanical verification passes, AI deep analysis runs and can trigger retries on VERIFY_FAIL. The feature can be disabled if opt-in is implemented.

---

## Tier 3: Cleanup

Technical debt. No functional impact. Safe to defer.

### Task 9: Populate PlanRound.Consultation field

**Problem:** `plan.go:41` defines `Consultation []string` but `cmd_plan.go:150-156` never sets it.

**Fix:** In `cmd_plan.go`, when constructing `PlanRound` objects (around line 150), add:
```go
Consultation: []string{consultation},
```
where `consultation` is the pre-computed string from line 84.

**Exit criteria:** plan.jsonl entries contain non-empty `consultation` arrays. `BuildPlanHistory` verbatim rounds show consultation content.

---

### Task 10: Delete unused progress-narrative.md

**Problem:** `prompts/progress-narrative.md` exists but no Go code calls `getPrompt("progress-narrative", ...)`. Narratives are built programmatically (better: deterministic, free, testable).

**Fix:** Delete `prompts/progress-narrative.md`.

**Exit criteria:** File deleted. `go build` still passes (embed picks up remaining files).

---

### Task 11: Delete dead buildProviderArgs and multi-provider tests

**Problem:** `loop.go:31-61` has `buildProviderArgs()` supporting stdin/arg/file modes for amp/aider/opencode/codex. Zero production callers. `loop_test.go:12-157` tests these dead code paths.

**Fix:**
- Delete `buildProviderArgs` function from `loop.go`
- Delete all `TestBuildProviderArgs_*` test functions from `loop_test.go`
- Keep `StoryVerifyResult` and `runCommand` (they have active callers)

**Exit criteria:** `go build` and `go test` pass. No multi-provider code remains.

---

### Task 12: Rename Story→Item terminology

**Problem:** ~77 occurrences of "Story" terminology across 7+ files. Internal naming inconsistency with the plan-item model.

**Scope (by file):**
- `logger.go` — `StorySummary` → `ItemSummary`, `StoryID` → `ItemID`, `SetCurrentStory` → `SetCurrentItem`, `currentStory` → `currentItem`, ~31 occurrences
- `consultation.go` — `storyID` → `itemID`, `storyDescription` → `itemDescription`, template map keys, ~14 occurrences
- `cmd_exec.go` — `SetCurrentStory` call → `SetCurrentItem`, ~3 occurrences
- `loop.go` — `StoryVerifyResult` → `ItemVerifyResult`, ~2 occurrences
- `prompts/consult-item.md` — `{{storyId}}` → `{{itemId}}`, `{{storyTitle}}` → `{{itemTitle}}`, `{{storyDescription}}` → `{{itemDescription}}`, ~7 occurrences
- `consultation_test.go` — matching test updates, ~11 occurrences
- `logger_test.go` — matching test updates, ~9 occurrences

**Critical constraint:** The `Event` struct's JSON tag `json:"story,omitempty"` (logger.go:49) is a **wire format**. Keep it as `"story"` for backward compatibility with existing JSONL log files. Only rename the Go field name, not the JSON tag.

**Exit criteria:** No "Story" in Go identifiers (except JSON tags for backward compatibility). All tests pass.

---

### Task 13: Fix stale PRD comments

**Problem:** Two comments reference "PRD" instead of "plan":
- `consultation.go:190` — "Used for feature-level consultations (PRD, verify)" → "Used for feature-level consultations (plan, land)"
- `discovery.go:19` — "Used to provide context to PRD creation prompts" → "Used to provide context to plan creation prompts"

**Fix:** Update the two comment strings. Nothing else.

**Exit criteria:** No "PRD" in comments (grep confirms).

---

### Task 14: Add diff from failed attempt to retry context

**Problem:** Spec line 789 says retry includes "diff from failed attempt" but `cmd_exec.go:347-353` only includes failure reason text.

**Fix:**
1. Add a `DiffBetweenCommits(hash1, hash2 string) (string, error)` function to `git.go`
2. In the retry context builder (cmd_exec.go:347-353), if `itemState.LastCommit` is set, compute `DiffBetweenCommits(preItemCommit, itemState.LastCommit)` and append a `## Previous Attempt Diff` section
3. Cap the diff at ~8KB to avoid consuming too much context window

**Note:** The spec (line 1028) explicitly lists "retry heuristics" as post-Phase 0 refinement work. This task implements the minimum viable version.

**Exit criteria:** Retry context includes a truncated diff from the previous attempt when available.

---

## Tier 4: Test Coverage

### Task 15: Add TestMergeWithExisting

**Problem:** `cmd_prep.go`'s `mergeWithExisting` function has zero test coverage. It has non-trivial merge logic (preserving user customizations).

**Fix:** Create `cmd_prep_test.go` with:
```go
func TestMergeWithExisting(t *testing.T) {
    // Test cases:
    // 1. Empty existing config — detected values win
    // 2. Existing verify commands — preserved over detection
    // 3. Existing services — preserved (always user-configured)
    // 4. Non-default provider timeouts — preserved
    // 5. Default provider timeouts — overwritten by detection
}
```

**Exit criteria:** `mergeWithExisting` has table-driven tests covering preserve vs. override semantics.

---

### Task 16: Add retry/stuck integration test scenarios

**Problem:** `TestPrepPlanExecLand` only covers the happy path (item passes on first try). Retry, stuck, and resume paths are untested at the integration level.

**Fix:** Add new test functions to `integration_test.go`:

1. **TestExecRetryThenPass** — mock provider returns STUCK on first attempt, DONE on second. Verify progress.jsonl contains item_stuck then item_done events.

2. **TestExecStuckSkip** — mock provider returns STUCK 3 times. Verify item is skipped (item_done with status "skipped").

These use the existing mock provider infrastructure (`testdata/mock-claude/main.go`) with sequential response configurations.

**Exit criteria:** Integration tests cover retry and skip flows with mock provider. `go test` passes.

---

### Task 17: Add TestStrictRendering for prompt templates

**Problem:** No test verifies that rendered templates have no leftover `{{...}}` patterns. Variable name typos or missing map entries would silently produce broken prompts.

**Fix:** Add to `prompts_test.go`:
```go
func TestStrictRendering(t *testing.T) {
    // For each template in prompts/:
    // 1. Extract all {{varName}} patterns via regex
    // 2. Build a map with dummy values for each
    // 3. Render via getPrompt
    // 4. Assert no {{ remains in output
    // 5. Assert template is referenced by at least one generate* function (not dead)
}
```

Use `promptFS` (the embedded filesystem) to iterate over all `.md` files.

**Exit criteria:** Every prompt template renders cleanly with all variables filled. No leftover `{{...}}` patterns.

---

### Task 18: Add TestLandFailPlanLoop integration test

**Problem:** The land-fail→replan cycle is the primary recovery mechanism but has zero end-to-end coverage.

**Fix:** Add `TestLandFailPlanLoop` to `integration_test.go`:

Mock provider sequence (8 steps):
1. Plan create → returns valid plan draft with DONE marker
2. Plan verify → returns VERIFY_PASS
3. Exec build → commits + returns DONE
4. Land analyze → returns VERIFY_FAIL (with findings)
5. Land fix → returns DONE (fix committed)
6. Land re-verify → commands pass
7. Land summary → returns summary between SUMMARY_START/SUMMARY_END
8. Verify progress.jsonl has land_failed then land_passed events

**Exit criteria:** Full land-fail-fix-pass cycle works with mock provider. progress.jsonl contains the expected event sequence.

---

## Implementation Contract

### What the agent should do per iteration:
1. Read this spec (`PHASE0-SPEC-refine-01.md`) to understand all tasks
2. Check which tasks are already complete (search codebase, run tests)
3. Pick the highest-priority incomplete task
4. Implement it fully — no stubs, no TODOs
5. Run `go build ./...`, `go vet ./...`, `go test ./...`
6. Commit with message: `scrip: <what was fixed/changed>`
7. Exit

### What the agent should NOT do:
- Modify `PHASE0-SPEC.md`, `PHASE0-SPEC-refine-01.md`, or `ROADMAP.md`
- Combine multiple tasks into one iteration (one task per commit)
- Add features not in this spec
- Refactor code beyond what the task requires

### Completion signal:
When ALL 18 tasks are complete — verified by `go build`, `go test`, and manual inspection of each task's exit criteria — create a file called `REFINE_01_COMPLETE` containing the current date and a summary of changes.
