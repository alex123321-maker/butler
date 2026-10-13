# Actual V10: source-check cancellation retained at557

The once-only native scope on0f9b61e23fd56fefddd1c42cd3aa9efa14087d83
admitted one parent start, ordinal557. It was cancelled after13506ms while the
request-local stage was `http_round_trip`. No provider terminal/final packet,
usage or model version was observed. Input, output and total expense remain NULL.
The known new subtotal0 is not an assertion of zero expense.

The actual Core/input failure was `execution_stopped memory_source_check_timeout`,
with an additional cancelled-context failure while transitioning the run. The
provider observer separately retained cancellation/`transport_connection_error`.
These are distinct observations. This is not a proved nominal provider timeout
and cannot enter the safe model-retry exception. The underlying source-check
queue/query/lock cause remains UNKNOWN. A bounded, value-free inspection of the
own PostgreSQL shutdown log found no recorded known deadlock or query cancellation;
absence of such a record does not establish the missing underlying cause.

The parent task became blocked at revision2, input blocked and run remained
model_running in the retained witness. No live lease, action, child, adapter,
original result or notice existed in this scope. One unverified model-run failure
result is not a verified final result. Normal failed; the other four cases did
not run. The native process exited1 without OOM before its global deadline.

The separate read-only auditor c1a4b7b5fc2b82731bc95db0100c0a6734c18c3c passed
22 exact Linux race names, zero failures/skips and empty vet diagnostics.
Auditor Tree44cf4ef237cd696dce60b6df2f025ea107facf7536385947275a3155739519d2;
binary97ee9948b24950f0d134a0daf34a94475308f0e6924c9587634f21d2d328adb4.
The final review mounted thirteen safe named leaves and an empty proposal
directory, without private provider/auth/SSE/profile directories. All eight
budget dimensions were recomputed from six reservations/seven scopes.

Canonical NO_GO5376bytes/SHA
28b4dbc9cdfbd8fb8e838b2f14bb698b7142e587f0a44ec2e534a716dce62534;
failure correlation8528bytes/SHA
f7c20a7246eb2fc5a653f39f4bfe7f06498e152be3d50c4b523357eff163af8d.
History557 is111374bytes/SHA
fb85f5ce35a5217990d83af664060908574c0ac0682fa8d111e20ac0c2deef0f;
carry3025bytes/SHA78d320c595483d666b25cbaee55be764b53b0004bdd6806ecb4924e4f01130ef.
Foldfb4b085728544f0709dbc58dc7d5a3ba678412cc6a0c96514363a6fab2c27dee
appends the spent NULL attempt without filling, refunding or reopening history.
Cumulative known subtotals remain475637 input/15159 output/793703 total,
with528 known pairs,29 unknown pairs and49 unknown complete totals. Old627 has
70 unused admissions; earlier615 windows remain closed. Neither allowance nor
model admission follows from the historical carry. All accepted retry subsets
remain zero; issue readiness remains false.

```bash
bash scripts/dev/release-chain-v10-final-audit-reproduce.sh \
 /srv/storage/butler/agents/scenario-release-chain-v10-high-20261013/reports/release-chain-process-20261013T164949Z-2616202 \
 /srv/storage/butler/agents/scenario-release-chain-v10-auditor-20261013/snapshots/20261013-235725-056 \
 /srv/storage/butler/agents/scenario-release-chain-v10-auditor-20261013/reports/independent-checker-c1a4b7b-20261013T165849Z-2645741/bin/chain-final-audit \
 c1a4b7b5fc2b82731bc95db0100c0a6734c18c3c \
 44cf4ef237cd696dce60b6df2f025ea107facf7536385947275a3155739519d2 \
 97ee9948b24950f0d134a0daf34a94475308f0e6924c9587634f21d2d328adb4
```

Expected result is NO_GO and exit1. Read-only reproduction never opens a new
native scope. Earlier accepted quality/comparison and partial adapter evidence
retain their own sources. Final current-code verification and five actual
original-result/fault cases remain required before readiness, merge and closure.
