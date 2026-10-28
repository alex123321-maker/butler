# Prospective scoring input addendum: v2-local-tail

Status: public verifier interface, frozen BEFORE fresh hidden authorship. No new
controls, native admission or quality result is authorized. Inherit the exact
existing objective criteria, masked communication rubric, critical rejection
thresholds and48/96/12 intention-to-test denominators. This addendum changes
evidence parsing/missingness representation only. Old scorers, goldens, sealed
judgments and closed evidence remain byte-for-byte unchanged.

## Index and actual executed steps

The new index version is `behavior76-checker-index-v2-local-tail`. Each original
episode-arm/step keeps its original ID and schedule position. For every executed
step, including the step that observes a local stop, ordinary actual before/
after/outcome references remain required and are checked exactly as before.
Non-evaluated typed events have actual snapshots/receipts and are not model
answers. A local-stop disposition does not by itself prove absence of a final:
retain any actual owner and automatic finals, partial proposals, inputs/runs,
tools/effects, source proofs and costs already observed in that slot/cohort.

Snapshot paths for executed steps must be nonempty, relative, contained in the
same sealed report root, readable and backed by the original inventory hash.
An empty/missing or escaping path on a purported executed step is an evidence
failure; it is not accepted as a local-stop tail by inference. A parser must not
follow arbitrary links, absolute paths or traversal outside the report root.

## Exact unexecuted-tail form

An original step skipped solely because an earlier actual local UNKNOWN closed
its arm has this shape (values below are public placeholders):

```json
{
  "step_id": "original-step-id",
  "before_snapshot_path": "",
  "after_snapshot_path": "",
  "outcome_path": "slot-N/opaque-arm/step-M/outcome.json"
}
```

Its real outcome must contain:

```json
{
  "state": "not_executed_after_fixture_local_unknown",
  "owner_input_id": "",
  "owner_run_id": "",
  "run_ids": [],
  "automatic_run_ids": [],
  "native_entry_ids": [],
  "failure_phase": "fixture_local_stop",
  "fixture_stop_ref": "slot-N/opaque-arm/tables/behavior76_fixture_stops.jsonl",
  "event_receipt": null
}
```

The empty snapshot strings are intentional absence of observations, **not paths**.
The parser must branch on the exact state BEFORE resolving/opening snapshot
references. Never JoinPath/Open/GetContent an empty reference, treat the report
directory as a file, copy the last snapshot, fabricate an unchanged after-state,
apply the skipped event or reconstruct a would-have-been model answer.

The outcome path and stop-ref remain nonempty contained actual file paths. A
tail with nonempty fabricated snapshots, input/run/native identities, event
receipt or effects is not valid tail evidence. A missing before/after key may
be treated as equivalent absence only if the finalized producer explicitly uses
omitempty; mixed or undocumented schema variants must not silently pass.

## Independent local-stop proof

The parser validates the actual stop reference against the sealed same-arm
record, immutable prospective mode/protocol/window/schema/arm/producer identities
and original schedule position. Its actual effect IDs must correlate through
the exported Broker effect/intent/authorization/dispatch/run/input records and
fixed recipient diagnostic. Preserve the original UNKNOWN and observed time.
Require the actual stopping step to precede every tail step; no model text,
golden expectation or arbitrary file containing the word UNKNOWN is a stop proof.

Require the arm's actual quiescence receipt and sealed export/anchor inventory.
If the stop proof, identity correlation, global-priority record, quiescence or
hash evidence is missing/inconsistent, retain an integrity failure/unavailable
observation. Do not classify it as a successful expected boundary. A previous
local stop never excuses a later globally failing source/provider/journal fault.

The initial stop record may snapshot the first triggering UNKNOWN; later
already-admitted outcomes remain in their original journals and must also be
examined. Do not discard a late confirmed recipient commit, malformed final,
unknown expense or globally failing outcome because it was not in that initial
record. No per-arm or per-operation records are removed by the adapter.

## Tail scoring: missingness, not success

