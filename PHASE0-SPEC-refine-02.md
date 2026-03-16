# Phase 0 Refinement Specification (Round 02)

**Purpose:** Fix CodeRabbit-verified bugs and address codebase organization smells identified by deep consultation analysis. No roadmap phase changes — those are deferred to refine-03 after brainstorming.

**Source of truth:** PHASE0-SPEC.md remains the design document. This refinement addresses 6 verified issues.

---

## Task Inventory

6 tasks across 3 groups. No inter-task dependencies. Work top-to-bottom.

| # | Task | Group | Files | Est. LOC |
|---|------|-------|-------|----------|
| 1 | Handle markerless analysis output | CodeRabbit | cmd_land.go, cmd_plan.go, cmd_exec.go | ~30 |
| 2 | Make exec-verify opt-in, default off | CodeRabbit | cmd_exec.go, config.go, config_test.go | ~25 |
| 3 | Add HasNewCommitSince check in land fix loop | CodeRabbit | cmd_land.go | ~15 |
| 4 | Rename schema.go to itemstate.go | Codebase | schema.go, schema_test.go | ~0 (rename) |
| 5 | Merge resourcereg.go into resources.go | Codebase | resourcereg.go, resources.go, resourcereg_test.go, resources_test.go | ~0 (merge) |
| 6 | Extract shared provider.go from cmd_exec.go | Codebase | cmd_exec.go, cmd_land.go, cmd_plan.go, provider.go (new) | ~60 |

---

## Group 1: CodeRabbit Fixes

### Task 1: Handle markerless analysis output explicitly

**Problem:** When AI analysis output contains no VERIFY_PASS or VERIFY_FAIL markers, three code paths produce confusing behavior:

1. **`landParseAnalysis`** (`cmd_land.go:364-385`) returns `passed=false, failures=nil`. The caller prints "Analysis found 0 issue(s):" and enters the fix loop with an empty findings list. Three fix attempts are wasted with blank prompts.

2. **`parsePlanVerifyOutput`** (`cmd_plan.go:402-414`) returns `Warnings: nil`. The caller prints "Verification passed." — silently treating markerless output as success.

3. **`cmd_exec.go:563`** reuses `landParseAnalysis`. When `deepPassed=false` with empty failures, the retry reason is "AI deep analysis: " (empty join).

**Fix for `landParseAnalysis`:** When output is non-empty but has no markers, return a synthetic failure:

```go
if !passed && len(failures) == 0 && result.Output != "" {
    failures = append(failures, "Analysis produced output but no VERIFY_PASS/VERIFY_FAIL markers — provider may have truncated or failed to follow instructions")
}
```

**Fix for `parsePlanVerifyOutput`:** When output is non-empty but has no markers, add a warning:

```go
if len(v.Warnings) == 0 && output != "" {
    // Check if VERIFY_PASS was found
    if !passed {
        v.Warnings = append(v.Warnings, "Verification produced output but no markers — treat as inconclusive")
    }
}
```

This requires tracking `passed` in the function (currently not tracked — only failures are accumulated).

**Fix for exec-verify path:** Falls out naturally from the `landParseAnalysis` fix since it reuses that function.

**Exit criteria:**
- Markerless land analysis output produces a descriptive failure reason (not "0 issue(s)")
- Markerless plan verification output produces a warning (not "Verification passed")
- Tests cover the markerless case for both functions

---

### Task 2: Make exec-verify opt-in, default off

**Problem:** AI deep verification after DONE in `scrip exec` (`cmd_exec.go:554-588`) runs always-on, doubling provider cost. For a 10-item plan, this adds 10 extra Opus calls. The landing analysis (`scrip land`) is a strict superset — it traces every criterion through the full diff, plus cross-item consistency and security auditing. The refinement spec itself (PHASE0-SPEC-refine-01.md Task 8) recommended making this opt-in.

**Fix:**

1. Add `DeepVerify bool` to `ScripVerifyConfig` in `config.go`:
```go
type ScripVerifyConfig struct {
    Typecheck  string `json:"typecheck,omitempty"`
    Lint       string `json:"lint,omitempty"`
    Test       string `json:"test"`
    DeepVerify bool   `json:"deepVerify,omitempty"`
}
```

2. In `cmd_exec.go`, wrap the AI deep analysis block (lines 554-588) with:
```go
if cfg.Config.Verify.DeepVerify {
    // existing AI deep analysis code
}
```

3. Default is `false` (omitempty means absent from config.json = off).

4. Update `config_test.go` to test the new field.

**Do NOT remove** the exec-verify code or the `generateExecVerifyPrompt` function or the `exec-verify.md` template. They remain available for users who opt in.

**Exit criteria:** AI deep verification is off by default. Setting `"deepVerify": true` in config.json enables it. Tests pass. The exec-verify.md template and generateExecVerifyPrompt remain in the codebase.

---

### Task 3: Add HasNewCommitSince check in land fix loop

**Problem:** After the fix agent signals DONE in the land fix loop (`cmd_land.go:194-213`), there is no check that the fix agent actually committed anything. If the initial failure was from AI analysis (not mechanical verification), and mechanical verification was already passing, the fix agent can say DONE without committing, and the feature lands with unfixed AI-identified issues.

Compare with `cmd_exec.go:501-520` which HAS this check after DONE.

**Fix:** In `cmd_land.go`, after checking `fixResult.Done` (around line 194), add:

