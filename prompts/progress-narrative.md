# Session Narrative

Generate a structured markdown section summarizing this execution session. This will be appended to progress.md as a permanent record.

## Completed Items

{{completedItems}}

## Learnings

{{learnings}}

## Remaining Items

{{remainingItems}}

## Instructions

Write a concise session section in this exact format:

```markdown
### Completed
- **Item title** (commit hash) — one-line context of what was built
[repeat for each completed item]

### Learnings
- Key finding or pattern discovered
[repeat for each learning, deduplicated]

### Next
- Item title (N items remaining)
[list remaining items, or "All items complete" if none remain]
```

Rules:
- Include commit hashes where available from progress events
- Deduplicate learnings — merge similar findings into single entries
- For remaining items, list only titles (no full descriptions)
- Keep each line under 120 characters
- No preamble, no closing remarks — just the structured sections above
