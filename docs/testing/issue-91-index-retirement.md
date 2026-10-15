# Issue91 explicit derivative retirement evidence

The authenticated Catalog owner API now retires one inactive, terminal-job
semantic generation by exact active-pointer CAS. It needs no running local
models, inference, source read grant or budget reset. Only vector entries for
that generation are erased; original sources, revisions, generation quality,
index/job metadata, attempts, reservations and charged scopes remain. Retired
generations cannot be rebuilt, read/previewed, verified or activated for rollback.

Actual Linux PostgreSQL/race evidence on `ssh laba`, one-CPU test client, isolated
schemas with MaxConns2 and the existing storage-bound test database:

- Source `f2e3c8c0`, `/srv/storage/butler/agents/issue89-20261016/reports/retire-v3/events.jsonl`:
  four tests passed, zero failures/skips. Owner retirement removed five inactive
  vectors while five other vectors survived. Original correction did not block
  erasure; exact source/revision/job/projection/reservation/budget audit hashes
  were unchanged by retirement. Foreign/missing owner, active, stale CAS,
  pending/running denials, terminal failed-job cleanup, repeated receipt and
  irreversible metadata were checked. The private HTTP path worked with
  `Indexing=nil`; authentication, delivery identity and strict JSON remained.
  Existing semantic Verify/Activate/rollback foundation also passed with078.
- Source `09c2a3bc`, `reports/retire-v4/events.jsonl` under the same isolated root:
  concurrent activation versus retirement had exactly one winner. The other
  request was a typed denial; no model call occurred and no retired generation
  became the active pointer.
- Local `go vet` passed for semantic, indexing and Orchestrator API packages.

Historical `retire-v1` and `retire-v2` failures remain: the new positive fixture
used job IDs shorter than the existing eight-character minimum. Owner admission
rejected them before indexing/retirement; the unchanged neighboring indexing
test passed separately. Only fixture identities and stage diagnostics changed.
No production validator was relaxed and no paid/cloud request was made.

This is bounded derivative maintenance, not a privacy source-erasure mechanism,
natural-language quality result, local model throughput benchmark or full
issue91 acceptance. Catalog deletion plus the external anchor still owns privacy
erasure and restore reconciliation. No automatic retention service was added.
