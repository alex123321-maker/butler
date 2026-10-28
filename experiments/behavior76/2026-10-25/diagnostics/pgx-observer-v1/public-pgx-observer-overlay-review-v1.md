# Independent assessment: research-only pgx callback overlay

Scope reviewed before any native admission: the96 safe matrix diagnostics,
actual local pgx5.8.0 source, official upstream5.8.0/5.11.0 source, and the
public real-PostgreSQL cancelled-query/rollback reproducer. No hidden semantic
answer, golden or mapping was used in this diagnosis; no hidden replay ran.

## Observed matrix evidence

There are48 decision and48 cleanup snapshots. No snapshot reports observer
correlation failure or dropped events. Three decision snapshots and their
cleanup partners retain exactly one unmatched query start. Every actual
unmatched start has the exact normalized ROLLBACK template hash
`da25480fb483e6ce3d30f1a179c3c07e9f3b045425b0ac54acc6758a97e2db62`,
already-cancelled context, and approximately748ms remaining until its original
deadline. Scope operations/evaluations are zero and the authority reserve has
no acquired connection. The trace counter alone cannot establish a live worker.

The full matrix remains FAIL45/48, with96 scripted owner turns observed. The
earlier47/48 matrix and one-arm diagnostic retain their original dispositions.
Their observations must not be combined into a fictitious successful full run.

## Driver mechanism and limits of the inference

The actual local pgx5.8.0 `Conn.Exec` starts query tracing, then can return an
error from cached-statement deallocation without ending that trace. Its normal
return path does call the end callback. This is independently confirmed by
[official5.8.0 source](https://github.com/jackc/pgx/blob/v5.8.0/conn.go).

The public real-PG test primes a parameterized cached statement, cancels a
context, then calls the prepared query and rollback with that context. Both
operations return `context.Canceled`; the connection stays open after the
query and closes after the failed rollback. The legacy test records7 events,
one phantom open span and only one of the two actual operation errors.
That deterministic failure reproduces the callback-hole mechanism without a
study stimulus, provider, native/auth call or changed authority timeout.

This strongly explains the three persistent cancelled-ROLLBACK counters. It
does not retrospectively supply missing callback records, nor establish that
every old uninstrumented fault had the same cause. In particular, the earlier
real750ms authority observation timeouts remain unresolved; this is not a
performance repair or an exception to source/authority GLOBAL-stop.

## Exact proposed overlay

The permitted derived file differs only by the upstream nil-guarded
`TraceQueryEnd(...Err:err)` call immediately before that early error return.
The same three-line correction is present in
[official5.11.0 source](https://github.com/jackc/pgx/blob/v5.11.0/conn.go) and
documented in its [changelog](https://github.com/jackc/pgx/blob/v5.11.0/CHANGELOG.md).

Independent byte-level derivation from the actual cache produced:

* original `conn.go` SHA256:
  `076f1d636cd6e2a1b8cadaea24ac60375cd98f1d63034ff796427f49a4408684`;
* derived `conn.go` SHA256:
  `9fdbb58e77cebad5ff489cec5481b1aac2d5e111e44157605b796f7a7d8078fc`.

This scope is methodologically acceptable as a prospective observer repair:
ordinary SQL, arguments, statement cache, contexts, return values, errors,
connection handling, authority and the original Failed/OpenSpans predicate
must remain unchanged. A cancelled operation stays an error and becomes
accurately paired in the trace; it must not become a successful query or grant.
Do not synthesize an end callback in the checker or manually clear counters.

Use an explicit Go build overlay for the exact one source file only. Preserve
the shared module cache, go.mod/go.sum, ordinary production builds and Broker
binary. Both experimental arms must use the identical derived collector.
Record original/derived source, overlay-map and patch hashes, compiler/source
identity and actual ELF in prospective admission/manifest evidence. Declare
this research instrumentation amendment before execution; do not describe it
as a production dependency upgrade or deployed Butler repair.

The public before/after test must retain the same two cancellation errors and
connection lifecycle, with the corrected trace reporting zero open spans and
both errors. Existing zero-native safety/composition checks then establish the
patched observer's complete evidence. This review itself authorizes neither a
hidden replay nor a native window. Old failures, costs, unknowns, controls,
goldens, thresholds and masked judgments remain immutable.
