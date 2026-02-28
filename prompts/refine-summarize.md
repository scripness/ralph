# Post-Refine Summary

Generate a technical summary of changes from this refine session for **{{feature}}**. This summary is consumed by AI coding agents — optimize for machine utility, not human readability.

## Git Log (commits from this session)

```
{{gitLog}}
```

## Diff Statistics

```
{{diffStat}}
```

## Previous Summary

{{previousSummary}}

## Instructions

Write a 100-200 word technical summary. Include:
- File paths and function names for everything changed
- New APIs, schemas, or components introduced
- Integration points modified (what connects to what)
- Patterns established or changed that constrain future work
- Gotchas or workarounds discovered

Do NOT write narrative prose. Every sentence should contain a file path, function name, or concrete technical detail.

Output your summary between these markers:

```
<ralph>SUMMARY_START</ralph>

## {{feature}} — refine ({{timestamp}})

[Your summary here]

<ralph>SUMMARY_END</ralph>
```
