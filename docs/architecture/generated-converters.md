# Bounded generated converter tools

This specification defines the first self-development class for issue 20:
`delimited_json_v1`. Its Go developer produces an immutable declarative program
that converts strict CSV, TSV, semicolon or pipe-delimited records into typed JSON
objects. There is no arbitrary Go/JavaScript/shell compilation, expression language,
regular-expression operator, file path, URL, process, import or credential operation.

## Current implementation boundary

The pure package parser, developer, fixed independent checker, private protobuf
runtime and typed owner permission are implemented. Durable need/discovery receipts,
versioned catalog admission, bounded trial/rollback, Broker routing and the original
task acceptance flow are pending integration. The runtime is therefore absent from
the production catalog and Compose stack. Passing a runtime test grants no capability
and is insufficient to complete issue 20.

## Independent need and code

The trusted need fixes the converter ID, class, input dialect/header/columns,
output names/types/source positions and finite limits. A package includes its IR
code, manifest, input/output schema, single pinned runtime dependency, additive
author tests, instructions and limitations. Unknown and duplicate JSON members,
unsupported operations, contradictory field types and unbounded resource values
are rejected. Schema hashes include all dialect, mapping and whitespace rules.

The checker receives the independently retained need. It varies each input position,
including otherwise indistinguishable boolean columns, and checks exact typed output,
quoting, delimiter characters, newlines, Unicode and whitespace. Wrong headers,
widths, malformed numeric/boolean values, invalid UTF-8, NUL, partial conversion,
byte/row/field caps and cancellation have fixed negative tests. The author's tests
are additive and cannot replace these cases, select another checker or declare pass.
Each report binds the raw package hash, need hash and operator profile hash; it stores
fixed check names without source bodies or synthetic fixture values.

String fields remain strings. Integers use signed 64-bit decimal syntax, decimals
preserve exact bounded JSON-number spelling without exponent notation, and booleans
accept `true`/`false`. A malformed later row returns no partial output. Blank CSV
lines are ignored according to the standard library CSV contract. An empty document
is rejected. Small limits that cannot admit the required synthetic cases fail testing.

## Permission and privacy boundary

`Mandate.converters` is a separate typed `ConverterPermission`: exact converter ID,
fixed class, input/output schema hashes, operator runtime profile pin, exact finite
input/output/row/field/time limits and trial allowance (1–8). It does not weaken
`ExtensionPermission`'s exact reviewed manifest rule. Wildcards, a changed schema,
larger bounds, another runtime, new executable class, revoked authority or expired
hours are denied. The mandate's ordinary tool/resource/effect scope and deny overrides
continue to govern every actual call. A converter cannot authorize another account.

Package, need, document and output bodies belong to deletion-aware Catalog DATA.
The planned Broker path uses only exact `SourceRef` arguments and resolves current
non-Secret data after authority/source/budget claim. Raw bodies must not be copied
into intents, approvals or checkpoints. The private runtime envelope contains source
revision/access revision, content hash, body hash and typed DATA origin. Classified
Secret, instruction origins, missing provenance, unknown origin fields and recognized
credential values are rejected before any workspace access. A runtime envelope is
not proof of PG authorization: that remains the Broker's responsibility.

## Isolation and operation

`tool-converter` uses Go and existing protobuf/gRPC dependencies. It has a dedicated
Broker-only service token, an operator-pinned runtime profile and fixed ports. It
does not receive PG, Redis, model, owner, action-authority or user credential access.
Development makes a fresh 0700 workcopy under an operator-controlled temp directory,
writes only a fixed candidate filename and uses the patched Go `os.Root` file boundary.
The workcopy is removed on success, failure and cancellation. The pure interpreter
uses neither the filesystem nor external networking during document conversion.

The container profile requires UID 10001, read-only root, dropped capabilities,
`no-new-privileges`, bounded PID/RAM, and a small noexec/nosuid/nodev `/tmp` tmpfs.
The isolated validation helper uses `--network none` and real production runtime
RPCs over loopback; production Broker connectivity will require a separate narrow
runtime network while the IR retains no outbound operation. Network-none validation
is not a claim that a connected production container has no network route.

## Evidence and remaining acceptance

On the isolated lab snapshot `20261011-073547-361`, the actual Docker-built production
runtime passed develop → independent test → conversion over authenticated gRPC.
It had no external network route, host checkout, Docker socket, core/provider/owner
credentials or writable root/token mount; temporary workspace cleanup passed. Narrow
Go tests and race tests cover malformed packages, independent-check tampering, secret
denial, wrong profiles, damaged envelopes and all-or-nothing conversion.

The remaining acceptance gate is the complete ordinary owned task/Broker flow:
existing tool/skill discovery, finite need admission, source-linked package publication,
same-transaction current permission and catalog generation checks, fixed tests,
bounded trial, automatic permitted activation, exact document conversion, immutable
verified result and failure preserving the prior catalog and original task. No result
or cost improvement is claimed from the isolated runtime alone.
