# Search/context implementation validation, 2026-10-15

Issues #79 and #80 are implemented in product commit
`0da5128f0c6743a764de30bc3688923961e4ee70`. The prospective comparison improved
complete, grounded retrieval answers from **3/8 to 8/8**, correcting all five
observed omissions. The sufficient-source oracle also answered 8/8 correctly.
These are eight deliberately targeted synthetic cases, not a population accuracy
estimate. See the [independent comparison](memory-search-comparison-results-20261015.md),
[protocol](memory-search-comparison-methodology-20261015.md),
[baseline score](../../testdata/acceptance/results/ru-memory-search-baseline-scored-20261015.json),
[candidate score](../../testdata/acceptance/results/ru-memory-search-candidate-scored-20261015.json)
and [exact safe evidence manifest/metrics](../../testdata/acceptance/results/ru-memory-search-evidence-20261015.json).

## Product behavior and regression proof

- Query locality and external release have separate explicit decisions. An
  assigned local/private task keeps permitted local lookup; that lookup never
  grants cloud generation. Inference for a local-only query requires the actual
  pinned internal backend and matching current LiveAdmission. Unproven inference
  has a fixed diagnostic, while allowed SQL remains usable. Backend errors
  propagate. Task/assignment/mandate authority and source access are rechecked
  after retrieval, including changes during a model call.
- Profile lookup uses currently effective, confirmed, accessible claims. It
  ranks applicable entries before the profile limit, so a relevant late key can
  survive 25 earlier irrelevant keys. Suppressed, expired, future, wrong-scope
  and stale-source entries cannot fill the quota.
- The complete mandatory context is protected. Optional documents, profile and
  episodes use conservative encoded-size estimates, separate optional allowance,
  class quotas and deterministic selection. Duplicate or oversized optional
  records are omitted whole. The product document default is four. Source
  handles survive rendering; final prompt admission trims one optional item at a
  time and reserves response/tool-continuation headroom. Oversized mandatory
  context fails explicitly. Diagnostics contain version/counts/estimates/reasons,
  not source text.

On **ssh laba**, race-enabled targeted Go tests and actual PostgreSQL checks
passed with vet. The two retained proof directories under
`/srv/storage/butler/agents/memory-search-79-80-20261015/reports/` are:

| Proof | Result |
| --- | --- |
| `memory-search-verify-20261015T110753Z-2337628` | 59 test/subtest passes, 0 failures, 3 skips; vet passes |
| `memory-search-verify-20261015T110943Z-2344456` | The 3 skipped Hybrid foundation checks rerun: 3 passes, 0 failures, 0 skips |

The initial three skips were caused by the separate
`BUTLER_TEST_SEMANTIC_POSTGRES_URL` being absent. They were **not** opt-in native
quality tests; the initial operator description was wrong. Setting that actual
database variable closed the gap before native execution. Total: **62 distinct
targeted checks passed, with no required skip remaining**. The verification
script now sets both PostgreSQL variables. The tested production Go bytes match
the executed commit; later report commits do not claim to be executed code.

Coverage includes the assigned-local-only regression; no legacy external
embedding call; unverified backend diagnostics; allowed exact/full-text/semantic
paths; cloud-release denial; per-source restrictions; task/permission/source
revocation during lookup; 25+ profile records; deterministic repeated selection;
long Unicode text; deduplication; mandatory overflow; individual final trimming;
accepted-profile fixture isolation, source provenance and model-input rubric
exclusion. The locality tests combine actual PostgreSQL authority/index state
with controlled model responses: they prove the code/admission behavior, not a
private end-to-end answer from Gemini. Actual pinned local embedding
calls occur separately in the public comparison below.

Local Windows service/orchestrator/prompt tests passed. An unchanged semantic
credential fixture relies on POSIX mode 0600 and failed on Windows; its Linux
counterpart passed in the retained lab proof. Intermediate fixture activation,
assertion and Docker address-pool failures remain in their original directories;
none are counted as model observations. Only this task's stopped disposable
networks/containers were removed to free network slots, preserving their bind
data, IDs and inspection evidence. No global prune was used.

## Executed source and retained originals

| Pin | Baseline | Candidate |
| --- | --- | --- |
| Product/instrumented commit | `94faa39d072f4e51a0494a3359adcd1e1a433f8e` | `0da5128f0c6743a764de30bc3688923961e4ee70` |
| Product baseline parent | `b4a2fd3f26ee7f479141a8d91ad78d86a965b798` | revised product |
| Source snapshot | `20261015-180514-973` | `20261015-181037-310` |
| Source tree SHA256 | `0665ac9d00bf247252fae377d67940f79d61799ff0232e59c4c1387249010c90` | `f88262364499fb6c8d2539b73e6924c4956e08adaf48f03152434d00e73ada1e` |
| Executed collector SHA256 | `d75f1d418bbd590c3d7ae615311e737d60c933a018c16f7665a2b68f22de61f7` | `fec1ed985ec71998ba1b9f6d57a97873db372db9f21db5ba4f8e86cfbab2e328` |

