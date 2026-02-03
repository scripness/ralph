# Final Verification

All stories have been implemented. Perform a comprehensive review.

## Project

**Name:** {{project}}
**Description:** {{description}}

## Stories Implemented

{{storySummaries}}

## Verification Commands

These commands should pass:

{{verifyCommands}}

{{learnings}}

## Review Checklist

1. **Test Coverage**: Are all new functions/routes tested?
2. **Acceptance Criteria**: Does each story meet ALL criteria?
3. **Code Quality**: Are patterns consistent with codebase?
4. **Missing Pieces**: Anything incomplete or skipped?

## Your Task

Review the implementation thoroughly. Run the verification commands if needed.

### If you find issues that need rework

Output the story IDs that need reimplementation:
```
<ralph>RESET:US-001,US-003</ralph>
<ralph>REASON:US-001 missing test coverage, US-003 form validation incomplete</ralph>
```

### If a story is fundamentally broken (can't be fixed by retry)

Mark it as blocked:
```
<ralph>BLOCK:US-007</ralph>
<ralph>REASON:Requires API that doesn't exist yet</ralph>
```

### If everything is complete and verified

```
<ralph>VERIFIED</ralph>
```

## Learnings for AGENTS.md

If there are important patterns discovered during this implementation that should be documented in AGENTS.md or similar project documentation, list them here for the user to review.
