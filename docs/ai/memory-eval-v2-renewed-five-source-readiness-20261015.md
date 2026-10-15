# Independent renewed-five source gate, 2026-10-15

Decision: **PASS_SOURCE_PREPARATION_ONLY; NO_NATIVE_ENTRY_GRANTED_HERE**.
The user's new, finite five-input instruction is prospective. It does not reopen
any old window or transfer any earlier gate. Reviewer
`/root/memory_independent_review` authors no candidate, golden or criterion and
performed no authentication, Infer, credential/import/key read or PG mutation.

## Exact reviewed source and proof

Reviewed clean release `a3d66736dcc5e7ebf94a8b1d614b0e538f387eb8`,
snapshot
`/srv/storage/butler/agents/memory-eval-78-20261014/snapshots/20261015-142056-119`.
Independent TreeHash
`d9824a33875f194085673f46aede24eea593f46ef990b8b9ea0a0855091d22f7`,
2590 files. It equals snapshot141856-402 byte-for-byte. Relative to verified
6928447/snapshot140259-668, the only changed files are the prior-pin clock helper,
its new test, and the committed historical independent WITHHELD document.

Cohort evidence is explicitly attributed, not represented as one full new-source
test invocation:

| Proof | Observed result | SHA256 |
| --- | --- | --- |
| staged692 narrow.jsonl | 22 PASS, zero fail/skip | ae3548a25a8f868295f5a1e293dcb0fd4a9ce51f48a64585870b2e51ec968152 |
| staged692 cli-auth.jsonl | 46 PASS, zero fail/skip | fd56302b05cdd2bac42d1a889bbc3f32e345caca7482024af5da72f0564278d7 |
| final helper clock.jsonl | codec roundtrip PASS | 55a5a653941aff00b6c21a3a469622cfa077353b1d91707ce8e04f8f172a8c3f |

Staged proof directory:
`reports/memory78-staged-20261015T071245Z-1578722`.
Its source-path and code-commit explicitly name6928447/snapshot140259-668.
All four precompiled binaries independently match compiled.sha256, and their
Go build info independently reports go1.27.2 and -race=true:

| Binary | SHA256 |
| --- | --- |
| acceptance.test | 7cca20076fdd7738ab2392d47721f59ea458ae997607f4976b7c42c40ee46843 |
| app.test | be68eb8b83d23a488a6e7902328e0b0dc9db920372a33527aadb23948b7a11fb |
| cli.test | 8861d9ae4d5af6fe6ee39b19ea81293e8ffdb1faaffc093a56619952bb312fb5 |
| auth.test | 47e4228db9859673fc1d7fcc1d4b249d6d97a0481b816016e456fa8e06136e33 |

The encrypted PostgreSQL import/reconnect integration and nested new-five plus
legacy mixed-prefix integration actually pass; they are not skipped. Tests ran
after reclaiming only derived compiler cache. The stopped synthetic test PG was
retained as an archive, SHA
`d11c9619ef93b8b08719ffe16837b3bbd682c2074979569809efe29c947b66fc`.
Staged command receipt completes PASS with empty build/test/vet diagnostics;
the integrating operator reports vet/bash exit0.

Final helper proof directory:
`reports/memory78-clock-20261015T072132Z-1616198`, code-commit a3d66736 and
source-path142056-119, completes PASS. It reports prior-helper vet and bash exit0;
this reviewer independently reran bash -n successfully on that exact source.
Its only new Go code is independently reviewed clock normalization plus the
actual pgx binary-codec roundtrip test. Collector, auditor, auth and launcher
dependency bytes are independently identical to the staged692 cohort.

Key raw file pins: launcher
`80a2c0519fc89cf5dbdf9ae77f46a14f2f60e3e0e0a0fa39a9cb7a80459b1d56`;
auth entry
`7bd104c0a9af8b503131a95ca0a8899e7bd52e734837423153af93d92f65022f`;
prior helper
`3c19bf2aa9dc2dbb4bd8b2ffefdea999c89db473ae2f8be1956bae960576b5d6`;
clock test
`508bc83ca65072b79bd4c7d9fc351843754d32ca035922c07b032b1efdd45a55`.

