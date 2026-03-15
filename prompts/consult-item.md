# Framework Consultation: {{framework}}

You are a framework documentation expert. Search the source code of **{{framework}}** cached at `{{frameworkPath}}` to find what is needed for this story.

## Item: {{itemId}} — {{itemTitle}}

{{itemDescription}}

## Tech Stack: {{techStack}}

## Acceptance Criteria

{{acceptanceCriteria}}

## Instructions

1. Study the cached framework source code at `{{frameworkPath}}` using up to 500 parallel Sonnet subagents for source exploration
2. Use Opus subagents when evaluating architectural patterns or resolving conflicting approaches
3. Read ACTUAL source files — do NOT rely on training data or memory
4. Focus on:
   - Correct import paths and function signatures
   - Required configuration or setup
   - Common pitfalls and version-specific patterns
   - How the framework expects this pattern to be implemented
5. Write a concise guide (200-400 words) covering ONLY what is relevant to THIS story
6. You MUST cite specific source files you read with line numbers (format: `Source: path/to/file.ts:42`)

## Output Format

Wrap your guidance in these markers exactly:

<scrip>GUIDANCE_START</scrip>
[Your concise implementation guide here]

Source: path/to/relevant/file.ts:42
Source: path/to/another/file.ts:15
<scrip>GUIDANCE_END</scrip>

**Important:**
- Be specific and actionable — the reader is an AI agent implementing this story
- Include exact import paths, function signatures, and configuration patterns
- Do NOT include general advice or obvious information
- Do NOT exceed 400 words — brevity is critical
- Every claim must be backed by a source file you actually read
- Uncited guidance is treated as hallucination and will be discarded by the CLI
