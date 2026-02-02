# Ralph Verification

Perform comprehensive verification to ensure the branch is ready to merge.

## Project Settings

- **PRD Path:** `{{prdPath}}`
- **Project Root:** `{{projectRoot}}`

### Quality Commands

```bash
{{qualityCommands}}
```

## Step 1: Gather Context

1. Read `{{prdPath}}` — extract:
   - Branch name
   - All user stories and their acceptance criteria
   - `run.learnings[]` — patterns discovered
   - Which stories involve routes, UI, backend, database
2. Check `lastResult` for each story — any with incomplete summaries?
3. Get list of files changed: `git diff --name-only main..HEAD`

## Step 2: Automated Checks

Run all quality commands (must all pass):

```bash
{{qualityCommands}}
```

**If any fail:** Fix issues before proceeding.

## Step 3: Test Coverage Audit

For each file changed in the branch:

1. **Check if test file exists** following project conventions
2. **For new service functions:** Verify unit tests exist covering:
   - Happy path
   - Error cases
   - Edge cases
3. **For new routes:** Verify e2e tests exist
4. **Flag gaps:** List any code without adequate test coverage

**Action:** Write missing tests for critical paths.

## Step 4: Browser Verification (If Applicable)

For stories with UI changes:
1. Start dev server
2. Navigate to each new/modified route
3. Test interactive elements
4. Verify responsive layout

## Step 5: Oracle Review

Ask the oracle for a comprehensive review:

```
Perform a comprehensive review of the implementation.

## Context
- Branch: {branch-name from prd.json}
- PRD: [attach prd.json]
- Changed files: [list from git diff]

## Review Tasks
1. PRD Compliance: Verify ALL acceptance criteria are met
2. Security: Input validation, no exposed secrets, authorization checks
3. Code Quality: Follows patterns, no dead code, error handling
4. Missing Pieces: Anything skipped?
```

**Action:** Fix every issue identified.

## Step 6: Learnings Check

Verify important discoveries are saved:
1. Check `run.learnings[]` is populated
2. If patterns should be permanent, ensure they're in AGENTS.md

## Step 7: Reset Failed Stories (If Needed)

When issues require reimplementation:

1. **For each affected story, update prd.json:**
   ```json
   {
     "passes": false,
     "retries": <current + 1>,
     "lastResult": null,
     "notes": "<what needs fixing>"
   }
   ```

2. **Report:**
   ```
   Reset for retry:
   - US-005: Missing test coverage
   - US-012: Security issue
   
   Run 'ralph run' to fix these stories.
   ```

**Important:** Only reset stories that genuinely need reimplementation.

## Step 8: Final Report

Generate:

```markdown
## Ralph Verification Report

**Branch:** {branch-name}
**Date:** {date}

### Automated Checks
- [x] All quality commands pass

### Test Coverage
- [x] New code has tests
- [x] No critical gaps

### Browser Verification
- [x] UI changes verified (or N/A)

### Oracle Review
- [x] All acceptance criteria met
- [x] No security issues
- [x] Code quality approved

### Stories Reset for Retry
- None (or list)

### Result: ✅ READY TO MERGE
```

## Completion

**If all checks pass:**
```
Ready to merge. Options:
1. git checkout main && git merge {branch-name}
2. Create PR for team review
```

**If issues found:**
```
Stories reset in prd.json:
- [list stories]

Run 'ralph run' to fix, then 'ralph verify' again.
```
