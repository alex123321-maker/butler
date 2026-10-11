# Lab acceptance evidence

This package and `cmd/acceptance` run and inspect fixed, programmatic product
contracts. They do not run an agent, change PostgreSQL authority, execute a
model-selected command, or approve deployment. The standard library is sufficient
for fixed Go test events, bounded JSON observations, digests and Ed25519 review;
no evaluation framework or new dependency is added.

`scripts/dev/fault-lab.ps1 -Profile Full` synchronizes only an isolated agent root
and invokes `fault-lab.sh` there. It creates its own internal Docker network,
PostgreSQL and Redis with synthetic credentials, one database per package, and
removes only the containers/network created by that invocation. It never invokes
the shared Compose project, uses Google credentials, or reuses main PostgreSQL.
Reports, caches, private dump/anchor and retained disposable PG files stay below
`/srv/storage/butler`. Do not publish raw event/dump files: even a future synthetic
fixture can include private-like values. Artifacts have private directory/file
modes. Go runs with race detection; Chromium uses the recorded image dependency
tree, exact compiled helper bytes from the tested snapshot, read-only source and
binaries, a bounded tmpfs, no capabilities, and only the disposable network.

Core selects persisted ownership, approval, stop/amend, authority, action effects,
atomic result/outbox, budgets, provenance, deletion, auxiliary ownership, semantic
freshness and controlled local-worker pressure. Full additionally uses actual
Chromium, real private Broker/HTTP/PG approval/credential paths and three separate
phases around actual `pg_dump -Fc`/`pg_restore --exit-on-error`. Restore keeps the
newest independent deletion anchor: old contents are denied before reconciliation,
then original and derived text/binary are physically erased and reindex is denied.
Installed extension tests become required when their package is present. An older
snapshot without that package reports PENDING, never PASS, for that boundary.
Full also starts the actual Broker and separate HTTP extension runtime against
fresh PG, binds the initialized private anchor and pins each run's transcript.
Current reviewed version, approval, readiness, changed version, rollback and
deferred credentials are exercised without weakening source or dispatch guards.

Core also requires explicit instruction-derivation, DNS/redirect/rebinding/byte
caps, workspace containment, reflected-value masking, reviewed model-loader,
generated-package isolation and actual PostgreSQL delegated-source checks.
Child selected content cannot gain instruction authority or expose unselected
ancestors. Fixed transport diagnostics distinguish a source-check timeout from
provider expiry and reject private error material before durable failure state.
These named checks have the same missing/skip/failure rules as other groups.

Each required named top-level test must run exactly once and pass with package
completion. Missing, skipped, failed, duplicate, foreign-package or malformed
events fail the selected evidence. `selected_passed` describes the selected
available checks; it never implies `issue_33_ready`. The latter remains false
while mandatory product scenarios are pending. JSON output retains hashes and
timings, not raw test output. Exact source tree, migration files, corpus, criteria,
commit and hardware/image manifests are pinned. Wall time is not a billed model
cost and controlled worker OOM is not actual hardware OOM.

The active corpus is `testdata/acceptance/ru-scenarios-v3.json`, self-authored
synthetic Russian material: 240 examples, 60 per
kind, derived from **30 source/topic groups**, not 240 independent observations.
Development has 12 groups/96 cases, tuning 8/64, holdout 10/80. All paraphrases,
positive/absent searches, extracted/counterfactual records and control examples
share the original group's split. Exact normalized and >=85% token-trigram
Jaccard matches across splits are rejected. That check is a leakage screen, not a
semantic proof; independent review must inspect harder similarities. V3 also
records 15 explicit injection families, each confined to one split: six in
development, four in tuning, five in holdout. Half the hostile documents also
contain a legitimate fact that must still be extracted. The family IDs describe
synthetic templates authored for this version, not attacks collected from an
independent population. Four
holdout source-bearing cases marked secret never enter model input or scoring.
There are 236 eligible model observations overall and 76 in holdout; the secret
contracts are tested before IPC by the kernel suites.

Export inputs with the lab binary:

```sh
acceptance -mode inputs -root /src -split holdout -output /private/inputs.json
```

