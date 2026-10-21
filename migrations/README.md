# Migrations

087 adds explicit owner local index maintenance policies and job-policy revision
projections. It does not enable a policy, accept a fact or publish an index.
Deploy new binaries after this additive migration.

SQL migrations live here.

`034_atomic_results_outbox` atomically separates immutable attempt results,
required final artifacts/transcript, stable notification chunks and durable
auxiliary jobs. Down refuses existing results instead of removing result and
delivery history. An operational downgrade needs an explicit preservation plan;
missing records cannot prove an outside effect or message was absent.

`031_action_authorization` stores canonical action intents, irreversible
dispatch grant states and versioned bounded mandates. Down removes these
permission/outcome records; preserve them before operational rollback because
their absence cannot safely imply that an external action was never dispatched.

- `001_sessions` creates the session store baseline.
- `002_runs` creates run lifecycle persistence per the lifecycle spec.
- `003_messages` creates transcript message storage.
- `004_tool_calls` creates normalized tool call persistence.
- `005_enable_pgvector` enables the vector extension for later retrieval work.
- `006_runs_idempotency_key` adds durable input-event deduplication on runs.
- `007_runs_metadata_json` adds durable run metadata storage.
- `008_memory_working` adds the durable working-memory snapshot store.
- `029_durable_tasks` separates persistent tasks/results from run attempts, adds optional run/task revision links, preserves historical run URLs as explicitly unverified legacy work, and copies only matching legacy working snapshots. Down migration preserves original run/transcript/session working data but removes new task-only state; back it up before operational rollback.
- `030_transcript_summary_coverage` assigns immutable per-session message sequences, versions edits/deletions and stores verified summary coverage with monotonic publication. Legacy unversioned summaries are retained in `sessions.summary` for audit/rollback, but cannot suppress history. Down migration removes sequence/coverage metadata while retaining transcript contents.
- `009_sessions_session_id` adds UUID session identifiers alongside stable session keys.
- `010_credentials` adds credential metadata storage.
- `011_memory_profile` adds profile-memory storage.
- `012_memory_episodes` adds episodic-memory storage.
- `013_credential_audit_logs` adds audit records for credential use decisions.
- `014_memory_schema_repairs` aligns confidence fields, vector dimensions, and episodic indexes with the memory model.
- `015_doctor_reports` adds persisted doctor diagnostic reports.
- `016_sessions_summary` adds durable session summaries for later context preparation.
- `020_run_state_transitions` adds lifecycle transition history for run observability and SSE catch-up.
- `021_flexible_vector_dimensions` makes memory embeddings dimension-flexible for multiple providers.
- `022_approvals_and_approval_events` adds durable approval persistence and approval audit events.
- `023_artifacts` adds durable task artifacts (`assistant_final`, `doctor_report`, `tool_result`, `summary`) for task-centric result access.
- `024_task_activity` adds process-level activity timeline storage separate from transcript events.
- `025_channel_delivery_events` adds explicit delivery-state history for Telegram/Web channel visibility in task detail and overview.
- `026_memory_management_fields` adds memory moderation and lifecycle management fields for task-centric UI operations.
- `027_single_tab_sessions_and_web_fetch_cache` adds durable single-tab browser session state, browser tab selection candidates, and web-fetch cache entries.

Use `make migrate-up` to apply all migrations and `make migrate-down` to roll them back in reverse order.

- `029_durable_tasks` separates owner tasks, working state and unverified/verified results from execution attempts.
- `031_action_authorization` persists exact Broker intents, bounded mandates and single-dispatch grants.
- `032_durable_ingress` persists ordered acceptance receipts, checkpoints and expiring monotonic session ownership.
- `033_approval_continuations` persists exact data-only continuations and owner decision input identities; legacy unbound tool waits expire without inventing authority.
- `035_external_effect_journal` persists permanent exact-intent effect states, spent dispatches and immutable evidence; old technical completions migrate to unknown. Down refuses existing effect history.
- `036_task_owner_controls` adds typed task execution policy and owner-control history/archived working state. Rollback refuses existing control history or prepare-only authority instead of discarding it.
- `037_execution_budgets` persists immutable shared scope policies, conservative idempotent reservations, model attempt/usage observations and normalized loop observations. Production model, effect and child admission charge these scopes atomically; run/task revision changes cannot reset them. Down refuses any durable scope, reservation, attempt or loop observation, including unspent zero-limit policy. See `docs/architecture/execution-budget-contract.md`.
- `038_owner_goals_mandates` adds confirmed owner intent, separate assumptions,
  broad versioned authority, immutable decisions, task assignments and pending
  propagation jobs. Legacy task mandates and ordinary tasks receive no new
  permission. Down refuses existing authority/history instead of discarding it.
- `038_owner_purpose_policies` is an additive second step, ordered by the full
  filename after the foundation. It versions confirmed owner preferences and
  constraints by exact purpose and adds assignment policy revisions. Existing
  assignments receive no invented review. Down refuses saved assignments or
  confirmed policy history. See `docs/architecture/owner-goals-mandates-spec.md`.