Historical failed proofs, including the exact6928447 SQLSTATE53100 disk-full
migration failure, remain preserved in
`memory-eval-v2-renewed-five-source-withheld-20261015.md`, SHA
`0cc145d9c552e8e955b8a2c9798fae556464bfae0b90cdd74aa65d4ee0d39d5a`.
This source gate supersedes that source-readiness block using successful
attributed evidence; it does not rewrite the failed logs.

Read-only hashing after the test independently used the exact sorted-path
TreeHash encoding from the checker. This is a post-run source comparison, not a
claim that this reviewer instrumented a pre-run manifest. Earlier failed runs
remain retained: 511c's migration EOF while storage was exhausted, and the
subsequent generic legacy fixture lifecycle failure. Its cause remains unknown;
a later successful rerun does not establish its cause. No runtime guard or
authority deadline was weakened. The following fixture-only change adds a typed
StartRun observation and safe failure log, without changing provider behavior.

## Substantive independent findings resolved

- The prior helper seals issued/expiry in UTC truncated to microseconds.
  Independently inspected pgx/v5@v5.8.0 binary encoding divides Nanosecond by1000,
  and decoding restores only microseconds. Unnormalized RFC3339Nano pins could
  therefore fail the auditor's exact deadline equality after durable SQL storage.
  The new test uses a non-UTC, non-microsecond input and the actual pgx codec;
  the normalized deadline roundtrips exactly and remains issued plus30 minutes.
  No historical timestamp or equality check is weakened.
- Renewal binds source, compiled binary, release/hardware-bearing plan, window,
  the immutable parent digest and ordered five-input digest. It permits exactly
  five admissions and 229679 conservative units within a newly issued window of
  at most 30 minutes. Issued time follows the expired historical deadline.
  New current leases/guards use this deadline; historical deadlines do not change.
- Five retained failed prefixes are loaded and correlated; a sixth is denied.
  Every paid attempt and all eight observed skips are excluded only from new
  execution. The original scored 72-input matrix and all retained expenses remain.
  Wrong source/commit/hardware, parent/carry/mask, expired/before-issued grant and
  a sixth admission have negative probes.
- Auth preflight verifies source/binary, filtered parent, renewal and actual
  container proof before opening private key/import. The exact entry has no
  model provider, StartRun or Infer path. It uses one fresh owned
  `memory_auth_test` database, strict encrypted PostgreSQL SecretStore and the
  same private settings key subsequently used by native execution.
- Refresh rotation is persisted before paid discovery. Native construction keeps
  already connected state, so a stale import cannot overwrite refreshed state.
  Incomplete/failed refresh or persistence stops later probes and model slots.
  At most three control-plane probes are allowed; only failed read-only
  discovery can be repeated after safe durable credential resolution.
- The diagnostics contain finite phases/outcomes and numeric HTTP status.
  JSON decode is distinct from semantic token/subscription/project validation.
  HTTP-client timeout classification also checks the returned deadline error.
  A passed probe requires actual semantic paid_discovery/completed/HTTP200.
  No raw error/body, credential/hash, account/project or URL is exported.
- Each auth probe has an immutable SQL admission before Resolve, and an immutable
  observation plus individual expense and exclusive original before any next
  call. Failed probes remain even if no native run follows. The collector imports
  each expense with its actual outcome. Resolve latency and one logical probe
  are attributed; invoice and unmeasured auth storage remain unknown. Import,
  migration/setup, physical I/O and host overhead are not measured zero.
- Native auth traces are bound to run/input and raw-file SHA in an append-only
  SQL table. Offline audit checks finite diagnostic values, safe rotation,
  probe originals/auth ledger/expense equality and every new trace. No auth
  history is invented for old attempts.
