# PRD to prd.json Converter

Convert the following PRD document to Ralph's prd.json v2 format.

## PRD Content

```markdown
{{prdContent}}
```

## Output Format (v2 Schema)

Create `{{outputPath}}` with this exact structure:

```json
{
  "schemaVersion": 2,
  "project": "[Project name from PRD or directory]",
  "branchName": "ralph/[feature-name-kebab-case]",
  "description": "[Feature description from PRD title/intro]",
  "run": {
    "startedAt": null,
    "currentStoryId": null,
    "learnings": []
  },
  "userStories": [
    {
      "id": "US-001",
      "title": "[Story title]",
      "description": "As a [user], I want [feature] so that [benefit]",
      "acceptanceCriteria": [
        "Criterion 1",
        "Criterion 2",
        "Typecheck passes"
      ],
      "priority": 1,
      "passes": false,
      "retries": 0,
      "lastResult": null,
      "notes": ""
    }
  ]
}
```

## Conversion Rules

1. **Each user story → one JSON entry**
2. **IDs:** Sequential (US-001, US-002, etc.)
3. **Priority:** Based on dependency order (schema → backend → UI)
4. **All stories:** `passes: false`, `retries: 0`, `lastResult: null`, empty `notes`
5. **branchName:** Derive from feature name, kebab-case, prefixed with `ralph/`
6. **Always add:** "Typecheck passes" to every story's acceptance criteria
7. **UI stories:** Add "Verify in browser" criterion

## Story Size Validation

Before saving, verify EACH story:
- [ ] Can be described in 2-3 sentences
- [ ] Touches 5 or fewer files
- [ ] Has 3-6 acceptance criteria (not 10+)
- [ ] No "and" joining unrelated work

If any story fails these checks, SPLIT IT before saving.

## Story Ordering

Stories execute in priority order. Earlier stories must not depend on later ones.

**Correct order:**
1. Schema/database changes (migrations)
2. Server actions / backend logic
3. UI components that use the backend
4. Dashboard/summary views

## Acceptance Criteria Quality

Each criterion must be VERIFIABLE, not vague.

**Good:** "Add `status` column with default 'pending'"
**Bad:** "Works correctly"

**Good:** "Filter dropdown has options: All, Active, Completed"
**Bad:** "Good UX"

## Output

Save the JSON to: `{{outputPath}}`

After saving, tell the user:
```
prd.json created! Run 'ralph run' to start implementing stories.
```
