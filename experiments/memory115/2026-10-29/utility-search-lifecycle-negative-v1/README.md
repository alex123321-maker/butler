# Search, source lifecycle and typed negative observer proof

PASS for controlled observer integration on laba: **10 scenarios, zero failures
or skips; 64 negative controls**. Of the controls, two perform actual Catalog
revision update/revoke; the remaining controls corrupt observer evidence. This
is not the full native A/B/C requested by the owner: that part is incomplete.

The actual reviewed-skill fixture now searches through Broker, selects the
returned digest and loads it before inspect/prepare/check. All five calls have
matching action/dispatch/effect evidence; Core's durable refs are checked against
the exact Catalog manifests and Broker result digests. The raw trace has one
owning skill usage and six synthetic transport attempts. Search descriptions give
no load or task-success credit.

After a successful memory read, the test changes the actual source revision
through Catalog's revision-CAS API and captures again. The old observation must
fail currentness. It then revokes the current source through Catalog's deletion
anchor workflow and repeats the check. No product guard is weakened.

An additive read-only recipient query validates exactly `ready=maybe`, reports
the typed `unsupported_ready_value` and derives absence from its actual empty
effect store. The evaluator verifies exact query/resource/format/value/reason and
Broker evidence. The correct final passes; a contradictory valid-JSON final fails.
The previous generic mutation rejection remains blocked UNKNOWN and incomplete.
The typed validation is not reconciliation of a prior unknown mutation.

Code commit `47829173`: 2,463 Go/config/migration/build-script blobs matched Git.
Go 1.27.2 acceptance race tests, adapter/app vet and actual race-enabled app tests
passed. See [build log](build.log) and [summary](verification-summary.json).

Root: `/srv/gitea/butler-acceptance/issue115-utility-trace-complete-20261029-r3`.
Private suites, bindings, original snapshots, scores and raw test stream remain
there. PostgreSQL is stopped cleanly and the internal network removed. No running
Butler deployment changed. Native/Gemini requests across r1-r3: **zero**. No old
native archive or expense ledger was opened for writing; synthetic usage stays
outside native totals.

Failed r1/r2 roots remain intact. r1 found two test errors: the update omitted its
expected source revision, and the typed-negative observer incorrectly required
typed proof for the earlier generic UNKNOWN inspection. r2 fixed the latter and
retained the former failure. r3 fixes the expected revision and passes all checks.
The first public audit accidentally carried the prior audit's stale list of
remaining work; additive `public-summary-v2.json` corrects that metadata without
changing any measured count/hash or replacing the original.

Remaining native work: implement the complete 24×3 collector with prior source/
note/skill preparation, common current-document access for every family, native
slot lifecycle materialization, actual original-UNKNOWN reconciliation lineage,
balanced frozen ordering and full historical ledger admission/correlation.
The old 48-control prose collector cannot supply these new formal observations.
No new native cohort was started and #115 is not qualified for closure.
