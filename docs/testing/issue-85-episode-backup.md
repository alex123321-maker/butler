# Issue85 accepted-episode backup/restore gate

`TestActualPostgresCandidateEpisodeDumpRestoreDeletionIntegration` covers the
new migration073 accepted episodic projection rather than treating the generic
profile/claim restore fixture as evidence for an unseeded branch. The driver
requires all three real PG/race phases to pass without skips. Its transport is
a controlled fixture: no native inference, extraction-quality or invoice-cost
acceptance follows from this privacy proof.

## Exact procedure

1. Apply migrations to a fresh dedicated backup database. The real candidate
   `ModelWorker` publishes two neutral `outcome_episode` claims through its
   ordinary transactional handler. Actual owner HTTP decisions project both
   to confirmed `memory_episodes` with null embeddings. Their current catalog
   guards must pass before any deletion. One is unrelated and must survive.
2. Save a private0600 receipt containing only identities, revisions and hashes.
   Hash complete published attempts, observed usage, reservations and cumulative
   budget scopes. Make an actual custom `pg_dump -Fc` **before** deletion.
3. Delete the selected source through owner HTTP controls. Its private independent
   anchor remains outside database backup rollback. Restore the old dump with
   `pg_restore --exit-on-error` into a different fresh dedicated database.
4. Before replay, original/result/unrelated reads, both accepted-episode guards
   and a repeated owner decision fail closed. Reconcile the newest unchanged
   anchor twice. The second replay must be idempotent.
5. Verify the selected `memory_episodes` row is physically absent, claim text and
   source/result revisions are empty, the accepted head is cleared, and old
   source hashes remain unchanged. Catalog access, object eligibility, owner
   reacceptance and source recreation deny resurrection. The unrelated accepted
   episode's entire row hash, claim/head and both source/result pins remain
   unchanged and readable. Complete paid attempt/reservation/scope hashes remain
   identical, including both observed usages and publication receipts.

The explicit database names are `butler_candidate_episode_backup_test` and
`butler_candidate_episode_restore_test`; the test rejects other databases.
Each process uses MaxConns2. The fixture helper accepts an existing pool/manifest
without initializing or reconciling it, so setup cannot bypass the fail-closed
restore checks. The ordinary fixture still initializes its own private anchor.

## Reproduction

Stage the reviewed source beneath an isolated issue83 root on `ssh laba`, use a
fresh immutable `snapshots/<label>`, and point `current` there. Then run:

```sh
bash current/scripts/dev/memory-candidate-backup-lab.sh "$PWD" fresh-label
```

The script refuses to overwrite evidence. It uses cached images/modules, an
internal Docker network and a storage-backed PostgreSQL bind with finite CPU,
memory and WAL bounds. It touches only its own containers/network/test databases.
The dump, private anchor/receipt, JSONL, hashes and PostgreSQL data remain after
disposable container cleanup. Existing candidate HTTP/revision/privacy tests and
vet additionally check the test-fixture refactor. Ordinary package execution
skips the external test; that skip cannot qualify this gate.

## Evidence

The isolated `ssh laba` procedure passed on 2026-10-16, from base
`cf5bc3239e95a76118a0dae710e6e1fc4d53535a` plus this test/fixture/driver change.
Its immutable source snapshot is
`/srv/storage/butler/agents/issue83-episode-restore-20261016/snapshots/v2`;
original evidence is `reports/attempt-v2` under that same root. `complete`
contains `PASS`, and `summary.json` records **11 PASS,0 FAIL,0 SKIP**: three
explicit seed/delete/restore phases and eight existing candidate HTTP/episode/
late-change regression test/subtest events. `vet.log` is empty; the driver exited0.

| Artifact | SHA256 |
| --- | --- |
| Whole source manifest `source.sha256` | `447daad1d4cac9c5dce29c75748a847f1c566fa77fd60e0c28c655454de8c2e1` |
| Evidence manifest `evidence.sha256` | `a305891064fde6b90f758cf7cc32e7f4caa87ac6ca74dd12013f3ebd7da6e3b1` |
| Real old custom `before-deletion.dump` | `138381d215bddd91345d0380776eb230e2f3929bc069de2c03b16f36ce0d5955` |
| New phased test source | `1d2807726ba15e81e3c9279eb4ed094e50884dc7158be717c3ea6e98e8a95b96` |
| Backup driver source | `20348d7b730d778d04eb5b22e198bb0810972b4bf77a5d7b2756e7d565554169` |

The selected old confirmed episode and populated source/result revisions were
observed in raw PostgreSQL immediately after actual restore. Normal source and
accepted-object access nevertheless denied release before replay. Current-anchor
replay physically erased that episode, nulled its claim projection pointer and
cleared its claim/source/result text; the second replay left the complete tested
business-state hash identical. The unrelated confirmed episode's entire row hash,
owner decision and source/result identities survived unchanged. Both original
published attempt rows, observed fixture usages, reservations and complete shared
scope state matched their pre-deletion digest. No runtime fix or waived assertion
was needed.

The driver removed only its own disposable PostgreSQL container/network. Private
anchor/receipt, old backup, source and original evidence remain retained. Staged
snapshotv1 was never executed; v2 is the first executed procedure. This final
evidence prose was added after the pass; tested Go/driver bytes are unchanged.
No auth/provider API or main data was accessed, no external inference occurred,
and this proof does not change production candidate qualification.
