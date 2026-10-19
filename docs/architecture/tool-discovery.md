# Tool discovery and deferred schemas

The optional operator flag `BUTLER_TOOL_DISCOVERY_ENABLED=true` must be set on
both Orchestrator and Tool Broker. It defaults to false and cannot be enabled
through stored settings. Existing private Broker authentication is required.
There is no new service, runtime, queue, dependency or migration.
Conversation action slots support nullable task identity and remain bound to the
exact live input/session/owner/run. Cross-run linking still requires a task.

## Ownership and model flow

Broker owns the current run-scoped capability catalog and fixed `tools.search`
and `tools.load` contracts. These are credential-free local reads through the
ordinary exact preparation, authorization, durable dispatch and budget path.
Each uses the current owning input claim. Autonomy/approval policy, explicit
denials and current task/owner authority still apply.

The Orchestrator initially advertises only these two schemas, a compact list of
permitted categories, and trusted kernel instructions explaining discovery.
`tools.search` accepts query/category/cursor/limit and returns at most eight DATA
cards (default five): name, bounded description, category and exact `tool_ref`.
It never returns schemas, runtime addresses or credential metadata. Package
descriptions remain DATA, not instructions. Durable search observations retain
names, categories and refs while removing descriptions.

Search runs over the complete already-filtered catalog, including installed
extensions and converters. A zero limit on the private catalog reader means
complete results; explicit positive limits still apply after filtering. The
historical pre-selection 64-method truncation is removed, including the
converter SQL list. Rank uses exact name, prefix, name/class/description words
and a small deterministic Russian-to-category vocabulary. There is no model,
external search, embedding dependency or authority inferred from similarity.
Cursor identity includes the query/category and exact ordered result refs;
changed results produce `stale_cursor`, requiring a fresh search.

`tools.load` accepts one to eight refs. Each ref hashes the complete immutable
contract, including effect/runtime/version metadata. Broker rechecks current
scope and exact refs, returning either selected refs or `unavailable_or_stale`.
The Orchestrator admits at most sixteen selected methods and 24 KiB of serialized
logical definitions, including discovery controls. An oversized load returns
`schema_budget_exceeded` without changing earlier selection. Limits are kernel
constants in v1; no automatic eviction or all-tools fallback exists.

Successful `tools.load` receipts in the existing PostgreSQL tool-call transcript
are the durable selection record for that run. Before the next provider request,
the Orchestrator resolves those refs against the current scoped catalog and
sends the selected native schemas. Full schemas are not copied into load-result
messages. Explicit subsequent loading can replace the selected revision of a
method. Stale, revoked or newly incompatible refs are removed, never silently
rebound to the latest contract. An unselected/unavailable proposal is rejected
before dispatch. Every eventual action retains normal Broker authorization.
The selected contract's action hash is pinned before preparation and compared
with Broker's independently prepared binding before approval/authorization.
A replacement between catalog lookup and preparation is rejected; subsequent
changes fail Broker's existing exact authorization/dispatch binding checks.

The fixed catalog controls are standard local reads for owner-goal, legacy and
delegated work; only their exact owned contracts, local resources and zero
credentials qualify. A child searches/loads only its independently filtered
delegation catalog. These controls do not extend the child's execution scope.

## Transport and recovery

The in-process Go transport adds optional `ToolSetSnapshot` to ContinueRun and
SubmitToolResult. A nil snapshot preserves definitions; a non-nil snapshot
replaces the complete set, including an explicit empty set. Revision is a hash
of ordered definitions and is validated before use. This does not add a gRPC
transport implementation or silently claim support in the existing protobuf
surface. The production Go Core calls the logical interface directly.

Orchestrator chooses the set. Provider adapters only encode it. Antigravity
replaces declarations and active validation schemas while reserving aliases for
historical signed function calls. OpenAI Responses/Realtime and Codex carry the
current set on continuations; Codex reserves collision aliases across updates.
Copilot replaces its session's selected definitions. Ollama's existing tool-free
observation provider continues to reject tools.

Recovery/approval resume reconstructs selected refs from the durable current-run
journal, then validates current task/authority/readiness/catalog state. Saved
provider definitions grant nothing. Restart does not re-execute a load or any
external action, and recovery never falls back to advertising the full catalog.
Same-run selection may grow until its fixed budget; a new run starts with only
the two discovery schemas. Search/load turns consume ordinary call/model budgets.

## Verification and release boundary

The [native comparison of 2026-10-19](../ai/tool-discovery-native-20261019.md)
proves actual search/load/Doctor execution. One full-usage pair used15.54% fewer
native tokens with2.14x latency; preliminary quality was eager4/4, deferred3/4.
Quality noninferiority and complete monetary cost remain unestablished. Defaults
remain false; the owner-requested laba opt-in is not a general release verdict.
Broker wrappers, including reviewed skills, must preserve the prior executor;
the actual Core/Broker process check exercises discovery with skills enabled too.
With converters enabled, taskless conversations omit converter lifecycle and
collection reads before their task-only guards. Those unavailable capabilities
must not deny the rest of the conversation catalog. Their owning execution
guards remain unchanged. Process checks also cover skills and converters enabled
together for ordinary search -> load -> call -> final.

`pwsh -NoProfile -File scripts/dev/tool-discovery-local.ps1` creates a disposable
PostgreSQL container and runs Linux/race checks plus an actual compiled Broker
process. It rejects skipped mandatory discovery tests and records JSONL under
`.cache/tool-discovery/`. Tests cover late catalog entries, Russian queries,
pagination/staleness, exact selection, parent/child scope, current input claims,
schema budgets, transcript recovery, and native wire-schema/alias continuity.
The scripted model-loop test covers search -> load -> call -> final.
The same mandatory, no-skip process/race/vet suite runs in the `Tool discovery`
GitHub Actions workflow against disposable PostgreSQL. It also checks unloaded
and revoked proposals, contract replacement during preparation, and preservation
of prior selection when the method-count limit rejects another load.

These are application/process/wire checks, not native model-quality acceptance.
Before production enablement, use a frozen task set for each selected real
provider: compare full-catalog and deferred arms with identical model, prompt,
tasks, limits and verification. Record task success, schema/input bytes and
tokens, search/load/model turns, elapsed time, complete usage/cost and failures.
Do not infer a cost or quality win from initial schema reduction alone. A real
native search -> load -> operation -> verified result chain is still required;
provider failures and unknown usage must remain explicit.