- After a clean, empty retained availability failure, only the next distinct
  frozen input is eligible. The code uses actual finite transport classes.
  No retry occurs. Protocol/tool, guard, unsafe rotation or retention errors
  stop the whole window. A narrow pre-HTTP unavailable rule permits the old
  one-dispatch/final receipt, while completed/inference cases keep exact guards.
- Auth has the minimum of its 75-second limit and renewal expiry, rechecked after
  migrations before credentials. Native execution has the same renewal deadline
  and an outer remaining-grant-plus-30-second retention timeout. Callback context
  construction occurs before the live guard goroutine; targeted race tests pass.

## Historical evidence refreshed independently

The final-six prepared pins still hash
`b3183ad2ab81f20d7207460aeab792565a543c230f1fbb5c55c80ce05cfe012f`.
Live SQL independently matches all 19 safe exported tables; the closed window
remains 112/3212748, carry111/3167573 and
2026-10-14T20:16:58.345536Z.

| Original | SHA256 |
| --- | --- |
| final-six ledger | 0ccd5a15b25c46b048aa50456465d014c3b94ac73909d9023b517968ea4cac5c |
| final-six measurements | 7047ccdf2c5c51893b031ed0ed47129fc80fb7288b028a7508e968a18e32bcbd |
| retained composite | 482d5f5cfe1c9c102db4521402e1a80a7dc0ca71413026dc93a7383b045a1a14 |
| original scored freeze | 4d7c5197bda48c7629ec3b7eafcd3a6482efc28454189328f4eaada0d419d8cd |

The composite remains 54 completed, five failed, eight skipped, five unobserved,
937 expenses and 391 logical stage calls. Original model/schema/instructions,
golden, alternatives, quote rules and criteria stay frozen. In particular golden
raw `5571d474ec54a8a29765e055cc0475e73653d679d5f250fa074dcea1d9d6b1a5`,
schema/instructions `0b10616370ea1e132c0fab74af381fe6fa4565f3c3e8a1d856bba3baf9b69f97`
and criterion `9be3b4b51b5832bffc6349915cb61db4761e4b54ee95097b43677a28b9997e68`
are unchanged. Completing these five fixed untouched slots is not a control
retuning opportunity.

## Concrete scope and remaining gate

This source decision permits offline preparation of one fresh root for
h11/full_context, h11/retrieval and the three h12 arms, with no new auxiliary
inference. It is not a native admission.

Before auth preflight, separately compare fresh prepared source/binary/five
checksums, hardware, filtered five-parent lineage and typed prospective renewal.
Measure available capacity after derived build-cache reclamation: at least
512MiB on the actual storage filesystem and256MiB on root. The helper proof's
454324224B storage snapshot is below that entry threshold and does not grant
auth. Recheck capacity after the new auth DB/migrations and before native entry;
retain any low-space failure without another call.
The compiled preflight must enforce its exact actual UID10001, image/command,
one CPU/512MiB/64PIDs, read-only inputs/rootfs, sole private writable mount,
two exact networks, required proxy/local bypass before credentials. Auth-preflight
must remain one-use and retain failed admission/probe evidence. No secret or
credential table is part of the review export.

After successful auth preflight, independently correlate its originals, safe SQL
ledger, per-probe expenses and shared durable auth-store path; then compare the
native actual runtime proof and issue a separate concrete five-slot gate.
All old failures remain failed/usageNULL; no old diagnosis is backfilled.
If any fresh pin, expiry, runtime, retention or rotation check fails, do not enter.

Only complete actual correlation of all five new attempts, old54 answers,
old5 failures, eight skips, original72 denominator and all expenses can support a
later harness-acceptance verdict. Clean provider failures can be retained outcomes
of a complete attempt matrix. This document does not establish that matrix.
Numeric quality/cost remains NO_GO and promotion denied: even before real auth
overhead, expected logical stage count445 exceeds frozen280. Strict canonical
rubric rejections are not automatically semantic mistakes, and proposed actions
are not proven external effects or privacy/kernel violations. Original pipeline,
invoice, tokenizer, physical host and broader-task outcomes remain unknown.
