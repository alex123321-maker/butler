# Issue89 consolidation evidence

Status: implementation and controlled boundary acceptance pass; native semantic
and paired full-cost qualification is pending. `consolidation.ReleaseQualified`
remains false. Both process opt-ins default false, and PostgreSQL model control
starts disabled. No primary deployment/auth/provider state was changed by this
consumer's controlled checks. No #89 native attempt has been made.

## Frozen controlled evidence

All runs use `ssh laba`, existing pinned Go1.27.2/PG17 images, repository-cached
modules with network disabled, isolated schemas/databases, storage-bound PGDATA
and content-free receipts. Custom backup and ordinary drivers clean up only
their own containers/networks. Commands are under `scripts/dev/`.

| Evidence | Source | Result and scope |
| --- | --- | --- |
| `context-v1` | `d8be8bb8` | PG/race jobs24.846s; all-context authority/fingerprint, fourteen current mutations before HTTP/during call, atomic shared root/mandate/day budgets; pure grouping1.048s |
| `context-v2` | `340161dc` | PG/race jobs53.605s, budget1.020s; legacy nil-policy retry/publication, atomic prepare rollback/staging, independently depleted root and earliest context deadline, supervisor regression |
| `runtime-v4` | `f9c7f21b` | PG/race consumer25.757s plus private owner API/closed registry; all-eight actual publish/select/read/retry/correct/forget/reconstruct, current accepted/temporal changes, head CAS/ABA, retention and honest cleanup failure/retry |
| `backup-v1` | `f9c7f21b` | Six mandatory phases,0 failures/0 skips: real custom dump before deletion, separate fresh restore, newest independent anchor and idempotent replay for direct source and original→candidate ancestor |
| `ordinary-v1` | `d8cfd3d9` |24 pass events,0 failures/0 skips. Actual Broker binary/private context+reader, authenticated durable input→ordinary Flow→owning worker→sealed result, independent raw/consolidated tasks. App read control2.610s, complete app target3.656s |

Server root: `/srv/storage/butler/agents/issue89-20261016`. Reports are immutable
under `reports/<label>`, with source manifests and hashes. Backup and ordinary
drivers additionally require `complete=PASS`, no failed/build-failed/skipped
events, and retain evidence/receipt hashes. Private backup receipts contain IDs,
revisions, hashes and head/spending identity, no copied source text or auth.

The backup checks physical erasure of input/result/accepted values and every
original/candidate ancestor, preservation of exact original observed attempt,
reservation and budget-scope hashes, denial before reconciliation/after restart,
unrelated source and accepted-decision retention, and no deleted rollback.

All-eight attacks traverse this consumer's real atomic publication and owner
activation. Current task goal/status/constraints/revisions, mandates and UNKNOWN
effect remain unchanged; the ordinary deletion mechanism's Working Memory
redaction is asserted separately. Unactivated outputs are unreadable DATA,
immutable generations cannot be edited into instructions, and repeated one-use
policies perform no second inference. These controls do not score model semantics.

## Preserved development failures

Earlier logs remain under `reports/runtime-v1.txt`, `runtime-v2.txt` and
`runtime-v3.txt`. They are not converted into passes or counted as paid inference:

- v1 used `Decide` on an already confirmed claim; that API accepts only a pending
  candidate. The corrected scenarios use actual candidate supersession and
  ordinary source revoke/delete, independently exercising the current guard.
- v2's all-eight setup omitted the unrelated task's session FK. It failed before
  those cases reached the consumer. The isolated fixture now creates that session.
- v3 compared all task bytes after deletion, although existing privacy deletion
  intentionally clears Working Memory. The assertion now keeps all authority,
  goal/status/constraint/version/effect fields and separately checks redaction.
  Its injected anchor Append error also occurred before any deletion commit;
  the corrected test verifies retained quarantined content/pending cleanup and
  healthy retry, rather than falsely expecting a divergent anchor watermark.

Independent review found a production guard mismatch in owner review: an old
selected root was described as a metadata lock but was also included in the new
target's summary/temporal proof. `b81c75d3` separates proof IDs from union lock IDs.
The metadata-only stale-selected→fresh generation CAS positive and stale rollback
negative pass on actual PG. No obsolete permission is used to read an old value.

## Remaining release gate

Compose the latest independently reviewed #88 ordinary typed read hook first.
Then freeze and review a prospective one-use native corpus/window through the
existing subscription transport and server proxy. Compare identical authorized
source/current-task questions before/after owner activation, counting every real
start/tool-result continuation and all foreground/background conservative quote,
known/unknown usage, bytes, latency and subscription cost basis. The finite
allowance must cover actual tool read loops; three paired questions do not imply
exactly nine transport entries. Keep every failed/unknown observation permanently.

Native grouping must retain all complete unique evidence/exceptions/failures,
associate exact repeats, leave historical differences visible and never turn
frequency into acceptance. A fresh correction/diff/head CAS and current accepted
fact metadata must remain distinguishable from unconfirmed observations. A smaller
foreground read alone does not establish a total cost reduction. No savings,
general quality or release qualification is claimed until that evidence exists.
