# Agent security boundaries

This contract complements the Tool Broker, credential, ownership and memory
contracts. Instruction classification never grants permission to execute an
action. The Broker checks current owner authority independently of the model.

## Context provenance

Only code-owned kernel instructions, the versioned operator prompt and current
PostgreSQL owner policies enter a provider as system instructions. Prompt
placeholders, retrieved documents, tools descriptions, working scratch,
transcript summaries, memory candidates and child observations enter as data.
The provider boundary downgrades legacy untyped system items. Derivation retains
source and parent references, raises sensitivity conservatively, and lowers
instruction authority even when summarizing an authentic owner message.
Secret-class observations cannot enter model input. Extraction cannot assign
its own authority or erase sensitivity; persisted legacy records default private.
Historical metadata retains sensitivity and source references. The complete
transcript is checked before a covered summary suppresses its prefix. The trusted
operator instruction reference pins a digest of its exact content.

Complete transcript tails remain complete conversational items. Their user or
assistant roles do not make their contents current owner policy. Native live
function responses retain the adapter's private preceding call and thought
signature. After restart, known tool results are Butler observations; they are
never orphan native function responses or instructions to repeat an action.

## Network and credential boundaries

HTTP and Chromium use a checked public-network transport. It rejects credentials
in URLs, private, loopback, link-local, reserved and ambiguous IP literals, and
ports outside the operator policy. Every DNS answer must be public; the checked
IP is passed directly to dial while HTTPS verifies the original hostname.
Redirects are returned without following them. Caller Host, proxy, connection,
authorization and cookie headers cannot replace the mediated destination/auth.

Chromium receives a private proxy bound to one approved public origin and an
expiry. HTTP requests and CONNECT destinations pass the same DNS check.
Subresources remain within that origin, service workers and WebSockets are
disabled, and ordinary observations permit GET/HEAD only. Authorized interaction
temporarily enables write requests. Existing Browser session guards still check
owner, exact current URL, observation version, scope and exclusive ownership.
Proxy expiry destroys open tunnels. The session shares a 4 MiB outbound cap;
HTTP requests and a webfetch batch also enforce a 4 MiB cap before sending.

Browser local preparation freezes both page routes and the proxy persistently,
including established CONNECT tunnels and in-flight HTTP. Delayed autosave cannot
send the newly injected draft during later observations. Only a separately
authorized click opens a bounded window after live guards, then refreezes; its
DOM acknowledgement still cannot prove external submission. Read/open/navigate
revision 2 enforce GET/HEAD, while legacy live fill/type remain opaque. See the
[browser session contract](browser-session-contract.md) for exact proof semantics.

Resolved credential and cookie values, including common URL/base64 variants,
are masked from returned HTTP/browser observations. Cookies and arbitrary
response headers do not appear in HTTP results. Helper stderr and network errors
cannot disclose a URL, request body, credential or provider response body.
The credentials remain deferred Broker inputs; no runtime fetches SecretStore.
Pattern redaction supplements these boundaries and is not a permission source
or a guarantee of discovering every arbitrary secret in user text.

Public DNS eligibility does not permit disclosing private context. The Broker
requires exact current owner approval or an assigned current owner-goal mandate
with a confirmed purpose and resource scope for arbitrary network reads too.
The canonical action hash binds path, query, headers and body. A changed query
cannot reuse approval. Revocation, task policy and public literal validation run
before credential inspection or DNS. Operator-owned AllowedDomains is explicit
release permission within the declared contract; an empty declaration grants
none. This is a conservative declaration rule, not a content classifier claiming
that arbitrary query text is public. A legacy origin-only mandate cannot waive
this release guard. Runtime allowlists are still enforced after approval.

The operator-configured self-hosted webfetch service is a fixed destination
exception: redirects are denied, outgoing requests are bounded, and requested
resources are checked before delegation. The operator must apply equivalent
DNS/redirect isolation at that service. Its private network is not controlled by
Butler's dialer. Jina's optional operator service token is not an agent-selected
credential and is removed from reflected content.

