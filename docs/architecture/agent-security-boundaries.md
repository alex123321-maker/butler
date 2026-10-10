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

Resolved credential and cookie values, including common URL/base64 variants,
are masked from returned HTTP/browser observations. Cookies and arbitrary
response headers do not appear in HTTP results. Helper stderr and network errors
cannot disclose a URL, request body, credential or provider response body.
The credentials remain deferred Broker inputs; no runtime fetches SecretStore.
Pattern redaction supplements these boundaries and is not a permission source
or a guarantee of discovering every arbitrary secret in user text.

The operator-configured self-hosted webfetch service is a fixed destination
exception: redirects are denied, outgoing requests are bounded, and requested
resources are checked before delegation. The operator must apply equivalent
DNS/redirect isolation at that service. Its private network is not controlled by
Butler's dialer. Jina's optional operator service token is not an agent-selected
credential and is removed from reflected content.

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

The following remain separate requirements, and are not satisfied by prompt
admonitions: exact data-release authorization for arbitrary public destinations;
same-transaction aggregate byte quotas and provider serialization bounds;
reviewed origin/revision updates for future extensions (#18); and transitive
retention/deletion of derived memory (#22). Until those integrations are verified,
this document describes enforced boundaries, not full completion of issue #31.
There is no optional model risk filter acting as an authority source. Owner stop
and revocation continue to use durable revisions and ownership fencing.