- `040_initiative_schedules` persists owner-defined sources, revisioned schedules, coalesced/fenced observation ticks, semantic candidates, normalized event/config audit and immutable owner UTC-day attention counts. Rollback refuses any configured source or history, including an unused attention row. Fixed external observers and verified project-repair acceptance remain outstanding; see `docs/architecture/initiative-contract.md`.
- `056_initiative_digests` persists grouped count-only notices and idempotent admitted/verified-result event coverage, independently of model final results. Rollback refuses retained delivery history. Owner channel choice, active hours and current source/goal authority govern delivery; uncertain sends are not repeated.
- `041_semantic_memory_indexes` stores immutable separate embedding spaces/generations, revision-bound derivative vectors, exact-space title/content hashes for reuse after citation changes, reviewed quality manifests and atomic active pointers. Source deletion cascades to vectors; down refuses retained generation/activation history. Reads/publications require a supplied current-source authority guard.
- `042_hybrid_retrieval_indexes` adds a derived simple/Russian GIN full-text expression index for guarded hybrid retrieval. Down only removes this rebuildable lexical index; durable memory, vectors and current authority remain.
- `043_memory_extraction_dedup` persists content-free source/version/pipeline checkpoints and exact candidate retry bindings. It deliberately has no foreign keys to the later046 source/claim tables. Source/claim text remains in the catalog and follows deletion policy; only hashes/opaque IDs/times remain here. Down refuses populated history rather than resetting retry identity.
- `044_voice_records` stores content-free recording/review/control/input bindings,
  roles, current revisions and finite voice-access lifetimes. Originals and derived
  prose remain in later catalog assets/sources; there are no forward foreign keys.
  Down refuses retained voice history instead of discarding input/control identity.
  The optional voice caller remains disabled and its human quality gate failed.
- `046_versioned_memory_sources` quarantines legacy memory, versions source/claim acceptance/access and transitive provenance/tombstones. Current task/goal authority remains separate.
- `047_owned_memory_auxiliary_jobs` stores independently owned finite auxiliary claims/attempts and complete original source/authority manifests; old session leases cannot authorize them.
- `048_memory_transcript_privacy` pins transcript/model/dispatch/result sources, adds one-way privacy redaction and restartable scoped cache cleanup. Missing/wrong independent deletion anchors fail closed.
- `049_structured_source_assets` retains immutable original asset revisions/parser diagnostics, ordinal neighbouring fragments, atomic owned index projection and path/repository lexical derivatives. Extends guarded semantic scopes to task/project. Source delete/revoke erases all original bytes. Down refuses retained asset/index history rather than discarding durable originals; a pristine down leaves harmless ordinal metadata.
- `055_converter_parent_continuations` records two immutable technical continuations admitted with an actual053 developer. A reference-only ordered input and WAITING-to-QUEUED transition commit together; restart cannot refill identity or budgets. Current owner/task/source/fence authority remains mandatory. Down refuses retained history; core requires the owned schema and immutable trigger before opt-in readiness.
- `062_resource_dispatches` binds guarded controlled-browser grants to actual finite budget/resource claims and one-use private runtime tickets. A hashed successful-open mapping derives exact owner/run/task browser provenance before later claims. Rows contain opaque IDs/hashes/deadlines only, no source text, tool arguments, raw session IDs or credentials. Rollback refuses nonempty dispatch or browser history; optional composition remains disabled pending full issue17 acceptance.
- `063_single_tab_resource_binding` adds private approved user-tab identity and frozen observation refs to one-use resource dispatches, plus immutable finite owner requests and atomically selected mappings. Legacy activation cannot satisfy the guarded marker. It stores no browser document/field content or service token. Down refuses populated bindings, requests or selections. Optional guarded status/click production composition and released-lease user-channel acceptance remain separate.
- `066_workspace_source_provenance` binds immutable cumulative code-copy contexts to actual task DATA, physical-observation digests and resource leases. Transitive Catalog deletion atomically blocks copies and queues exact fixed-root physical erasure; failures remain pending. It stores no source/code body or credential and grants no model tool or recovery permission. Down refuses retained provenance/deletion history. Workspace Broker/shared budget/runtime bootstrap remains pending.
- `067_workspace_dispatches` binds immutable exact prepared workspace operation/path/body digest/file CAS, original input/source context and actual shared budget/resource claim to a finite one-use private dispatch ticket. Independent physical observations are immutable and commit with source/file/resource completion. No raw body/root/credential or reusable token is stored. Down refuses retained history; production application bootstrap and owner reconciliation remain pending.
# Migration071: bounded background generative memory

Run the normal migrator before the new Orchestrator. The existing auxiliary queue
gets a `model` kind and immutable policy binding; attempts reference the existing
execution reservations and catalog DATA sources. Global execution starts disabled.
Application rollback keeps this additive schema and all accounting history after
disabling workers. Down refuses policy/job/attempt history; do not erase spending
to force it. See [the contract](../docs/architecture/background-memory-jobs.md).