```go
if fixResult.Done {
    // Verify the fix agent actually committed something
    if !git.HasNewCommitSince(preFixCommit) {
        fmt.Println("\n  ! Fix agent signaled DONE but made no new commit.")
        reason := "Fix agent claimed completion without committing changes"
        _ = AppendProgressEvent(progressPath, &ProgressEvent{
            Event:  ProgressItemStuck,
            Item:   "landing-fix",
            Reason: reason,
        })
        continue // retry fix
    }
}
```

This requires capturing `preFixCommit` via `git.GetLastCommit()` before spawning the fix agent (add around line 170).

**Exit criteria:** A fix agent that signals DONE without committing does NOT cause the landing to succeed. The fix loop continues to the next attempt. Tests cover this case.

---

## Group 2: Codebase Organization

### Task 4: Rename schema.go to itemstate.go

**Problem:** `schema.go` is misleadingly named. It contains `ItemState`, `ComputeItemState`, `GetNextItem`, `AllItemsComplete`, `CountItemsPassed`, `CountItemsSkipped`, `CollectLearnings`, `normalizeLearning`, `resolveItemRef`, `depsResolved` — all item state computation, not schema definitions.

**Fix:**
```bash
git mv schema.go itemstate.go
git mv schema_test.go itemstate_test.go
```

No code changes needed. Just rename the files.

**Exit criteria:** `schema.go` and `schema_test.go` no longer exist. `itemstate.go` and `itemstate_test.go` contain the same code. Build and tests pass.

---

### Task 5: Merge resourcereg.go into resources.go

**Problem:** `resourcereg.go` is 9 lines containing only the `Resource` struct type. It exists as a separate file for no clear reason. Its only consumer is `resources.go`.

**Fix:** Move the `Resource` struct from `resourcereg.go` into `resources.go` (place it near the top, before `ResourceManager`). Move any test code from `resourcereg_test.go` into `resources_test.go`. Delete the empty files.

**Critical constraint:** `resourcereg_test.go` tests `ResourceRegistry` functionality (save/load, URL cache, CachedRepo versioning) — these tests belong with `resourceregistry.go`, NOT with `resources.go`. Only move tests that actually test the `Resource` struct. If all tests in `resourcereg_test.go` test registry functionality, move them to `resourceregistry_test.go` instead (create if needed, or the file may already exist as `resourcereg_test.go` was testing registry types).

Read both test files carefully before moving code. The file names are confusing:
- `resourcereg.go` = the `Resource` struct (9 lines)
- `resourceregistry.go` = the `ResourceRegistry` type (160 lines)
- `resourcereg_test.go` = tests that may test EITHER or BOTH

**Exit criteria:** `resourcereg.go` deleted. `Resource` struct lives in `resources.go`. All tests pass. No test code lost.

---

### Task 6: Extract shared provider.go from cross-command code

**Problem:** `cmd_land.go` directly calls functions defined in `cmd_exec.go` (`scripSpawnProvider`, `scripRunVerify`) and uses regex vars from `cmd_plan.go` (`scripVerifyPassRe`, `scripVerifyFailRe`). This creates cross-command coupling where command files depend on each other's internals.

Git co-change analysis confirms `cmd_exec.go` + `cmd_land.go` co-change 3 times — they share subprocess/verification patterns but should be peers, not parent-child.

**Fix:** Create `provider.go` containing the shared code extracted from cmd_exec.go and cmd_plan.go:

1. **Move from `cmd_exec.go` to `provider.go`:**
   - `scripSpawnProvider` function (the provider subprocess spawner)
   - `scripProcessLine` function (marker detection per line)
   - `scripRunVerify` function (mechanical verification runner)
   - `ProviderResult`-related constants: `ScripDoneMarker`, `ScripStuckMarker`, `scripLearningRe`, `scripStuckNoteRe`
   - `scripMaxRetries` constant

2. **Move from `cmd_plan.go` to `provider.go`:**
   - `scripVerifyPassRe`, `scripVerifyFailRe` regex vars

3. **Move from `cmd_land.go` to `provider.go`:**
   - `landParseAnalysis` function (used by both cmd_land.go and cmd_exec.go)

4. **Keep `ProviderResult` and `ItemVerifyResult` types in `loop.go`** — they are already there and have the right abstraction level.

**Do NOT move** command-specific logic (the exec loop, the land pipeline, the plan rounds). Only move shared infrastructure that multiple commands depend on.

**Do NOT create a new package.** This stays in `package main`. It is purely a file organization change — extracting shared code into its own file for clarity.

**Exit criteria:** `cmd_land.go` no longer imports from `cmd_exec.go` or `cmd_plan.go`. All three command files import shared code from `provider.go`. Build and tests pass. No behavioral changes.

---

## Implementation Contract

### Per iteration:
1. Read this spec to understand all tasks
2. Check which tasks are already complete
3. Pick the highest-priority incomplete task
4. Implement it fully
5. Run `go build ./...`, `go vet ./...`, `go test ./...`
6. Commit with message: `scrip: <what was fixed/changed>`
7. Exit

### Rules:
- One task per iteration
- Do NOT modify PHASE0-SPEC.md, PHASE0-SPEC-refine-01.md, PHASE0-SPEC-refine-02.md, or ROADMAP.md
- Do NOT add features not in this spec
- Do NOT refactor beyond what the task requires

### Completion signal:
When ALL 6 tasks are complete, create `REFINE_02_COMPLETE` containing the current date and a summary.
