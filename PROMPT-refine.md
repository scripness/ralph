# scrip v1 — Refinement Prompt

You are refining the scrip v1 CLI. The core implementation is complete — all 6 sessions pass, build/test/vet clean. This loop fixes verified gaps between the spec and the implementation.

This is an autonomous loop — each iteration you fix ONE task, verify it, commit, and exit. The next iteration starts with fresh context.

## Phase 0a: Orient — Study the refinement spec

Study `PHASE0-SPEC-refine-01.md` using up to 500 parallel Sonnet subagents. This contains 18 verified tasks organized by priority tier. Each task has the exact problem, fix instructions, affected files, and exit criteria.

Also study `PHASE0-SPEC.md` for design context when needed — it is the original specification the tasks reference.

## Phase 0b: Orient — Study the current codebase

Study the relevant source files using up to 500 parallel Sonnet subagents. Focus on the files listed in each task's scope. Understand the current state before making changes.

## Phase 0c: Orient — Determine what's next

Check which tasks from `PHASE0-SPEC-refine-01.md` are already complete:

1. For each task, check if the fix described has already been applied
2. Run `go build ./...` and `go test ./...` to confirm the build is green
3. Pick the highest-priority incomplete task (work top-to-bottom by tier)

**One task per iteration.** A "task" is one numbered item from the spec. Some tasks are small (5 LOC), some are larger (200 LOC). Either way, complete the entire task in one iteration.

## Phase 1: Implement

Before making changes, search the codebase — don't assume not implemented. Use up to 500 parallel Sonnet subagents for searches/reads. Use Opus subagents when complex reasoning is needed. Ultrathink.

Implement the task's fix COMPLETELY following the spec's instructions. The spec provides exact code locations, fix strategies, and constraints.

## Phase 2: Test

Run verification using only 1 subagent for build/tests:

```bash
go build ./...
go vet ./...
go test ./...
```

If the task adds new functionality, add tests. If it modifies existing behavior, update tests. The build must be green after every commit.

## Phase 3: Verify against exit criteria

After tests pass, verify your implementation matches the task's exit criteria from `PHASE0-SPEC-refine-01.md`. Use Sonnet subagents to cross-check.

## Phase 4: Commit

When everything passes:

```bash
git add <specific files you changed>
git commit -m "scrip: <what you fixed/changed>"
```

Commit message should describe WHAT was done, not the task number.

## Guardrails

99999. Important: ONE task per iteration. Complete the entire task, verify, commit, exit. The loop handles iteration.

999999. Important: Don't assume not implemented. Check if a previous iteration already applied the fix.

9999999. Important: Follow the fix instructions in PHASE0-SPEC-refine-01.md closely. Each task was verified by adversarial agents — the problem descriptions are accurate and the fix strategies are validated.

99999999. Important: Do NOT modify PHASE0-SPEC.md, PHASE0-SPEC-refine-01.md, or ROADMAP.md. These are your specification — read them, don't write them.

999999999. Important: If tests unrelated to your current task fail, fix them. The build must be green after every commit.

9999999999. Important: Capture insights via LEARNING markers. Good: "Task 12 Story→Item rename: logger.go Event.StoryID has json tag 'story' — kept for wire format compat." Bad: "I renamed things."

99999999999. Important: When you finish your task and commit, EXIT cleanly. Do not start the next task.

999999999999. Important: If ALL 18 tasks are complete — verified by build, tests, and each task's exit criteria — create a file called `REFINE_01_COMPLETE` containing the current date and a one-line summary. This signals the loop to stop.

## Learnings from previous iterations

If a file called `LEARNINGS.md` exists in the project root, study it. It contains insights from the original implementation loop and from previous refinement iterations.

When you emit `<scrip>LEARNING:text</scrip>` markers, the loop script appends them to `LEARNINGS.md` for future iterations.

## Completion signal

When done with your single task:
```
<scrip>DONE</scrip>
```

If stuck and cannot proceed:
```
<scrip>STUCK:reason</scrip>
```

To record an insight for future iterations:
```
<scrip>LEARNING:text</scrip>
```
