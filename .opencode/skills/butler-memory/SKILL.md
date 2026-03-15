---
name: butler-memory
description: Apply Butler memory rules for durable truth, retrieval-only vector search, and memory-class separation.
---
## What this skill covers
- Transcript Store, Working Memory, Episodic Memory, and Profile Memory boundaries
- Durable vs transient state placement across PostgreSQL and Redis
- pgvector as retrieval-only infrastructure
- Retrieval, provenance, and memory pipeline behavior

## Use this skill when
- A task changes memory persistence, retrieval, summaries, or extraction
- A task affects PostgreSQL/Redis/pgvector roles in memory behavior
- A task risks blurring memory classes or source-of-truth rules

## Core rules
- PostgreSQL is durable truth
- pgvector is retrieval only
- Redis is transient only
- Keep durable working snapshots separate from transient execution state

## Required references
- `docs/architecture/memory-model.md`
- `docs/architecture/butler-prd-architecture.md`
- `docs/ai/engineering-rules.md`
