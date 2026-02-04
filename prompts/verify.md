# Final Verification

All stories have been implemented. Perform a comprehensive review to verify the feature is complete and working correctly.

## Project

**Name:** {{project}}
**Description:** {{description}}
**Branch:** {{branchName}}
{{serviceURLs}}

## Stories Implemented

{{storySummaries}}

> For complete story details (descriptions, browser steps), see {{prdPath}}.

## Verification Results

{{verifySummary}}

### Commands Run

{{verifyCommands}}

If configured, the CLI has also run browser verification and service health checks. Results are in the summary above.

{{learnings}}

## Your Task

Review the implementation thoroughly. The CLI has already executed verification commands and the results are summarized above. You may re-run specific commands if needed for deeper investigation, but do NOT modify any code.

{{diffSummary}}

### Acceptance Criteria Checklist

You MUST verify every criterion below. Check each one against the actual implementation. Any unchecked item is a RESET for that story.

{{criteriaChecklist}}

### Review Checklist

Every item below must pass. If ANY item fails, you MUST RESET the affected stories.

1. **Test Coverage**: Does every new function/route/behavior have tests? Untested code is a RESET.
2. **Acceptance Criteria**: Confirm every checkbox above is satisfied. Missing criteria = RESET.
3. **Code Quality**: Are patterns consistent with the codebase?
4. **Integration**: Do the stories work together as a complete feature?
5. **Edge Cases**: Are error conditions handled?
6. **Documentation**: Has `{{knowledgeFile}}` been updated with patterns discovered during implementation? New conventions or gotchas left undocumented is a RESET.
7. **Missing Pieces**: Is anything incomplete or skipped?

{{btcaInstructions}}

### Review Verification Output

Review the verification results summary above:
- Any FAIL line means that check did not pass — investigate before marking VERIFIED
- FAIL lines include the command output (last 50 lines) — read it to understand what failed
- Any WARN line indicates a potential issue that should be investigated
- Check test output for failures
- Review any error messages carefully
- Note any warnings that could indicate latent issues

## Response Options

### If everything is complete and verified
```
<ralph>VERIFIED</ralph>
```
Use this ONLY when ALL of the following are true:
- All verification commands pass (no test failures, no lint errors)
- All acceptance criteria are met for every story
- Every new function/route/behavior has test coverage
- `{{knowledgeFile}}` has been updated with relevant patterns
- Code quality is consistent with existing codebase patterns

### If you find issues that need rework
```
<ralph>RESET:US-001,US-003</ralph>
<ralph>REASON:US-001 missing test coverage, US-003 form validation incomplete</ralph>
```
Use this when stories need reimplementation. The CLI will:
- Mark those stories as needing rework
- Re-run the implementation loop for them

### If a story is fundamentally broken (can't be fixed by retry)
```
<ralph>BLOCK:US-007</ralph>
<ralph>REASON:Requires API that doesn't exist yet</ralph>
```
Use this when a story cannot be completed due to external factors.

## Knowledge Preservation

Before completing verification, check documentation completeness:

1. Verify that `{{knowledgeFile}}` files in affected directories have been updated with relevant patterns. If they haven't, RESET the stories that failed to document.
2. Verify that new conventions, gotchas, or integration patterns are recorded — not just code, but the knowledge needed to maintain it.
3. List any patterns that the user should review:

```
<ralph>LEARNING:pattern or convention discovered</ralph>
```

## Important

- **Do NOT modify code** during verification — only report status
- **Be thorough** — a false VERIFIED wastes time when issues surface later
- **Be specific** in REASON — vague reasons make fixes harder
- **Reset is okay** — it's better to reset than to mark broken code as verified
