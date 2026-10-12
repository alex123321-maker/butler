# Private filtered-final diagnostics

The test-only observer retains only the first matching `AssistantFinal.Content`
in bounded private memory. It forwards every original transport event and EOF
unchanged, including legacy/event usage conflicts and contradictory tails. It
stores no deltas, hidden reasoning, signatures, raw SSE, requests or provider
errors. It does not authorize a provider request or repair model output.

The trusted diagnostic driver must persist owning PostgreSQL spend first, then
call `persistFinal` once. At most8192 exact bytes are written through O_EXCL,
0600 and Sync in a private operator-owned directory. Oversized content has only
length/hash metadata, without a raw file. Duplicate finals, retention failure or
repeated persistence cannot overwrite the first raw observation or its metadata.
Known and conflicting cost survive independently of filesystem failures.

Safe metadata contains only byte count, SHA256, retention flag and code-owned
encoding reason. UTF8, JSON syntax, object shape, decoded duplicate keys,
depth16 and array256 bounds are checked. This is a structural classification,
not proof of schema, source-witness correctness, clean lifecycle, model quality
or execution permission. A future driver must check all of those separately;
it cannot infer a clean lifecycle from the last failure stage alone.

Independent latest narrow proof used exact clean helper source
`35ea04cbd5f7ecf003a25232f00d4d218fa97263`, snapshot
`20261013-000130-090`, on laba:
`/srv/storage/butler/agents/scenario-private-final-helper-proof/reports/private-final-helper-20261012T170134Z-2438665`.
All24 test/subtest names across six top-level tests passed, without fail/skip,
with race/vet and actual PostgreSQL. Test events SHA256:
`1a31c608f8c1361bbbfbdbb23979886ef183b5e9fa06cfa9b9ded64b1c533858`;
source-manifest SHA256:
`943a837dac6425bfb5917da09e08a63048ec6105e166bbf36fb5dc9b21d53449`.
The database had one explicit proof/storage bind and no anonymous data volume.
Provider/auth/import/TLS/model-load calls were zero.

An earlier exact `ffc039185cefaef86bf92c4ec95d58394d5a93a0` proof remains
separate:21 names passed at
`/srv/storage/butler/agents/scenario-private-final-helper-proof/reports/private-final-helper-20261012T165645Z-2422996`.
It does not substitute for the subsequent post-SQL and immutable-metadata fix.
No old V3 collector, report, source snapshot, counter or verdict changed. The
discarded bytes of failed ordinal266 remain unavailable and cannot be backfilled.
The prospective two-input diagnostic runner still requires its own independent
review and exact-source preparation before any new inference.
