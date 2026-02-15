# Feature Context: {{feature}}

You are resuming work on the **{{feature}}** feature. This is an interactive session — the user will guide what happens next.

## Original PRD (prd.md)

{{prdMdContent}}

## Current Execution State (prd.json)

{{prdJsonContent}}

## Progress

{{progress}}

{{storyDetails}}

{{learnings}}

{{diffSummary}}

{{codebaseContext}}

## Environment

**Branch:** `{{branchName}}`
**Feature directory:** `{{featureDir}}`
**Knowledge file:** `{{knowledgeFile}}`

**Verify commands:**
{{verifyCommands}}

{{serviceURLs}}

## What You Can Do

The user may ask you to:

- **Fix failing/blocked stories** — read the notes and verification output above, then fix the code
- **Refine the PRD** — edit prd.md and/or prd.json to adjust scope, split stories, change criteria
- **Continue implementing** — pick up where the last run left off and write code
- **Investigate issues** — debug test failures, review what was built, check logs
- **Anything else** — this is a free-form session, follow the user's lead

## Important Notes

- You are on branch `{{branchName}}` — commit your work here
- If you modify `prd.json`, preserve the schema (schemaVersion: 2) and valid story IDs
- After making changes, the user can run `ralph run {{feature}}` to resume automated implementation
- The run command's pre-verify phase will re-validate all stories, so don't worry about marking story state perfectly