This emits the fixed per-kind JSON instructions, documents as DATA, case IDs and
blocked IDs, without golden answers. It is an observation request, not an execution
API. V1 received an independent **NO_GO** for ambiguous absent-search answers,
cross-split injection templates and undefined compound command intents. Its
original bytes and verdict are retained; historical reports remain measurements
of V1. V2 replaces seven ambiguous absent questions with undocumented facts,
defines all 17 command types and compound priority, and changes d02 to an explicit
draft-only amendment. V2 also received NO_GO: other absent-search gold remained
ambiguous, one negative injection header contained the target field, three
draft-only commands remained underspecified, and missing measurement fields
became zeros. Both rejected versions and independent verdicts are preserved.
V3 asks for 30 specific undocumented properties, removes target values from
negative injection documents, makes those commands explicitly draft-only and
requires explicit measurement fields. The separately appointed critic approved
V3 golden answers and criteria, with all 240 cases reviewed and 76 independent
metadata negative checks; the signed external verdict is
[retained](../../testdata/acceptance/reviews/ru-v3-golden-review-20261011.json).
This is `PASS_GOLDEN_REVIEW_ONLY`, not model-quality or release approval. Actual
model measurements remain absent;
the lab writes `model-no-go.json` with zero actual model calls. Existing local
quality reports do not measure this corpus. Do not train/tune on holdout or infer
model accuracy from the scorer's controlled unit fixture.

Scoring requires private bounded regular files, an operator-mounted Ed25519
public key and an independently signed approval, all outside candidate source.
The unsigned approval JSON is marshalled in field order
`digest,criteria_sha256,rules,reviewer,candidate_author,signature` with an empty
signature; sign those UTF-8 bytes and hex-encode the signature. Reviewer and
candidate author must differ. Corpus/criteria changes invalidate it. The harness
has no key-generation, signing or approval-writing command. The verifier itself
must be the independent operator's pinned read-only binary; running an arbitrary
candidate's rewritten verifier does not enforce independence. Integrating that
boundary with automatic development/repair release is still pending #20/#21.

The V3 reviewer root is `/srv/storage/butler/reviews/issue33-20261011-r3`, outside
candidate source. It contains `public-key.hex` (file SHA256
`eab1a10c81977f9da1bac8174e8cfa47e15effa499bc547ce38be166316165b3`),
`approval.json` (`b2ddf7534660c004b23e43d40a583923f659cd889ebde600ec6c1ac197ad2c6b`)
and independently pinned `verifier/acceptance`
(`defbb25076f772d4078073f406aed742fc5d8ab32a84b13295be9121325e5d69`).
Run `ssh -o BatchMode=yes laba '/srv/storage/butler/reviews/issue33-20261011-r3/verifier/verify-golden'`
to reproduce signature/digest approval with zero model calls. Read-only file
modes and an independent AI role on the shared host do not protect against its
administrator. The public synthetic corpus is not a hidden benchmark. Runtime
key/binary pinning in a protected checker remains a separate release boundary.

```sh
acceptance -mode score -root /src -split holdout \
  -golden-public-key /review/public-key.hex -golden-approval /review/approval.json \
  -observations /private/observations.json -output /private/model-report.json
```

Observations contain `pin` (model, weights SHA256, runtime, hardware SHA256,
corpus digest, `mode=observation_only`) and all selected non-secret `observations`
(case ID, JSON output, measured milliseconds/input tokens/output tokens). Missing,
foreign and duplicate observations are rejected; duplicate/nested JSON keys cannot
hide a forbidden command. Exact JSON accuracy, source/command contract mismatches,
missed-important/false-important classifications and latency p50/p95 are separate.
False *initiatives*, task verification and effect errors require the actual kernel
scenario/observer ledger; they are not invented from classification labels.

Runner metadata uses exact field names and rejects missing, null, duplicate,
unknown and differently cased fields. Explicit measured zero is allowed; absent
tokens, times, boolean verification and per-task error/tool/child counters do not
become zero. Actual null model output is retained as an incorrect observation,
and duplicate keys inside the raw model answer remain available to the scorer.
Overflowing token totals deny scoring. Unknown provider usage must remain an
unavailable measurement in the runner's audit; it cannot form a complete score or
single/multi resource report by substituting zeros.

`-mode compare` reads `{single: ComparisonRun, multi: ComparisonRun}` and the same
independent approval. It requires identical code/task/resource-allowance hashes,
model/weights/runtime/hardware/data pins, task IDs and explicit pricing. All
requests, child tasks, tokens, tools, cost, memory and wall time remain in the
output. It rejects missing measurements, foreign tasks, a widened multi-agent
allowance and children in the single baseline. Measurements must come from the
independent runner's actual kernel/budget/result records, not model assertions.
No actual single/multi-agent experiment exists yet; #16/#17 are prerequisites.

Pending entries are deliberately explicit: real child-resource competition,
delegated full-cost comparison, real hardware model
pressure evaluation, skill/self-development/self-repair release and the entire
mandate-to-one-notification chain across restart/stop/delivery failure. Adding an
available boundary test is useful; it does not complete those product scenarios.
