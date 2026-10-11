# Bounded generated converter tools

This specification defines the first self-development class for issue 20:
`delimited_json_v1`. Its Go developer produces an immutable declarative program
that converts strict CSV, TSV, semicolon or pipe-delimited records into typed JSON
objects. There is no arbitrary Go/JavaScript/shell compilation, expression language,
regular-expression operator, file path, URL, process, import or credential operation.

## Current implementation boundary

Migration052 retains content-free need/discovery receipts, versions, independent
reports, spent trials, catalog generations, use and owner decisions. The optional
Broker path implements discovery, development, fixed checks, bounded trials,
activation/rollback and original document conversion. It defaults disabled.
Passing a runtime test grants no capability. The actual delegated developer/task
acceptance from issue16 remains a separate gate; runtime RPC alone is not its proof.

The separately reviewed developer proposal contract is `task.delegate_converter`.
Its arguments select exact Need and synthetic-contract Catalog references, all
eight finite allocation dimensions, deadline and dependency IDs. They cannot
contain code, a working document, permissions, owner/task identity or credentials.
The selected synthetic contract itself must strictly match
`butler.converter.developer.v1`: exact Need hash, candidate version and the pinned
independent checker. Free-form narrative, working inputs, code, permissions and
author-defined criteria are rejected, even when disguised as contract DATA.
The initial context-only `task.delegate` revision remains separate and unchanged.
This contract definition alone is unregistered and grants no capabilities.
Its pending admission bridge must require an actual parent `converters.discover`
receipt and current approved dispatch; missing or stale prerequisites deny rather
than causing hidden actions. Only discover/develop/test rights may be derived for
the developer. Trial, activation and original conversion remain parent actions.

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
The Broker path uses only exact `SourceRef` arguments and resolves current
non-Secret data after authority/source/budget claim. Raw bodies must not be copied
into intents, approvals or checkpoints. The private runtime envelope contains source
revision/access revision, content hash, body hash and typed DATA origin. Classified
Secret, instruction origins, missing provenance, unknown origin fields and recognized
credential values are rejected before any workspace access. A runtime envelope is
not proof of PG authorization: that remains the Broker's responsibility.

## Isolation and operation

`tool-converter` uses Go and existing protobuf/gRPC dependencies. It has a dedicated
Broker-only service token, an operator-pinned runtime profile and a fixed socket. It
does not receive PG, Redis, model, owner, action-authority or user credential access.
Development makes a fresh 0700 workcopy under an operator-controlled temp directory,
writes only a fixed candidate filename and uses the patched Go `os.Root` file boundary.
The workcopy is removed on success, failure and cancellation. The pure interpreter
uses neither the filesystem nor external networking during document conversion.

The container profile requires UID 10001, read-only root, dropped capabilities,
`no-new-privileges`, bounded PID/RAM, and a small noexec/nosuid/nodev `/tmp` tmpfs.
The optional `deploy/docker-compose.converters.yml` uses `network_mode: none`.
Production Broker communicates only over the fixed mode0600 Unix socket
`unix:///run/butler-converter/runtime.sock` on a dedicated volume, writable only
by runtime and mounted read-only in Broker. Both use UID10001. Startup verifies
the actual executable SHA256 against the operator profile. Neither the model nor
a package can select a transport, target, binary, dependency or secret path.

## Evidence and remaining acceptance

On isolated snapshot `20261011-083222-488`, the actual production Broker and
PostgreSQL task passed discovery → development → independent checks → bounded
trial → permitted activation → original document conversion → owner review,
using the actual Docker-built runtime in a separate network-none container over
the fixed read-only Unix socket. Checked JSON matched the original document;
tool results exposed no document/program bytes and spent-grant replay was refused.
The atomic final result retained the six journal observations with an unverified
verdict. Only the fixture owner's independent document/check review and explicit
verified result then completed the original task. The runtime also passed authenticated RPC
develop → independent test → conversion.
It had no external network route, host checkout, Docker socket, core/provider/owner
credentials or writable root/token mount; temporary workspace cleanup passed. Narrow
Go tests and race tests cover malformed packages, independent-check tampering, secret
denial, wrong profiles, damaged envelopes and all-or-nothing conversion.

Actual isolated PostgreSQL MaxConns2 tests cover missing discovery, bad candidates,
finite shared-trial races, failed/unknown trials, stale generation, physical source
deletion, real owner-mandate revoke, lease expiry, rollback and spent-grant replay.
The owner UI passed TypeScript, lint and Chromium review/output-integrity checks.
The remaining gate is actual delegated developer admission with separately reviewed
exact child converter permissions and typed Need publication in that same original
task scenario. No wider generated language or quality/cost improvement is
claimed from these fixtures.

## Durable lifecycle and recovery

The six Broker-owned contracts are `converters.discover`, `develop`, `test`, `trial`,
`activate` and `rollback`. Arguments contain only SourceRefs, versions, digests and
expected generations. Development requires an actual discovery receipt under the
same current task/constraints revision, plus a fresh current tool/skill search.
An exact schema-compatible installed converter must be reused. Skill search is
bounded discovery, not semantic proof that every procedure is unsuitable.

All calls use Prepare → Authorize → irreversible Claim → execution → outcome.
Locks follow ownership → session → run/task → goal/mandate → complete sources →
converter catalog/state → effect rows. Discovery/runtime RPCs hold no transaction
or pooled connection. Publication repeats live lease/task/owner/source checks and
requires the initial generation. Independent reports are repeated in core against
the retained Need; original output is independently checked before publication.

Candidates cannot enter the working catalog. A trial slot is saved before RPC;
running, failed and unknown slots all consume the shared finite allowance and
crash/restart does not refund it. Activation requires fixed checks, an actual
passed trial with current output/parents, current ConverterPermission and exact
generation. Prepare-only tasks cannot activate. Rollback repeats these checks
for the historical version. Bad candidates leave the prior catalog/task intact.

Dynamic `converter.<id>.convert` pins active digest/generation/profile and uses
`digest:generation` as contract revision. Its acknowledgement proves only local
preparation/publication, never semantic task completion. Current source/authority
checks precede converter lifecycle checks at provider release/recovery, Broker
claim and atomic final publication. Saved observations do not revive revoked
versions or replay spent effects. Derived package/output bodies keep complete
actual parent edges; deletion physically redacts them and denies late reuse.
Migration052 rollback refuses retained history.

## Operation and owner review

Enabling requires migration052, reviewed skills051, the persistent privacy anchor,
configured owner, actual binary profile and a dedicated runtime token file. The
overlay supplies `BUTLER_CONVERTER_CONTROL_URL=http://tool-broker:8081/internal/converters`.
This route is operator/environment only; DB settings cannot redirect authenticated
relay requests. The private context reader confirms `converter_lifecycle_ready`
only after provider/recovery/final guards are composed; a Broker enabled alone
denies converter release. Missing prerequisites fail closed.

Owner `/api/v4/converters` and `/converters` expose exact version/permission/checks,
trial counts and actual use references, plus permanent exact-version revocation.
The private Broker endpoint accepts only list/revoke with verified owner provenance;
it offers no creation/activation bypass. Output review rechecks current Catalog
source revision/access/hash and raw SHA256, displays escaped text, and retains no
owner token across navigation. All generated bodies and results remain DATA.

See the [acceptance matrix](generated-converters-acceptance.md) for actual evidence
and the remaining delegated-child gate.
