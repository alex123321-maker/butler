# Final generic review of fixture-only AttachRun repair

Verdict: no remaining source-authority bypass or ordering blocker found in the
reviewed repair. This approves its structure for the declared local TEST fixture;
it does not approve native admission, fresh controls, semantic outcomes or issue
closure. Full composition and fault-policy gates remain separate.

Reviewed source: `behavior76_task_context_test.go` and the precise tracked changes
in `behavior76_collect_test.go` and `behavior76_fixture_test.go`. The continuation
helper diff is formatting only; the local Boundary selector additionally runs
the two new integration tests. No production kernel, Store, trigger, Broker,
candidate instruction or source policy was changed by these repairs.

The wrapper now requires the actual execution token's run/session, delegates
ordinary Store.AttachRun, then verifies current durable binding/revision before
touching metadata. It carries that same context into the ordinary fresh
Get/SaveWorking path. A mutex-protected once-per-Core map is set only after a
successful write. Duplicate calls still execute Store.AttachRun and validate the
current revision before consulting the map. Thus the second ordinary Core
attachment does not rewrite working_revision after task_context loaded a record.
The cache grants no authority and is not a durable recovery receipt.

Concrete `*task.Store` embedding retains every Store optional method; explicit
compile assertions cover task.Manager, ValidateRunAuthority and BlockRun. The
ordinary Controller's mutation path revokes/locks session ownership, while
SaveWorking receives the actual fenced execution context. The repair does not
switch writes to an unowned f.ctx/owner actor or weaken task/source fences.

The public Linux race v2 log independently inspected here records PASS for:

- lifecycle correction/revoke/delete followed by actual fresh Input/Core
  preparation, with old-write SQL40001 still required, old run sources still
  invalid, corrected profile present or revoked/deleted bytes absent, and no
  restored redacted plan/pending fields;
- exact model-facing synthetic metadata equality with durable state at the
  scripted provider boundary and actual CheckRunSources success;
- first metadata write increments WorkingRevision once, duplicate AttachRun
  does not increment it, missing/mismatched token and stale revision deny, and
  the promoted ValidateRunAuthority method is actually called.

The provider is explicitly scripted zero-native infrastructure evidence. Its
constant answer and fake usage do not establish model interpretation, quality,
real cost or native version. The reviewed map is per-Core; this does not claim
durable once-only behavior across a reconstructed Core resuming the same run.
The declared owner-event restarts create a fresh subsequent run, not such a
mid-run recovery. Any later resume composition would need its own proof.

Outstanding before a fresh pilot: full production preparation across all
independent controls/events/restarts; final source/Broker/fixture hash freeze;
the separately declared transport-error stop/advance policy demonstrated through
actual Input/kernel terminalization; complete evidence/export guards and new
independent stimuli/expectation commitments. A/B/C history stays immutable.
