---
name: butler-transport
description: Apply Butler transport rules for WebSocket-first providers, streaming, and provider-normalized contracts.
---
## What this skill covers
- Model Transport Layer responsibilities
- WebSocket-first strategy where providers support it
- Provider-side session handling vs Butler-owned run/session truth
- Streaming, resume, cancel, and provider normalization behavior

## Use this skill when
- A task changes model provider integration
- A task changes streaming, resume, or cancellation behavior
- A task changes transport contracts or provider session references

## Core rules
- Transport is not orchestration
- Transport is not memory
- Provider-side session is an optimization, not authoritative Butler state
- Cloud and local providers must fit one logical transport contract

## Required references
- `docs/architecture/model-transport-contract.md`
- `docs/architecture/run-lifecycle-spec.md`
- `docs/architecture/butler-prd-architecture.md`
