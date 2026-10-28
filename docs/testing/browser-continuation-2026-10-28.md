# Browser continuation: historical stop recovery

This is the first recovery slice of #163, stacked on baseline PR #171. It does
not complete the browser-interaction contract or the remaining native matrix.

## Implemented and applied

Four historical uncertain stops blocked native nav-01 before browser dispatch.
Inventory correlated exact original Input/run/owner/session and effects without
exporting arguments, messages or credentials. Three unknown actions belonged to
the historical atomic local publication protocol; none had its own publication.
Same-run widget/blocked publications were unrelated and were not used as their
receipts. The fourth was a pinned read-only web.fetch with unknown result.

The new owning recovery adapter runs inside terminal Input/session/run locks.
It validates original canonical binding/hash/fingerprint and the reviewed
historical contract. Pinned local publication absence after fencing gives
effect_absent; an exact canonical publication receipt can give local confirmed,
independently of delivery. A pinned read gets append-only no-write proof while
its result/effect stays unknown. Current/unsupported contracts, arbitrary writes,
foreign/malformed receipts, active execution and changed grants remain denied.
No current generic browser-fill contract is covered by this compatibility path.

The default operator command rolls back its transaction and reports predicted
classification separately. Actual dry run on laba predicted known for all four,
while durable classification stayed uncertain. The checked identical binary was
then applied to those exact four references: three local effects became absent;
the read remained unknown; all four stopped-isolation classifications became
known. Failed Inputs/runs stayed failed. Before/after retained-history hashes
match across Inputs, runs, model usage, grants, dispatches and publications.
There was no model/runtime/delivery call or old work replay during review.

Evidence is under [continuation](browser-baseline/continuation/): dry/apply
reports, original/final states and the operator binary hash
`9e3aa96b828a77ae823ba75739bb87f76ef22a43829de8b9dbd34771e27341ea`.
Only this one-shot kernel operator was executed against the existing stack;
the dirty main Core/Broker/browser binaries were not replaced by a claimed
reproducible build. The source change also composes review into maintenance
after deployment; that long-running deployment has not been performed.

## Validation and operational constraints

Local portable PostgreSQL + race passed publication/read/fencing negatives.
Full Linux Go1.27.2 + isolated pgvector17 passed **83 selected tests/subtests,
zero skips/failures**, including real Input/Communication composition, dry-run
rollback, retained usage/grants, fresh-owner-reply limits and existing uncertain
write denial. Vet and the operator build passed. No dependencies or migration
were added. Reproduce with `scripts/dev/stopped-continuation-tests.sh OWNED_ROOT`;
`scripts/dev/stopped-local-review.sh` runs exact dry/apply review with a private
database configuration file, never a DSN argument.

The first disposable validation PostgreSQL run ended with EOF/recovery errors.
Its failed JSONL log remains in the owned remote reports; it was not counted as
passing. The successful fresh run used larger bounded tmpfs, bounded WAL and
private PostgreSQL logs; its OOM flag was false. Test containers were removed.
Source and build cache used `/tmp/butler-browser-continuation-20261028` because
the main storage volume was full. Copy durable evidence before reboot/cleanup.
Raw test reports were also copied into the worktree's private `.cache` directory.
The existing mandatory Communication CI selector includes StoppedLocal cases;
that suite now also vets/builds the operator without executing it.

The first live dry run also encountered PostgreSQL's explicit no-space failure
before review began. Only immutable Go build-cache `*-d` files older than two
days under the verified `/srv/storage/butler/.cache/go-build` path were removed
(4191 entries, 3,774,931,537 bytes by preflight). Source, database and journals
were retained; 3.6GiB became available and PostgreSQL accepted connections.
Storage capacity remains an operational concern; no general cache/database
prune, filesystem configuration change or service restart was performed.

## Native continuation: nav-02

The fixture was updated for the already reviewed `Arctic delivery` query;
deployed binary SHA256 is
`2520e5e4628fa91dffbb3678c79a98ea25a5b8adec53a28210f1d620c5ea1482`.
Previous recipient journals were retained. nav-02 used the next preplanned
matrix slot after formal recovery, not a new session to evade uncertainty.

Input `input-061938dee29caaafe4810062c8e36178`, run
`run-1793187500406146660-d1561e6a7d75fb3c`, ordinary owner ingress and existing
Gemini profile. The agent reached a separate browser.fill approval in 51.099s,
after seven model attempts (51,157 input / 706 output tokens), three discovery
searches, tool load, browser open and snapshot. One operator intervention
approved exactly `input` / `Arctic delivery` / the public `/catalog` URL via the
normal owner API. A first GET observer invocation had a CRLF duration argument
error after approval; correction performed GET only, without another POST or
approval. It observed the same original Input.

Unlike nav-01, fill reached the runtime (11:40:21.440–11:40:21.640 UTC): returned
`ok=true`, then Broker returned effect_unknown under its actual effect=unknown,
retry_safety=unsafe, verification_method=none contract. The old
stopped_attempt_uncertain pre-dispatch blocker was therefore removed. The Input
failed with a new uncertain stop. Post-call network_state=read_only does not
prove that no external mutation happened during the operation, and was not used
to manufacture a successful effect. No final answer was published or delivered.

**General acceptance remains NO_GO.** Matrix: 5/15 native slots attempted,
three fresh reads pass, nav-01 and nav-02 fail at distinct boundaries; remaining
ten slots are unexecuted. nav-03, forms and credentials must not use new
session/Task identities to bypass nav-02's actual unresolved effect. The next
#163 slice needs supported browser interaction evidence and continuation, plus
the protected owner credential flow for login. Existing contracts cannot prove
arbitrary remote acceptance from DOM success or from the fixture oracle.
After terminal failure, nav-02's recipient was stopped with its journal intact.
A fresh `infra-after-continuation` instance serves HTTPS; it is not a matrix slot.