Migration037 reserves conservative EgressBytes and ProviderInputBytes across
the task root, current assigned mandate and owner day. The tool reservation is
in the irreversible grant claim transaction. Current network runtimes enforce
the independently fixed 4 MiB quote; a webfetch attempt/fallback batch shares its
private counter. A Chromium session shares its cap across commands and tunnels.
The native adapter checks the actual complete serialized history, tools and
results before inference. Other HTTP/WebSocket adapters also apply serialization
bounds, but remain unavailable for budgeted inference until they declare verified
model capacity/cost limits. No model-visible argument can increase a cap.

Untrusted provider HTTP errors retain only status, never response text, codes or
details. Transport normalization removes raw network errors. Auxiliary response
parsing errors and memory logs retain no model output or source text.

## Runtime, files and reviewed code

Current tools have no shell or general filesystem contract. Chromium helper
bytes are embedded in the Go binary and compared against the configured helper
inside the operator workspace before startup. Node executes those embedded
bytes, not mutable JavaScript selected by a model. Its environment is limited
to PATH, HOME, NODE_ENV and the browser installation path. Package-lock pins
Playwright; image builds use npm ci without lifecycle scripts.

Workspace file reads use os.Root, bounded regular files and clean relative paths.
Symlink escapes, absolute paths and traversal are rejected. This is a file-access
boundary, not an OS sandbox against bind mounts, devices or proc files. Runtime
images have a read-only filesystem, dropped capabilities, no new privileges,
bounded memory/PIDs and explicit temporary mounts. Tool executors do not receive
the Docker socket, PostgreSQL credentials or model credentials. Doctor instead
reads fixed health endpoints and a bounded, read-only core diagnostic response.
The separately approved user-tab relay and operator restart helper keep their
existing narrower contracts; their privileges are not granted to tool-http or
Chromium. A compromised runtime executable still requires OS/network isolation;
in-process checks are not a substitute for that isolation.

