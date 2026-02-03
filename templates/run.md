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
6. When complete, output exactly: {{doneMarker}}

## Verification

After you signal done, these commands will be run:

{{verifyCommands}}

Your story only passes if ALL verification commands succeed.

## Adding Learnings

If you discover important patterns or context that would help future work, output:
```
<ralph>LEARNING:description of the pattern or context</ralph>
```

{{learnings}}

## Important

- Do NOT modify prd.json - Ralph manages story state
- Output {{doneMarker}} only when you're confident the work is complete
- If you're stuck, still output {{doneMarker}} and let verification catch issues