The baseline changes only the shared test collector/export/launcher, new corpus
and prospective documentation; production files are unchanged from b4a2fd3.
Both runs use the same runtime image
`sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61`.

Original roots:

- `/srv/storage/butler/agents/memory-semantic-trial-search-baseline-20261015`
- `/srv/storage/butler/agents/memory-semantic-trial-search-candidate-20261015`

Each writer exited, PostgreSQL restarted, and a new reader captured current
context before inference. Each run retains original native input/final/events,
usage/version/auth receipts, attempted and failed observations, capture hashes,
and a 23-table safe SQL export. Both have 75 accepted profiles and 16 indexed
document chunks. Profile sources are never duplicate document chunks. The
manifest pins 16 specifically named non-credential leaves for each root; raw
credential/import/settings/key material is excluded. Roots are sealed and not
rerun. All 32 scheduled native inputs were attempted once: baseline 8/8
completed; candidate 22/24 completed, with two no-LTM transport failures retained
separately. Both retrieval arms and the oracle have zero failures/skips.

The independent review binds exact original input/output bytes and witnesses;
the Go `semantic-score` CLI reproduced both reports offline, including the two
excluded failed observations. Replay performs no model, embedding or auth call.

The two failed no-LTM controls are g01 and g08. Their original native events
contain `run_started` followed by `transport_error`, code `provider_unavailable`;
the safe auth receipts show `paid_discovery` starting and reaching its deadline.
For g01, OAuth refresh returned HTTP 200 and refresh persistence completed before
that discovery timeout. No discovery HTTP status or underlying network/provider
cause is established. This diagnoses the failed provider-account discovery
stage, not a slow embedding or an answer judged incorrect; no completed model
version/usage is fabricated. The independent report retains the original event
hashes and absence of a success-result leaf for each failed attempt.

## Context usefulness and resources

The new context delivers all required evidence in five previously incomplete
cases; current task restrictions, entity separation and justified uncertainty
remain correct in the other three. Literal memory-section UTF-8 bytes are shown
below; these are **not provider tokens**, invoices or the complete request size.

| Case | Old bytes | New bytes | Useful evidence change |
| --- | ---: | ---: | --- |
| g01 | 320 | 588 | All three requested fields instead of two |
| g02 | 1866 | 224 | Room/floor document replaces unrelated profiles |
| g03 | 1866 | 539 | Late-key train preference becomes visible |
| g04 | 379 | 424 | Current restriction retained; profile source handle added |
| g05 | 11514 | 209 | Meeting document replaces long irrelevant profiles |
| g06 | 270 | 718 | Latest warehouse plus section/time, with dated history |
| g07 | 605 | 650 | Correct entity exception retained; profile handle added |
| g08 | 439 | 439 | Missing confirmation and distinct entity retained |
| Total | 17259 | 3791 | All eight answers complete |

For the matched **eight retrieval generations**, provider-reported input usage
was 8202 → 3898 tokens, output 801 → 862, and total usage 16434 → 11731. Provider
total usage includes more than visible input/output (for example reasoning);
the counters are retained as reported, not reconciled by inventing a value.
No per-fact tokenizer measurement is claimed. Useful content is assessed by
required-fact presence and answer quality, alongside the size measurements.

Both matched retrieval slices made 24 auxiliary embedding calls (8 document
batches and 16 query embeddings), 24 logical reads and 158 logical
original/profile-acceptance writes. Reranking was not performed in this native
comparison. The complete candidate matrix additionally contains its no-LTM and
oracle controls: 40 logical reads and 24 generation attempts, versus 24 reads
and 8 generation attempts in the baseline. These different full-matrix totals
are retained rather than presented as a matched cost reduction. No retry, automatic
extraction, summary or consolidation call was made. Native retrieval latency
was p50 12.560 → 10.127 seconds and p95 18.743 → 26.896 seconds, with summed
generation time 109.211 → 107.115 seconds. This one paired sample supports a
quality/context improvement, not a reliable speed improvement. Auxiliary sums
include separately overlapping indexing stage counters and must not be added to
wall-clock time. Token counts unavailable from the local embedding backend stay
unknown. The owner-provided incremental-price premise and local price basis are
zero; generation expense `microunits` values remain NULL/unknown where no invoice
measurement is available. They are not converted to measured zero. Physical
storage/CPU overhead remains unknown. Retained
logical storage counters are in the evidence JSON, not inferred disk savings.

## Scope

This validates document retrieval, accepted Profile Memory selection, mandatory
task/context preservation and privacy/revocation code paths. It does not claim
automatic extraction/write quality, episodic consolidation quality, every
Transcript/Working lifecycle, adaptive multi-step retrieval or general production
accuracy. Selection is explicitly versioned lexical applicability plus bounded
hybrid retrieval; further tuning needs another fresh corpus.

PR #96 is stacked on still-open PR #95. These changes were executed in isolated
lab snapshots and disposable databases. The main lab deployment was not rolled
forward by this task; merge/deployment remains a separate operational step.
