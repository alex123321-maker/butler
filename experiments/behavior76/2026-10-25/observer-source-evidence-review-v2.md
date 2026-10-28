# Observer amendment and fresh-admission evidence review v2

Pre-entry public source review only; fresh native calls remain0. Fresh hidden
stimuli8d064bab... and expectations7479b3d9... remain read-only, unchanged and
undisclosed to the author. This document does not approve a final new freeze.

The normal-success observer repair captures actual admitted pending Input IDs
after owner acceptance and before starting the ordinary two-worker service. It
waits until that cohort has no queued/running rows before normal shutdown.
Within the declared fixture composition, Controller continuation admissions
occur before evaluate; the two model tools do not admit new owner Inputs. Thus
this captures the actual eligible cohort without hidden expected actions and
fixes fast-owner cancellation of slower automatic work on the success path.
Blocked/waiting rows are not rewritten or resumed. Existing deadlines and all
per-slot/per-run/native limits remain. Mutex around scripted request collection
is a test recorder race guard; its response is unchanged.

The public delayed-A regression first observes B's actual completed Input before
allowing A to finish. The original helper demonstrably failed with A incomplete;
the repaired narrow run passes with two independent execution Input/final rows.
Controller's completed audit receipt is retained but correctly excluded from
the two-execution assertion by actual outcome.RunIDs. The300ms post-condition
delay widens the ordering gap; a channel-controlled release would be a stronger
timing-independent barrier, but the preserved before-failure is meaningful.

Remaining owner-terminal boundary: failed/blocked/expired/cancelled owner paths
still return immediately and cancel other workers. Such a state is not by itself
a proof that unrelated automatic work finished. The fresh collector does not
seal merely because outcome.State is failed. This path must therefore either
drain a nonfatal ordinary outcome under the same actual-cohort rule or be
explicitly classified as a failure-stop/partial automatic observation in the
prospective amendment. It cannot be described as successful general quiescence.
Fatal provider/source/journal/global-deadline stop remains authoritative; no
seal clearing, forced resume or protocol-error advance is proposed.

Additional read-only evidence exports are appropriate. Besides existing sources,
source revisions/dependencies/grants, tasks/runs/claims/ownership and effect rows,
retain memory_run_transcripts, transcript_heads, memory_restore_state,
memory_deletions and messages. CheckRunSourcesTx also consults summary and temporal
state: retain session_summaries, memory_summary_sources/dependencies,
memory_temporal_run_pins/source_pins, memory_claim_temporal_heads/revisions and
memory_fact_decision_history, including actual empty exports when unused. Include
task_authority_bindings and owner_goals so the actual authority composition is
evidenced rather than inferred. Do not export auth/settings/credential material.

Preserve each fixture's exact existing deletion-anchor bytes privately before
Go cleanup, with SHA256 and matched PG watermark/deletion IDs. Do not initialize
a replacement or imply that a PG-only backup restores a missing anchor. The
copy/export should follow quiescence (or explicitly report partial failure) and
use bounded independent evidence contexts. It must not change model context,
source eligibility or any privacy guard.

Correct a public proof assertion: CheckRunSources(f.ctx,oldRun)!=nil is guaranteed
by lack of ownership token and is not specific evidence that old source pins
remain stale after fresh preparation. The earlier SaveWorking SQL40001 before
attachment is meaningful. After fresh attachment, assert the actual PG
butler_memory_check_run_sources(oldRun) still raises40001 (or the ordinary
CheckRunSourcesTx path); do not weaken the guard to make this assertion pass.

Fresh admission guard findings:

- Exact predecessor manifest equality and aggregate174 attempts/172 measured/
  1,274,988 known tokens are useful, but do not prove unchanged individual
  attempts. Compare original A/B/C DB attempt rows programmatically against the
  sealed C final JSONL by window_id+n and exact jsonb row content. This is a
  content-blind integrity comparison; no payload or mapping is disclosed.
- Fresh window deferred closure currently uses context.Background without a
  finite bound. Use an independent bounded cleanup context and retain/report
  closure failure rather than hang or imply a closed window.
- Native-entry correlation row iteration must check rows.Err before accepting
  native_entry_ids. A partial read cannot silently become a complete projection;
  retain the underlying journal and fail/seal explicitly.
- The trusted entryLimit field is pinned against manifest.max_native_entries and
  bounded at768; it does not grant model-selected authority. Fresh masks/schedule
  and original history linkage are separated correctly from old B/C suffixes.

After these infrastructure/evidence choices are fixed, publish a linked
prospective amendment and exact new producer pin, rerun affected public and
complete zero-native composition gates, and independently review literal bindings.
No frozen fresh golden, scoring criterion or candidate change is permitted.
