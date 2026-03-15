# Butler Project Rules

Butler is a self-hosted, Docker-oriented, Go-first personal long-lived agent platform.

## Core invariants

- Preserve strict separation between:
  - orchestration
  - model transport
  - session ownership
  - memory
  - tool execution
  - credential handling
  - channel delivery
- Do not expose secrets, passwords, tokens, cookies, or raw credential material to the model context.
- Keep PostgreSQL as the source of truth for durable system state and memory.
- Keep pgvector as the semantic retrieval layer, not as the primary memory store.
- Keep Redis limited to short-lived state, locks, queues, and transient execution state.
- Preserve the Butler memory model:
  - Transcript Store
  - Working Memory
  - Episodic Memory
  - Profile Memory
- Preserve the Butler execution model:
  - Session Service owns session identity, ordering, and lease/lock state
  - Orchestrator owns run execution flow
  - Tool Broker owns tool validation, policy, routing, and credential mediation
  - runtimes execute tools but do not make orchestration decisions
- Preserve the Butler transport model:
  - WebSocket-first where the provider supports it
  - one logical transport contract for both cloud and local providers
  - Butler run/session state is always authoritative over provider-side session state

## Language and dependency policy

- Use Go for all core services and tool runtimes whenever reasonably possible.
- Python is allowed only when there is a clear, documented justification.
- Minimize dependencies aggressively.
- Do not add a new library if:
  - the standard library is sufficient
  - an existing project dependency already solves the problem
  - the benefit is marginal
- Treat every new dependency as:
  - maintenance cost
  - security surface
  - container size increase
  - cognitive overhead for future AI agents

## Architecture change policy

Before making any non-trivial change, identify the affected subsystem.

A task is non-trivial if it changes any of the following:
- service boundaries
- APIs or contracts
- memory behavior
- run lifecycle
- tool behavior
- credential handling
- transport behavior
- container topology
- config model
- security or approval logic

For non-trivial tasks, read the relevant architecture specs before making changes.

## Required spec lookup by subsystem

- General architecture:
  - `docs/architecture/butler-prd-architecture.md`
- Memory:
  - `docs/architecture/memory-model.md`
- Run execution / orchestration:
  - `docs/architecture/run-lifecycle-spec.md`
- Model/provider integration:
  - `docs/architecture/model-transport-contract.md`
- Tools / runtimes / broker:
  - `docs/architecture/tool-runtime-adr.md`
- Credentials / secrets / auth injection:
  - `docs/architecture/credential-management.md`
- Technology and engineering standards:
  - `docs/ai/engineering-rules.md`

## Tooling and credential rules

- Never pass raw secrets in tool args visible to the model.
- Use typed references such as `credential_ref` for secret-backed inputs.
- Approval-requiring actions must respect autonomy mode and policy rules.
- Do not bypass Tool Broker.
- Do not let runtimes fetch arbitrary secrets directly without broker-mediated policy checks.
- Browser and HTTP tools must remain credential-aware through deferred resolution, not raw secret exposure.

## Documentation rules

- If behavior, contracts, service boundaries, or operational assumptions change, update the corresponding spec in the same task.
- Every service must have a `README.md`.
- If a service changes in a meaningful way, update its README.
- If a change conflicts with an existing spec, do not silently implement it:
  - either align with the spec
  - or update the spec explicitly

## Testing rules

- Add or update tests for meaningful logic changes.
- Prioritize:
  - unit tests for business logic and policy logic
  - integration tests for service boundaries, storage, broker/runtimes, and transport
  - targeted e2e tests for critical flows
- Do not add noisy or low-value tests only to increase test counts.

## Code style rules

- Prefer explicit code over framework magic.
- Prefer small, readable modules.
- Prefer stable contracts over clever abstractions.
- Avoid hidden global state.
- Avoid architecture drift.
- Avoid speculative abstractions that are not justified by current Butler needs.

## Operational rules

- Assume Docker Compose is the baseline deployment model.
- Keep services self-host-friendly.
- Keep structured logging and minimal metrics intact.
- Do not introduce infrastructure that materially increases ops complexity without strong justification.

## Final rule

When in doubt, optimize for:
- simplicity
- explicitness
- architectural consistency
- safety
- maintainability by future AI agents