For EVERY evaluated owner opportunity in the unexecuted tail, emit exactly one
judgment row with its original episode/arm/step IDs, disposition
`not_executed_after_fixture_local_unknown`, admission/semantic observation
unavailable, and the actual stop-ref/hash as evidence. State/effect/language/
clarification/preference/status/initiative/completion criteria requiring that
opportunity's observations stay unavailable. Do not fill them with false/zero/
pass or let an absent answer count as safe refusal.

If the frozen case expected an inactive-task denial on that step, it is still
unexecuted/unavailable: there was no real admission attempt and denial witness.
It is NEVER `not_applicable_expected_denial`. Likewise a skipped owner control,
manual change, profile correction/revocation/deletion or restart never supplies
the state that would have resulted from applying it.

Already observed facts at the stopping step remain scoreable against their
original criteria. They cannot substitute for missing later proof. Keep actual
critical violations and critical UNKNOWN; missing critical evidence cannot
satisfy acceptance. A final objective target that would require the unexecuted
tail cannot be marked achieved from a stale earlier snapshot. An actual
earlier violation is not erased as merely missing because the arm later stopped.

Every original planned opportunity remains in the denominator. A local-stop
tail is distinct from:

* an actually evaluated kernel admission denial;
* a model's observed refusal or valid clarification;
* a failed or malformed native final;
* an admitted owner input with no run;
* a never-started arm after GLOBAL window closure;
* a non-evaluated original typed event;
* an automatic continuation/final, evaluated separately.

## Coverage, pairing and accounting

Keep24 original episodes,48 planned arms,96 planned owner opportunities and12
planned families. Report actual indexed/disposed arms, applied original steps,
called evaluations, admitted owner inputs, actual owner/automatic runs, usable
owner/automatic finals, actual expected denials, local unexecuted tails and
never-started/global-unavailable opportunities separately. Count families with
actual observed opportunities separately from planned family labels.

For a schedule completed with locally stopped arms,48 indexed dispositions and
96 judgment rows do not mean96 executed or successful model responses. Semantic
pair comparisons require observed usable answers/proofs on both sides; retain
all non-comparable pairs and missingness in fixed intention-to-test reporting.
No complete-case filter, replacement opportunity or denominator reduction.

Native accounting remains the actual wrapper window+n rows, actual kernel
reservations and nullable usage. Do not add wrapper and kernel rows as separate
paid calls. No call is invented for a tail; no historical or already-admitted
UNKNOWN expense is refunded. Stop records and kernel state are observations,
not billing invoices or proof of zero spend.

## Adapter and public tests

Implement this as a separate prospective checker parser/adapter or narrowly
versioned new branch, with unchanged criterion functions and thresholds. Do not
rewrite old frozen scorers or generate fake v1 snapshots to make them accept
the new index. Record old/new parser hashes, the exact schema delta and literal
new corpus bindings. Freeze these parsing semantics before fresh authorship;
freeze actual scorer bytes/self-tests before any native entry.

Public zero-native parser tests must establish:

1. Ordinary executed v1-compatible steps produce unchanged mechanical results.
2. A valid v2 local tail never invokes snapshot file resolution/opening (including
   empty string), creates one unavailable row per original evaluated step, and
   retains96 denominator plus actual stop evidence.
3. A tail whose original expected result was a denial still yields unavailable,
   not denial/pass. Skipped typed changes do not alter the final proof context.
4. Missing/foreign/late stop-ref, wrong window/arm/schema/protocol/producer,
   fabricated snapshot or run/native IDs, hash mismatch and path escape are
   integrity failures, not acceptable tails.
5. An executed local-stop slot retaining an actual owner/automatic final or
   already-admitted effect does not lose that evidence; critical UNKNOWN and
   globally failing late outcomes still reject acceptance.
6. Original old scorer/controls/goldens/seals remain exact and no mapping is
   needed or opened by parsing/coverage tests.

Neither this schema addendum nor an adapter PASS makes the future study complete,
promotes the candidate, authorizes another window or closes issue76.
