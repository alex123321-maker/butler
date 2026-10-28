# Research observer callback correction

This additive build note leaves the frozen fixture-local protocol, candidate,
criteria, corpus expectations, original failures and all closed native windows
unchanged. No new native study has been admitted by this note.

The full r2 zero-native composition failed45/48 arms with all96 owner turns
observed. The96 exact decision/cleanup snapshots established three unmatched
ROLLBACK starts after cancelled schedule checks; no events were dropped and the
observer did not report correlation loss. A public real-PostgreSQL reproducer
then demonstrated pgx5.8.0 returning from both cancelled operations while
reporting only one return callback. The connection remained open after the
cancelled query and closed after the failed rollback. These raw failures remain.

The upstream [pgx5.11.0 correction](https://github.com/jackc/pgx/blob/v5.11.0/conn.go#L440-L449)
adds the missing nil-guarded `TraceQueryEnd(Err:err)` before the existing early
return. The [changelog](https://github.com/jackc/pgx/blob/v5.11.0/CHANGELOG.md)
documents that fix. Only these three callback lines are applied to an isolated
copy of the existing5.8.0 module for this research build. No SQL, result, error,
connection handling, execution mode, statement cache, timeout or authority check
changes. Both study arms use the same compiled copy; the unchanged frozen Broker
has no research query tracer. This does not repair or explain the historical
750ms authority deadline.

`scripts/dev/behavior76-pgx-build.go` requires original conn.go SHA256
`076f1d636cd6e2a1b8cadaea24ac60375cd98f1d63034ff796427f49a4408684`
and derives only
`9fdbb58e77cebad5ff489cec5481b1aac2d5e111e44157605b796f7a7d8078fc`.
It retains a complete module copy/inventory and a separate `research.mod`/sum
with one local replacement under the repo cache. The research runner alone uses
explicit `-modfile` and `-mod=readonly`. Original go.mod/go.sum, the shared module
cache and ordinary builds are untouched. Existing derived files must match
exactly; unexpected files or different hashes fail closed.

The first attempted Go `-overlay` mechanism was rejected because Go1.27 forbids
replacing files under GOMODCACHE. Its failure is retained; it was not used to
produce the corrected successful build. The actual mechanism is the isolated
research modfile. The native manifest records its exact pins and complete module
inventory after comparing original/copied bytes. Producer commit and actual ELF
remain separate recorded identities. Existing public/setup/composition/resource
admission checks still apply, with no predicate or threshold relaxation.