Go is pinned to 1.27.2. Earlier os.Root final-symlink/trailing-slash behavior is
affected by [GO-2026-4970](https://pkg.go.dev/vuln/GO-2026-4970); fixed releases
include 1.25.12 and 1.26.5. The workspace contract does not use a vulnerable
toolchain or claim stronger guarantees than
[os.Root documents](https://go.dev/src/os/root.go).

## Owner administration and remaining integrations

Prompt updates/previews and settings/provider mutations require authenticated
owner proof before parsing a body. A payload identity or an extension relay
token cannot provide that proof. The UI holds the owner token in memory only.
Read APIs retain the existing deployment perimeter; this change does not make
the dashboard an authenticated multi-user product.

There is no optional model risk filter acting as an authority source. Owner stop
and revocation continue to use durable revisions and ownership fencing.

Production auxiliary extraction, summary and embedding adapters are disabled
even when the legacy memory-pipeline flag is set: they have no durable job-owned
budget admission. A completed run lease cannot authorize a later cloud job.
Profile, working and covered-summary reads remain available. Re-enabling model
jobs requires a fresh claimed operation, owner/task-root and current mandate
checks, privacy policy and a reservation before every actual model/embed call.
The legacy adapters remain testable; that is not production job admission.

## Sensitive source and derivative retention policy

Unknown and historical content defaults private. Derivatives retain complete
source references, never reduce source sensitivity, and never gain instruction
authority. Secret-class material is excluded before extraction, summarization,
model input and retrieved memory bundles; the pipeline cannot create its vector.
Broker credentials remain encrypted secret-store records and ephemeral private
runtime inputs. Operators must not enter raw credentials into ordinary chat or
document memory. Pattern masks supplement this rule without proving that an
unknown string is safe.

Private transcript, summaries, profiles, episodes, chunks, checkpoints and
artifacts remain PostgreSQL records under the existing owner/deployment boundary.
The same private classification applies to their backups and exported copies.
Record suppression/expiry excludes retrieval but is not physical deletion. A
source removal request must include all derived records identified by provenance
and the separately retained backups; deleting one source cannot be described as
complete erasure. The versioned source catalog now performs transitive deletion
and privacy cleanup, fences pending/live model publication and checks a permanent
deletion anchor before restoring backups. A backup is not silently rewritten:
restored records are blocked/reconciled against the current deletion history.
Suppression, expiry and deletion retain distinct meanings. See the
[source contract](versioned-memory-sources.md) and
[actual deletion/restore evidence](../ai/versioned-memory-acceptance.md).

## Acceptance evidence and integration limits

The isolated lab uses Go 1.27.2, separate *_security_* test databases, Redis 13
and disposable read-only containers. Synthetic public DNS is injected through a
trusted test constructor and pinned to an HTTP fixture; private-network policy
remains enabled. No real external write or deployed service is modified.

Validation on 2026-10-11 passed full Go tests with real PostgreSQL/Redis and full
race checks, real Chromium session E2E, Compose configuration validation, and Web
typecheck/lint/build plus 10 Playwright cases. A parallel budget initialization
failure found during the run was corrected in the budget owner's separate
change; final full tests and focused budget/task race checks passed afterward.

| Criterion | Enforced boundary and proof | Limit |
| --- | --- | --- |
| Instruction provenance | Malicious document/tool/summary/child context stays data; PG pipeline reload retains source refs and private sensitivity | Model compliance is not execution authority |
| Workspace and privilege | os.Root escape/oversize tests; changed embedded helper is rejected; Chromium E2E runs read-only, no capabilities/no-new-privileges/no Docker socket | Approved user-tab relay and fixed restart helper retain documented operator exceptions |
| Network destinations | Mixed/changing DNS, literals, redirects, CONNECT, expiry and cumulative-byte fixtures; real Chromium blocks POST/private iframe/foreign fetch/WebSocket and permits same-origin GET | Remote operator webfetch must enforce its own DNS policy |
| Data release and volume | Exact GET approval works through real PG Broker journal and HTTP recipient; changed query has zero dispatch; revoked mandate stops before credential inspection; fixed runtime and serialized model caps plus037 ledger | Unbudgeted auxiliary model jobs remain disabled |
| Secret handling | Synthetic reflected credential/cookie/password masks; provider errors discard bodies; secret provenance cannot enter extraction/summary/bundles | No claim of discovering every arbitrary secret through pattern matching |
| Reviewed code updates | Runtime executes embedded reviewed helper, validates workspace file equality; package-lock/npm ci without lifecycle scripts; reviewed HTTP-JSON manifests use immutable review/profile pins and current task/mandate admission before credential release/claim | Extension packages are declarative DATA and never load code; declared dependency provenance is not a measurement of a deployed binary; local-model provenance/runtime gate is not tensor-integrity proof |
| Optional model filter | No detector has an authorization or bypass interface | Russian detection accuracy is not claimed or required by this boundary |
| Retention | Current source/derivative guards, transitive privacy cleanup, permanent deletion anchor and actual PG dump/delete/restore | Suppression is not erasure; private backups remain subject to operator storage policy |
| Independent owner stop | Current task/revisions/mandate and ownership fence checked before dispatch; existing real-PG stop-versus-claim/completion races remain authoritative | External effects already accepted retain truthful unknown/confirmed journal evidence |

The current integrated acceptance includes the actual installed extension
process, current model-loader provenance guards, selected child source grants
and versioned deletion/restore. The
[security acceptance audit](../ai/security-acceptance.md) pins their evidence.
Native delegation and model quality remain separate release gates; no optional
classifier can waive these programmatic boundaries.

## Packaged Web runtime

The operator UI also runs as UID10001 with a read-only filesystem, dropped
capabilities, no-new-privileges and finite RAM/PID limits. It receives public API
locations, not the owner token or encryption key as server-side credentials.
Its image includes only Nuxt `.output`; dependency installation and the reviewed
Nuxt preparation/build lifecycle occur in a separate build stage.

The reviewed selector-parser override pins published 7.1.6, removing vulnerable
6.x transitive copies. Upstream node-forge/braces have no published fix for the
current advisories and remain explicit lockfile findings. A lab output inventory
checks shipped package names and selected known module references without
printing source. Empty matches in a particular artifact are limited packaging
evidence, not a general exploit-reachability proof. Build/dev tools must not take
untrusted patterns/certificates, and audit findings are not silently suppressed.
