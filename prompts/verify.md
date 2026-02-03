# Final Verification

All stories have been implemented. Perform a comprehensive review to verify the feature is complete and working correctly.

## Project

**Name:** {{project}}
**Description:** {{description}}

## Stories Implemented

{{storySummaries}}

## Verification Commands

The CLI has run (or will run) these commands:

{{verifyCommands}}

{{learnings}}

## Your Task

Review the implementation thoroughly. This is a **report-only verification phase** — you should NOT modify code during this phase.

### Review Checklist

1. **Test Coverage**: Are all new functions/routes tested?
2. **Acceptance Criteria**: Does each story meet ALL its criteria?
3. **Code Quality**: Are patterns consistent with the codebase?
4. **Integration**: Do the stories work together as a complete feature?
5. **Edge Cases**: Are error conditions handled?
6. **Missing Pieces**: Is anything incomplete or skipped?

### Run Verification

If you need to verify behavior:
- Run the verification commands listed above
- Check test output for failures
- Review any error messages carefully

## Response Options

### If everything is complete and verified
```
<ralph>VERIFIED</ralph>
```
Use this ONLY when:
- All verification commands pass
- All acceptance criteria are met
- The feature works as intended
- Code quality is acceptable

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

Before completing verification:

1. Check that `{{knowledgeFile}}` files in affected directories contain relevant patterns
2. If important discoveries were made during implementation, ensure they are documented
3. List any patterns that the user should review:

```
<ralph>LEARNING:pattern or convention discovered</ralph>
```

## Important

- **Do NOT modify code** during verification — only report status
- **Be thorough** — a false VERIFIED wastes time when issues surface later
- **Be specific** in REASON — vague reasons make fixes harder
- **Reset is okay** — it's better to reset than to mark broken code as verified
