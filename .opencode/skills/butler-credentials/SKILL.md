---
name: butler-credentials
description: Apply Butler credential and secret-resolution rules for alias-based auth, approvals, and audit-safe execution.
---
## What this skill covers
- Credential aliases, credential records, and secret storage boundaries
- Deferred secret resolution via `credential_ref`
- Approval policy, allowed domains, and tool restrictions
- Audit logging and secret masking requirements

## Use this skill when
- A task changes auth flows, credential handling, or approval logic
- A task adds secret-backed tool inputs
- A task changes audit behavior for sensitive actions

## Core rules
- Raw secrets never enter model-visible context
- Secret resolution happens only in system/runtime layers
- Tool Broker and Credential Broker enforce policy before execution
- Logs and outputs must not reveal secret values

## Required references
- `docs/architecture/credential-management.md`
- `docs/architecture/tool-runtime-adr.md`
- `docs/ai/engineering-rules.md`
