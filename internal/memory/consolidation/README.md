# Owner-reviewed memory consolidation

`memory-consolidation-v1` prepares immutable DATA views of a bounded exact
evidence set. It preserves all unique complete evidence, associates repeats and
keeps differences unresolved. Model output can only group existing unit IDs;
trusted code retains provenance and current typed acceptance/temporal semantics.
It never rewrites accepted facts, owner preferences or task/action authority.

The [architecture contract](../../../docs/architecture/memory-consolidation.md)
defines finite explicit cross-session context pins, shared budget/transport,
version/diff review, owner activation CAS/rollback, retirement and deletion fences.
The implementation uses the existing model queue/transport, migration076 and
#88's catalog temporal pins. `ReleaseQualified=true` follows the bounded
[native V3 paired review](../../../docs/testing/issue-89-native-v3-independent-review.md)
and lifecycle/restore controls. The selected profile improves measured full
foreground/background tokens in those three pairs; its larger bytes and time
remain explicit limitations. Process defaults and the primary deployment stay
unchanged. This is not automatic selection or broad workload qualification.

Owner workflow, under the existing bearer owner authentication and a unique
`X-Butler-Delivery-ID` for every POST:

1. `POST /api/v4/memory/consolidation/prepare` with `view_key`, exact `sources`
   and/or accepted `claim_ids`, optional `temporal_query`, and a finite `policy`.
   It must contain `max_jobs:1`, `privacy:"cloud_allowed"`, one to eight exact
   `contexts` (session, terminal run, optional root task), an allowance/deadline,
   and empty top-level `job.sources`, `task_id`, `original_run_id`. The handler
   and trigger are fixed by code. Input and policy commit atomically. Retrying
   identical policy identity returns the original receipt; changed input requires
   a new policy ID. Every selected original must pass current scope/privacy proof.
   Supported scopes are owner-user and the exact pinned sessions/root tasks;
   project-scoped inputs are rejected pending an explicit project privacy contract.
2. The ordinary durable worker schedules the prepared generation after its quiet
   window. Model output supplies grouping IDs only. A valid result is quarantined.
3. `GET /api/v4/memory/consolidation` lists bounded content-free receipts;
   `GET .../preview?source_id=...` shows grouping, exact current evidence, typed
   temporal metadata, reproducible diff, current head and actual attempt costs.
4. `POST .../activate` supplies `source_id`, `expected_active_id` and
   `expected_revision`. Both head ID and revision must match. Selecting an older
   version uses the same operation and current proofs. No accepted fact changes.
5. If a source or accepted decision changes, the prepared/result version is stale.
   Prepare a new policy/snapshot, review its diff, then activate against the old
   selected ID/revision. Old selected text is never copied into the diff. The old
   choice can be retired without recovering its obsolete access permissions.
6. `POST .../deactivate` supplies only the expected active ID/revision and clears
   presentation eligibility. `POST .../retire` supplies `input_source_id` and the
   expected current head. A selected version must first be deactivated/replaced.
   Retirement is irreversible, then the unchanged catalog deletion mechanism
   erases that prepared input and its result descendants. Originals survive.
   `cleanup_complete:false` is a visible failed erasure acknowledgment: the view
   stays fenced and counts against retention. Retry after restoring the existing
   catalog/independent deletion anchor health; never reset an anchor/watermark.

Content-bearing preparations are capped at eight per owner/view and32 per owner,
including failed jobs and pending cleanup. One preparation admits at most one
result. List retains all pending receipts plus64 latest completed audit entries.
Durable metadata/spend/deletion history is not pruned to fake a fresh budget.
Ordinary search/open require an activated source and its entire current original
graph; activation does not mint another session's foreground read authority.

`BUTLER_MEMORY_MODEL_JOBS_ENABLED` and
`BUTLER_MEMORY_CONSOLIDATION_CONSUMER_ENABLED` both default false. PostgreSQL model
control separately starts disabled. Registration also requires the code-owned
qualification and the exact supported existing provider profile. Prepare and
activate remain unavailable while unconfigured; list/review/deactivate/retire
stay private operational controls for existing data. No legacy memory pipeline
or automatic accepted rule is enabled.
