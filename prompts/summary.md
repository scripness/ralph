# Feature Summary Generation

You are generating a technical summary of a completed feature. This summary is the permanent record — plan files are purged after landing. The summary must preserve all information an agent needs to understand what was built and how.

## Feature

{{feature}}

## Progress Events

{{progressEvents}}

## Diff

{{diff}}

## Learnings

{{learnings}}

## Instructions

This summary is consumed by AI coding agents, not humans. Optimize for machine utility.

Use 2-4 subagents to review the progress events, implementation files, and diff in parallel. Extract concrete technical details — file paths, function names, data models, API endpoints, schemas, component hierarchies.

Write a dense technical summary (300-600 words). The summary MUST include these sections:

- **Implementation map**: Key files created/modified with their purpose (e.g., `src/auth/login.ts — LoginForm component using react-hook-form, validates email+password, calls POST /api/auth/login`)
- **Data models & APIs**: Any schemas, endpoints, database tables, or state shapes introduced
- **Patterns & conventions**: Architectural decisions that constrain future work (e.g., "All API routes use middleware chain: auth → validate → handler → serialize")
- **Integration points**: How this feature connects to the rest of the codebase — imports, shared state, event buses, config dependencies
- **Gotchas**: Non-obvious behaviors, workarounds, or constraints discovered during implementation (from learnings and retry history)
- **Skipped items**: What and why (if any items were skipped)

Every sentence must contain a file path, function name, or concrete technical detail. Do NOT write narrative prose. Use terse technical notation.

## Output

Output your summary between these markers:

```
<scrip>SUMMARY_START</scrip>
[your summary here]
<scrip>SUMMARY_END</scrip>
```
