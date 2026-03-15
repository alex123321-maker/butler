# Butler Engineering Rules

## 1. Purpose

This document defines the engineering baseline for Butler.
It is intended primarily for AI agents working with the Butler repository.

The goals are:
- consistent implementation decisions
- minimal dependency footprint
- stable architecture boundaries
- predictable code and documentation updates

---

## 2. Technology baseline

### Backend
- Primary language: Go
- Python: exception-only, must be justified

### Frontend
- Vue
- Nuxt.js

### APIs
- External / user-facing APIs: REST
- Internal service-to-service contracts: gRPC

### Storage
- PostgreSQL: durable system of record
- pgvector: semantic retrieval layer
- Redis: transient state, locks, queues, short-lived execution state

### Deployment
- Docker Compose is the baseline and primary deployment model

---

## 3. Dependency policy

### General rule
Use the smallest reasonable dependency set.

Before adding any dependency, verify:
1. standard library is not sufficient
2. the functionality is not already present in the repo
3. the new dependency materially improves implementation quality
4. the long-term maintenance cost is acceptable

### Prefer
- standard library
- mature, boring, well-supported libraries
- libraries with small dependency trees
- libraries that fit self-hosted Docker deployments

### Avoid
- large frameworks without clear necessity
- libraries added only for convenience wrappers
- multiple libraries solving the same problem
- libraries that hide too much control flow

---

## 4. Architecture guidance

### Preserve these boundaries
- Session Service: session identity, ordering, lease/lock ownership
- Orchestrator: run lifecycle and execution flow
- Model Transport Layer: provider-normalized model interaction
- Tool Broker: tool validation, policy, routing, credential mediation
- Tool runtimes: concrete execution only
- Memory subsystem: durable memory and retrieval
- Channel adapters: Telegram / Web UI delivery and input normalization

### Never collapse these into one layer without explicit architectural change
In particular, do not merge:
- transport with orchestration
- tools with broker
- credentials with model context
- memory with provider-side state

---

## 5. Task classification

### Trivial task
A task is trivial if it only changes:
- copy
- comments
- formatting
- isolated bugfix with no architectural impact
- local refactor inside an existing boundary

### Non-trivial task
A task is non-trivial if it changes:
- service contracts
- runtime behavior
- memory flow
- tool contracts
- transport behavior
- credential handling
- approval logic
- session/run behavior
- config model
- deployment assumptions

For non-trivial tasks, the agent must read the relevant architecture specs first.

---

## 6. Spec lookup matrix

### If the task affects general architecture
Read:
- `docs/architecture/butler-prd-architecture.md`

### If the task affects memory or retrieval
Read:
- `docs/architecture/memory-model.md`

### If the task affects run state, orchestration, approval flow, or session/run transitions
Read:
- `docs/architecture/run-lifecycle-spec.md`

### If the task affects model providers, WebSocket-first behavior, provider sessions, or streaming
Read:
- `docs/architecture/model-transport-contract.md`

### If the task affects tools, runtimes, broker logic, tool contracts, or runtime container behavior
Read:
- `docs/architecture/tool-runtime-adr.md`

### If the task affects credentials, secrets, auth injection, or approval rules tied to credentials
Read:
- `docs/architecture/credential-management.md`

### If the task affects language choice, dependency policy, testing, logging, or repo-wide engineering decisions
Read:
- this file

---

## 7. Implementation rules

### Prefer
- explicit interfaces
- explicit DTOs at boundaries
- typed contracts
- predictable state transitions
- small packages with one clear responsibility
- boring code over clever code

### Avoid
- reflection-heavy solutions without necessity
- hidden side effects
- global mutable state
- architecture-defining decisions inside helper packages
- silent fallback logic that changes system behavior unexpectedly

---

## 8. Tool and credential rules

### Tools
- tools are public contracts, not random helper functions
- each tool must have a stable schema
- tool execution must go through Tool Broker
- runtimes do execution only

### Credentials
- raw secrets must never enter model-visible context
- use typed references such as `credential_ref`
- secret resolution must happen in system/runtime layers only
- approval and domain restrictions must be enforced before runtime execution

---

## 9. Memory rules

### Source of truth
- PostgreSQL is the durable source of truth
- pgvector is retrieval only
- Redis is transient only

### Memory classes
Do not blur or collapse:
- Transcript Store
- Working Memory
- Episodic Memory
- Profile Memory

### Working memory rule
- durable logical snapshots belong in PostgreSQL
- transient execution state belongs in Redis

---

## 10. Transport rules

- WebSocket-first where supported by provider
- same logical transport contract for cloud and local providers
- provider-side session is optional optimization, not Butler truth
- transport must not own memory or orchestration semantics

---

## 11. Testing baseline

### Required
- unit tests for meaningful business logic
- integration tests for storage, transport, broker, runtime boundaries
- targeted e2e tests for critical user flows

### Not required
- excessive snapshot testing
- broad e2e coverage with little value
- tests created only to inflate coverage metrics

### Add tests when changing
- policy logic
- run lifecycle behavior
- transport behavior
- tool contracts
- credential resolution flow
- memory extraction or retrieval behavior

---

## 12. Logging and metrics

### Logging
All services should use structured logs.

Minimum useful fields:
- service
- component
- run_id where available
- tool_call_id where available
- level
- message
- timestamp

### Metrics
Keep metrics minimal but useful:
- request counts
- error counts
- latency
- tool call counts
- doctor checks
- credential-aware tool usage where relevant

---

## 13. Documentation rules

### Always update docs when changing
- contracts
- architectural assumptions
- service boundaries
- required configuration
- operational behavior

### Required docs
- repository-level architecture docs
- `README.md` per service

### If a change conflicts with the docs
Do not silently ignore the docs.
Either:
- implement according to docs
- or update docs explicitly in the same task

---

## 14. Security baseline

Always preserve:
- secret isolation
- least privilege
- runtime isolation by tool class
- auditability of sensitive actions
- approval-aware execution
- masked logging for sensitive values

Do not:
- log secrets
- return secrets in tool outputs
- put secrets into prompts
- bypass policy layers

---

## 15. Preferred decision criteria

When choosing between two acceptable implementations, prefer this order:

1. architectural consistency
2. safety
3. simplicity
4. maintainability
5. performance
6. convenience

---

## 16. Final rule

A good Butler change is one that:
- respects architecture boundaries
- adds minimal new complexity
- preserves future maintainability
- stays understandable to the next AI agent
