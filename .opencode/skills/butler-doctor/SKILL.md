---
name: butler-doctor
description: Apply Butler doctor and observability rules for safe diagnostics, config introspection, and operator guidance.
---
## What this skill covers
- Self-inspection and doctor responsibilities
- Configuration introspection requirements
- Observability, health checks, and operator-facing diagnostics
- Secret-safe diagnostic reporting

## Use this skill when
- A task changes doctor tools or reports
- A task changes config introspection or health checks
- A task changes observability expectations or operator UX

## Core rules
- Doctor should expose effective configuration and validation state without exposing secrets
- Diagnostics should be clear, actionable, and self-hosted friendly
- Reports should preserve masking and audit expectations

## Required references
- `docs/architecture/butler-prd-architecture.md`
- `docs/architecture/tool-runtime-adr.md`
- `docs/ai/engineering-rules.md`
