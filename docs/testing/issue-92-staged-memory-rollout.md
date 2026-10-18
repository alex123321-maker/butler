# Staged memory rollout contract

This is the prospective operating procedure for issue92. It does not activate
the primary laba deployment. Entry requires a reviewed release manifest; the
current candidate still has the outstanding gates in
[composition progress](issue-92-composition-progress.md). Controlled fixtures and
synthetic owner decisions do not count as human review of real user memory.

## Release manifest

Before starting a stage, record the exact source/image, schema, model/profile,
canonical contracts, feature settings, policy IDs and conservative resource
vectors. Attach the independent quality reviews, complete foreground/background
usage including unknowns, latest full composition/lifecycle receipts, capacity
profile and external deletion-anchor identity. A component receipt applies only
to its measured contract and profile. Material changes require fresh relevant
checks; a flag or HTTP200 cannot replace them.

The candidate hardware profile is bounded exact search with at most10000 chunks
per active generation. Larger single scopes and the1M target remain unsupported
until separately measured; see [capacity evidence](issue-91-memory-capacity.md).
The complete mixed-load receipt must pass its prospective limits before claiming
that profile ready. Provider latency and real user quality need their own samples.

Adaptive retrieval remains off after its
[measured v5 rejection](issue-87-native-v5-independent-review.md). The explicit
source reader can be assessed independently. Local interpretation27/28 and the
optional93/94 research remain off. A component without its own passing gate
cannot enter a stage through another component's qualification.

## Stage boundaries

| Stage | Allowed behavior | Entry | Exit |
| --- | --- | --- | --- |
| Shadow | Isolated synthetic owner/scopes; read-only comparison and quarantined candidate proposals; new candidates are not accepted or used | Current lifecycle/security, backup/restore and resource controls pass; sources and owner policies explicitly selected | All scheduled cases and costs reviewed; zero unauthorized release, invented authority or privacy resurrection; every quality/SLO miss retained |
| Limited proposals | One selected owner; one active policy at a time; each policy permits one job against exact sources; every candidate requires the owner's ordinary decision | Shadow passes; candidate model/profile qualified; current bounded policy/day allowance recorded | Ten distinct proposals inspected by the owner with source quotes, scope and conflicts; decisions and rejection reasons retained; no automatic acceptance; withdrawal and pause/cancel controls pass |
| Limited use | Only selected, accepted sources and separately qualified consumers; one interactive task and one background job at a time | Proposal stage passes; every enabled consumer's quality/full-cost gate passes; supported model and measured capacity match | Ten owner-reviewed tasks retain correct applicable evidence and outcomes, with complete costs and no critical error; mixed-load limits hold; scope expansion requires a new review |

These counts are prospective operational samples, not statistical proof of broad
user quality. When no real user workload exists, the release record says that
explicitly and the real-user exit remains pending. Scripted fixture acceptance
cannot satisfy either ten-item human-review exit. The owner can reject or pause
at any step without a model overriding that decision.

## Flags and effective state

All new consumer flags default false. The existing legacy
`BUTLER_MEMORY_PIPELINE_ENABLED` pipeline remains disabled for this rollout.
Source controls and policy creation retain ordinary owner authentication and
delivery identity. Endpoint credentials are private references, never model data.

| Capability | Requested configuration |
| --- | --- |
| Source reads | `BUTLER_MEMORY_READ_ENABLED` in Orchestrator and Broker; Broker's fixed `BUTLER_MEMORY_READ_CONTROL_URL` |
| Candidate tool | `BUTLER_MEMORY_CANDIDATE_ENABLED` in both services; fixed `BUTLER_MEMORY_CANDIDATE_CONTROL_URL` |
| Candidate jobs | `BUTLER_MEMORY_MODEL_JOBS_ENABLED` plus `BUTLER_MEMORY_MODEL_CANDIDATES_ENABLED`; exact owner model policy |
| Compaction | Global jobs plus `BUTLER_MEMORY_SUMMARY_CONSUMER_ENABLED`; current task/source policy |
| Explicit temporal facts | `BUTLER_MEMORY_TEMPORAL_ENABLED` in both services and fixed `BUTLER_MEMORY_FACT_CONTROL_URL`; independently qualified profile |
| Consolidation | Global jobs plus `BUTLER_MEMORY_CONSOLIDATION_CONSUMER_ENABLED`; owner reviews diff and activates exact version by CAS |
| Experience | Global jobs plus `BUTLER_MEMORY_EXPERIENCE_ENABLED`, exact environment fingerprint and current tool dependencies; owner accepts note/skill through existing lifecycle |
| Adaptive retrieval | `BUTLER_MEMORY_ADAPTIVE_RETRIEVAL_ENABLED=false` |

Startup flags alone do not create policies, accepted facts, read grants or code
qualification. Record fresh Doctor `effective_features` before and after each
stage: requested, wired, qualified, profile-ready and actual handler stay
separate. Also record current worker/policy state, used scopes, unknown usage and
pending/running jobs. Missing observations are unknown, not zero or ready.

## Stop and rollback

Any unauthorized disclosure/action, stale output after withdrawal, cleared
UNKNOWN effect, deletion resurrection or lost charge stops promotion immediately.
A quality, resource, provider or latency miss pauses the affected consumer and
retains its failure; a high average cannot compensate for a critical error.

1. Pause the durable worker with `POST /api/v4/memory/model-worker-control` and
   pause/revoke affected policies through `model-policy-control`; cancel selected
   jobs through `model-cancel`. Inspect current jobs and durable charges before
   creating any separately authorized recovery attempt. A NULL observation is
   never refunded or rewritten as zero.
2. Disable the affected feature in both owning services where applicable and
   restart only the staged composition. Check fresh effective Doctor state and
   absence of new admission. Main deployment changes require their separate
   reviewed deployment action.
3. Roll back only to a compatible inactive index/summary/consolidation version
   using its current CAS and privacy proof. Reverting a model or index cannot
   revert accepted owner decisions or the latest deletion anchor. Use explicit
   retirement for unused derived generations; preserve primary sources and cost
   metadata.
4. For recovery, restore a custom PostgreSQL dump into an empty isolated database,
   apply the latest independently retained deletion anchor, reconcile twice and
   verify erased copies plus unchanged effects/charges before allowing reads.
   An old dump cannot supply the latest anchor. Missing/partial anchors fail
   closed. Populated080/081/082 provenance must not be downgraded to make an old
   binary start; retain a compatible binary or keep that composition stopped.

The runbook records the stop trigger, exact affected policies/versions, actual
operations, owner decision and re-entry evidence. It never clears a deletion
manifest, effect journal or budget scope to resume work. The physical custom
restore/ENOSPC scenarios are documented in
[the coherent lifecycle report](issue-92-coherent-pilot.md).
