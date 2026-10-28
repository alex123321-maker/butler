# Independent checker input index v1

This is a public layout adapter, not an expected-answer file. The collector may
retain its own files and generate this small index without changing evidence.
All paths below are relative to the index's directory. Absolute paths and paths
outside that evidence root are rejected by the checker.

```
{
 "version":"behavior76-checker-index-v1",
 "stimuli_sha256":"...",
 "episodes":[{
   "episode_id":"opaque control ID", "masked_arm_id":"opaque arm token",
   "aliases_path":".../aliases.json",
   "tables_dir":".../tables",
   "steps":[{
     "step_id":"opaque original step ID",
     "before_snapshot_path":".../before.json",
     "after_snapshot_path":".../after.json",
     "outcome_path":".../outcome.json"
   }]
 }]
}
```

`aliases.json` has `tasks` mapping original symbolic key to
`{task_id,session_key}`, and `sources` mapping original source key to an array of
actual `{source_id,revision}` records, retaining old revisions after correction.
No baseline/candidate labels or arm mapping occur in these files.

Snapshots contain `tasks` as actual task.Record projections, `objects` as actual
independently SQL-read behavior76Object records and `profiles` as actual owning
Profile/Catalog rows, each with source/provenance and confirmation/effective state.
Keep original field names; do not rewrite a proposed action as observed state.
Empty arrays are different from missing fields.

Every actual step, including non-evaluated owner events/restart, has before/after
snapshots. `outcome.json` contains
`{state,owner_input_id,owner_run_id,run_ids,automatic_run_ids,native_entry_ids}`.
The owner input/run IDs must correlate to actual `accepted_inputs` rows. Typed
Controller amend/resume may enqueue real automatic continuations; retain/process
them through ordinary FIFO. All automatic runs/finals/effects/expense remain in
the evidence. The scorer selects the exact owner-message final, not whichever
automatic final happens to be last. `state` is
`completed`, `admission_denied`, `failed`, or `not_evaluated`; safe error category
and owning failure phase accompany denied/failed rows. Actual IDs correlate to
the raw owning tables. An inactive-task denial can legitimately have zero native
IDs, but it must have its actual admission/input receipt and unchanged state.

`tables_dir` includes raw JSONL at least for tasks, task_results,
run_final_results, task_control_events, task_control_targets, accepted_inputs,
runs, action_intents, action_authorizations, action_effects,
action_effect_dispatches, action_effect_evidence, tool_calls,
execution_model_attempts, execution_budget_reservations,
behavior76_objects, behavior76_object_events, behavior76_recipient_receipts,
memory_profile, memory_claims, memory_sources, memory_source_revisions,
memory_source_dependencies, memory_run_sources and memory_scope_grants.
Preserve the actual version-specific table/column shape; unavailable evidence
must be declared, never replaced by an invented empty table.

Every collector/native request/event artifact and original result must also be
present, with its SHA256 in the final evidence manifest. A normalized index does
not prove the collector's projection correlation; the independent audit checks
those references separately and will mark unresolved correlation unverified.

Mechanical score output deliberately leaves communication judgments unassessed.
Only the independent reviewer adds masked per-turn semantic judgments against
the frozen rubric after reading exact answer/evidence bytes. Mapping is disclosed
only after that judgment file is sealed and hashed.
