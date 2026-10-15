# Verified task compaction

`task-summary-v1` is a separately gated consumer of the existing durable model
worker. It compacts a primary transcript prefix and leaves four newest messages
in the foreground tail. The scanner waits until effective serialized history
reaches 60% of the actual configured context allowance; small conversations do
not invoke a model. Existing explicit owner policy, privacy, task/assignment
revisions, claims, accounting, interactive priority and cancellation still apply.

The model selects exact excerpts from assistant/tool events. Every user event is
preserved verbatim and every primary message has one ordered record. Trusted code
attaches observed tool IDs/names/statuses independently. Historical JSON never
changes the current typed task goal, constraints, approvals or effect journal.
In particular `unknown` does not become `success`. This is deliberately an
extractive continuity representation; accepted Profile facts remain separate.

An authenticated owner submits `POST /api/v4/memory/model-summary-policies` with
`session_key` and `policy` (the existing finite ModelPolicyRequest, with an empty
`job.sources` and matching `job.original_run_id`/optional `job.task_id`). Trusted
code snapshots the actual owner transcript and binds its stable session anchor.
Handler/trigger are code selected. The ordinary policy controls and jobs view
apply. `BUTLER_MEMORY_MODEL_JOBS_ENABLED=true` and the separately default-false
`BUTLER_MEMORY_SUMMARY_CONSUMER_ENABLED=true` are both necessary, as is enabled
PostgreSQL model-worker control. Owner policy remains necessary for inference.
Production registration and the owner policy endpoint additionally require the
existing native Flash High adapter with the qualified generated-token ceiling of
exactly 32768 and a matching finite execution budget `output_tokens` allowance. Set
`BUTLER_ANTIGRAVITY_MAXIMUM_OUTPUT_TOKENS=32768`; the default 0 keeps the previous
8192 transport ceiling and does not qualify this consumer. This construction-time
setting includes thinking plus visible output and never becomes a model option.
The qualified profile also requires the adapter's actual normalized timeout of
at least 300s (`BUTLER_OPENAI_TIMEOUT_SECONDS=300` in the current shared provider
config) and finite budget `model_millis>=300000`. Shorter owner/task deadlines
remain authoritative. Smaller output/time profiles have no native qualification.
Changed task/constraints/assignment authority requires a new explicit policy;
the consumer does not silently rebase permission onto a newer task version.

Each compaction rereads primary events instead of summarizing earlier summaries.
Publication compares every covered primary byte, ordered identity, transcript
revision and observed tool state, then atomically commits coverage CAS, immutable
catalog derivative and the model-job result. Raw message corrections/deletions
and tool status updates invalidate stored coverage. Catalog privacy deletion
also traverses session snapshot anchors and erases all copied input revisions and
their derivatives, including work not yet published. A failed or stale result
never suppresses primary history. The immutable original event IDs survive.

Source input is finite: at most 256 KiB, 4096 primary events and 2000 complete
original privacy dependencies. Output is at most
32 KiB and must reduce source size. Oversized/secret input, failed generation,
invalid excerpts, changed authority or stale coverage retain complete history.
The policy/job view explains bounded wait/failure reasons. If protected history
still exceeds the foreground allowance, existing preparation returns a visible
context-budget error rather than silently truncating user constraints or
manufacturing successful tool effects. No unlimited-lifetime guarantee follows
from these finite bounds.

Copied transcript anchors include all current original message/catalog ancestors,
with revision/access/hash and graph identity. Ordinary catalog reads, derivative
reads, live source checks and DB publication fences reject corrected-away or
revoked originals. Secret, project and other-task ancestors cannot be laundered
through a session snapshot. The scanner can regenerate corrected messages or
changed tool states under the same still-current finite policy before ordinary
source access, without releasing old text. Non-message original corrections,
access/privacy/graph changes and typed task/constraint revisions require fresh
owner policy. A removed session retains an unreadable snapshot guard rather than
turning an orphan copy into current evidence.

Production registration additionally requires the code-owned `ReleaseQualified`
gate, now passed for the exact profile above. The native three-compaction/six-continuation
quality and complete-cost evidence is recorded in the [acceptance report](../../../docs/testing/issue-84-verified-summaries.md).
Environment flags cannot bypass qualification or its actual provider/budget profile.

Real PostgreSQL acceptance: `go test -race -count=1 ./internal/memory/compaction
./internal/memory/summary ./internal/memory/jobs ./internal/memory/catalog`.
The controlled provider is a transport fixture, not native Gemini quality proof.

The copied-snapshot privacy branch additionally requires the actual phased
[`summary backup/restore gate`](../../../docs/testing/issue-84-summary-backup.md).
Run `scripts/dev/memory-summary-backup-lab.sh` on an isolated issue83 root. Both
original-message and non-message-ancestor deletion must survive a real old dump,
fresh restore and newest independent anchor replay. An ordinary package skip
does not satisfy this consumer-specific gate.
