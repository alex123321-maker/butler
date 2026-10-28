# Independent pre-entry review: automatic worker observation boundary

Finding confirmed from public source. `input.NewService` starts two workers.
`Claim` orders FIFO per session, while independent session heads may run in
parallel. `behavior76Fixture.evaluate` currently watches only owner input status.
When it returns, deferred cancellation stops the whole worker.Run context, which
is also the parent of concurrent automatic executions. Waiting for worker.Run's
goroutines after cancellation does not prove they had completed beforehand.
Therefore an owner B result can artificially truncate slower automatic A.

The public successful queued-v2 log includes both actual A/B final completions;
that observation is retained, but its favorable timing does not rule out this
race. The existing two-effect/tool-call join can pass while an automatic model
resume/final is still pending, so it is insufficient as a terminality assertion.

## Required public regression and drain semantics

Use a deterministic scripted provider and synchronization barriers, not a race
whose success depends on a short sleep. Queue the actual owner-control
continuation A, then the independent owner B input. Let A reach its actual
post-tool model resume and wait on a barrier while B returns rapidly. Observe
B's actual durable accepted_input completion. Before releasing A, assert that
evaluate has not returned, A's context is still live and its final has not been
invented. Release A, then require two independently correlated actual finals,
terminal Input records, unchanged exact task/mandate/tool identities and no
artificial cancellation. The old helper should fail this regression.

The helper must wait for ordinary processing of the actual admitted automatic
cohort before a normal worker shutdown. Identify that cohort from durable
accepted_input rows in the isolated fixture schema: pending work present at
invocation plus actual owner admission and any real continuation admitted while
this invocation executes. Do not select it using hidden expected actions or
model-declared authority. Preserve per-session ordering and concurrency.

Quiescence must mean there is no queued/running executable work or in-flight
claimed execution in that cohort at the normal observation boundary. Record
actual cohort IDs/statuses, run/final/claim/lease evidence and automatic outcomes.
Do not equate effect_confirmed with run finality, or exclude a slow automatic
run because its owner message already completed. Terminal run/input outcomes
can be successful or failed; genuine blocked/waiting states remain what they
are and cannot be relabelled completed or resumed by the observer. Report their
unresolved automatic quality/effect boundary explicitly.

All owner terminal paths need the same observation-boundary reasoning; a failed
owner input also does not by itself prove unrelated automatic execution is done.
Fatal global provider/source/journal/observer failure and the original deadline
retain the declared fail-closed stop policy. Do not drain by authorizing another
entry after the native seal, clearing it or extending a task/mandate/deadline.
If a safe complete observation boundary cannot be established within existing
limits, preserve its actual partial/failure evidence and stop rather than force
termination statuses. This is not the previously rejected protocol-error advance.

Include an offline negative for unresolved/blocked automatic execution so the
drain cannot hang indefinitely or turn it into a successful final. No force
Resume, SQL status rewrite, reseed, queue deletion, budget refund, enlarged
allowance, synthetic owner verification or production worker change is allowed.
Use the ordinary worker and current finite fixture/parent deadline.

## Freeze implications

This is a generic fixture observer defect found from public code/logs after the
fresh-v3 authorship freeze and before any fresh native output. It is not a
candidate hypothesis or control-specific quality finding. The accepted public
contract explicitly requires real automatic continuation results to be retained;
repairing premature cancellation aligns the observer with that requirement.

The current semantic freeze cannot silently cover this code change. First fix
and pass the deterministic public regression, independently review the exact
delta, rerun affected public and complete zero-native composition gates, and
publish a linked prospective observer amendment/new producer source pin.

The already frozen fresh corpus/goldens/scorer can remain unchanged if it is
verified that no fresh native response/semantic quality was observed, no hidden
content/expectation was disclosed to the implementation author, and the change
is justified exclusively by this public race. Do not overwrite its original
22e834fb authorship commitment; link the new semantic producer and rationale in
a separate amendment. Source changes must be identical across both arms and
cannot remove a control, change expected outcomes or scoring thresholds.

No new hidden controls or native inference are authorized by this review. Fresh
hashes remain stimuli8d064bab... and expectations7479b3d9..., unchanged.
