# Story Implementation

## Your Task

Implement the following story:

**ID:** {{storyId}}
**Title:** {{storyTitle}}
**Description:** {{storyDescription}}
{{tags}}
{{retryInfo}}

## Acceptance Criteria

{{acceptanceCriteria}}

## Instructions

1. Read the codebase to understand context and existing patterns
2. Implement the story following existing code conventions
3. Write tests for your implementation
4. If this is a UI story (tagged "ui"):
   - Write e2e tests that verify the UI works
   - Tests should cover the acceptance criteria
5. Commit your changes with message: `feat: {{storyId}} - {{storyTitle}}`
6. Signal completion using the appropriate marker (see below)

## Verification

After you signal DONE, these commands will be run:

{{verifyCommands}}

Your story only passes if ALL verification commands succeed.

## Signals

Use these markers to communicate with Ralph:

### When implementation is complete
```
<ralph>DONE</ralph>
```
Only output this when you're confident the work is complete and tests pass.

### When you're stuck and need help
```
<ralph>STUCK</ralph>
<ralph>REASON:description of what's blocking you</ralph>
```
Use this when you can't proceed - don't just output DONE and hope verification catches it.

### When a story is impossible or underspecified
```
<ralph>BLOCK:US-XXX</ralph>
<ralph>REASON:why this story cannot be implemented</ralph>
```
Use this to mark a story as blocked (e.g., depends on unfinished work, missing requirements).

### When you discover important patterns
```
<ralph>LEARNING:description of the pattern or context</ralph>
```
These are saved for future iterations.

### When you think a different story should be next (advisory)
```
<ralph>SUGGEST_NEXT:US-XXX</ralph>
<ralph>REASON:why this order would be better</ralph>
```
Ralph may honor this suggestion if the story is valid.

{{learnings}}

## Important

- Do NOT modify prd.json - Ralph manages story state
- Output DONE only when you're confident the work is complete
- Use STUCK if you can't proceed - this preserves retries for real attempts
- Use BLOCK if the story itself is problematic (missing deps, unclear requirements)
