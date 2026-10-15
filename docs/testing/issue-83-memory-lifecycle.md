# Issue83 lifecycle gate acceptance, 2026-10-16

The isolated `ssh laba` run passed against code commit
`e5061ae3f0665b07043dbf54aab7869d3588bb30` (base
`cc751b2c2aef446ef8fffaad357cf3a57892e66a`). The final evidence-only report commit
does not change tested code. No production memory mechanism needed a logic fix;
this change adds shared adversarial fixtures, regression coverage and the
mandatory consumer acceptance contract.

## Reproduction and original evidence

```sh
bash /srv/storage/butler/agents/issue83-20261016/current/scripts/dev/memory-lifecycle-lab.sh \
 /srv/storage/butler/agents/issue83-20261016 verified
```

`verified` is consumed: the script refuses to overwrite evidence. A reproduction
must stage the reviewed source in another snapshot and use a fresh label.

Original source: `/srv/storage/butler/agents/issue83-20261016/snapshots/verified`.
Original evidence: `/srv/storage/butler/agents/issue83-20261016/reports/verified`.
`complete` is PASS. The script checked every exact mandatory test name and found
no mandatory skip/failure. It retained 198 test/subtest PASS events (117 top-level
events, including three independent backup phases; 81 subtests), zero failures
and four optional skips. `go vet` passed for catalog/jobs/semantic/Broker.
Disposable PG/Redis containers and the private network were removed afterward;
source, JSONL, private custom backup and mode600 independent anchor remain.

| Artifact | SHA256 |
| --- | --- |
| Whole-source file manifest `source.sha256` | `0cad071610b376db58896159d06857e1c754c9eed3e8ffc8dd32d66c582986e8` |
| Corpus `lifecycle-v1.json` | `35eeedcf23b6444c74c57eb1f1a62c53fe6293be0d0627b1728f666019ee487c` |
| Complete evidence manifest `evidence.sha256` | `d6a95048c40ea38269eef533b13ef8e291c9ca9cc975c8f58fdf496d85f26fb8` |

The [gate matrix](../ai/memory-lifecycle-acceptance.md) maps each lifecycle
boundary to its named executable evidence. All eight public synthetic attacks
ran through catalog retention/two-level dependencies, current source reads,
candidate-only publication, selective forgetting and Broker zero-dispatch. The
durable quote consumer also ran all eight through publication/repeat/forget.
Current read-only mandate JSON, revision and status survived selective removal;
no new permission was created. Two semantic generations and fresh bundles were
checked after correction/deletion. Actual Redis bytes were purged by the
production cleaner while another owner's cache remained. Existing real PG
provider-callback, expiry/wait, killed-worker, privacy and prepared-context tests
passed, including retained spending on rejected late results.

Actual `pg_dump -Fc` preceded deletion. Actual `pg_restore --exit-on-error`
restored that old snapshot into a fresh dedicated test DB. Reads failed closed
before replay of the newest external tombstone anchor; originals, children,
claims and chunks were erased afterward, reindexing was denied, and an unrelated
original/confirmed preference remained readable.

## Explicit limits and skips

- `TestActualHybridRetrievalQuality` and `TestActualLocalRetrievalQuality`: real
  local inference service not configured in this offline kernel fixture.
- `TestActualOwnedSemanticAuthority`: deployed local service private file not
  mounted. Deterministic current CatalogGuard/job authority cases did run.
- `TestLegacySummaryCoverageMigrationIntegration`: requires an exclusive legacy
  summary migration database. Current coverage/privacy/stale publication tests ran.

No real credential file, owner memory or provider API was read. There were zero
external inference calls and no internet requests: cached dependencies,
`--pull never`, `GOPROXY=off`, `GOSUMDB=off`, internal Docker networking. The main
Compose/deployment was untouched. This accepts the existing lifecycle kernel,
not semantic model quality or future summary/lesson/consolidation consumers;
those must add their applicable gate tests in their own changes.

Earlier private `first-pg` and `final-1` runs remain retained. Their tests had no
failures, but overall acceptance was withheld respectively for a mandatory
backup-test skip bookkeeping error and host-side hashing of the private
container-owned anchor. Both harness problems were corrected before the exact
final run. Anchor permissions were never relaxed.

Previously delivered/exported external copies and unknown offline backups remain
outside Butler's recall guarantee; no external erasure or effect undo is claimed.
