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

## Verification Commands

The CLI has already run these commands:

{{verifyCommands}}

If configured, the CLI has also run browser verification for UI stories and checked that managed services are still responding. Any issues are shown in the output above.

{{learnings}}

## Your Task

Review the implementation thoroughly. The CLI has already executed verification commands above. You may re-run specific commands if needed for deeper investigation, but do NOT modify any code. Use `git diff main...HEAD` to review the full scope of changes on this branch.

### Review Checklist

Every item below must pass. If ANY item fails, you MUST RESET the affected stories.

1. **Test Coverage**: Does every new function/route/behavior have tests? Untested code is a RESET.
2. **Acceptance Criteria**: Does each story meet ALL its criteria?
3. **Code Quality**: Are patterns consistent with the codebase?
4. **Integration**: Do the stories work together as a complete feature?
5. **Edge Cases**: Are error conditions handled?
6. **Documentation**: Has `{{knowledgeFile}}` been updated with patterns discovered during implementation? New conventions or gotchas left undocumented is a RESET.
7. **Missing Pieces**: Is anything incomplete or skipped?

### Review Verification Output

Review the output from the verification commands above:
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
