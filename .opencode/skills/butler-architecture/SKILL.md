---
name: butler-architecture
description: Apply Butler architecture boundaries, subsystem ownership, and spec-first change discipline.
---
## What this skill covers
- Butler core boundaries between orchestration, transport, sessions, memory, tools, credentials, and channels
- When a task becomes non-trivial and requires spec lookup before changes
- How to detect architecture drift, hidden coupling, or changes that need spec updates

## Use this skill when
- A change touches service boundaries, APIs, deployment topology, or subsystem ownership
- You need to decide whether a request fits the existing architecture or requires an ADR-level adjustment
- You need a fast architecture compliance pass before implementation or review

## Core rules
- Preserve Butler run/session truth inside Butler, not inside providers or transports
- Prefer incremental changes with explicit interfaces over sweeping abstraction changes
- If behavior conflicts with docs, align with docs or update docs in the same task

## Required references
- `docs/architecture/butler-prd-architecture.md`
- `docs/ai/engineering-rules.md`
