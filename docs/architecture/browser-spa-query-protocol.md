# Bounded SPA query profile

Source implementation for #165, under qualification. This is an explicit
supported read profile, not a generic POST authorization or universal SPA claim.
The old browser read/prepare/click/login contracts remain distinct.

## Meaning and authority

`browser.read_spa` revision1 reads one same-origin, credential-free HTTPS SPA in
a fresh ephemeral browser. Optional public text/search/select input changes a
filter. Go permits conventional GraphQL **query** POST documents under a fixed
bounded grammar, GET/HEAD, passive WSS, typed `graphql-transport-ws` query/read
subscriptions, and SSE/fetch streams. The typed socket state machine requires
init/ack before a query/subscription, binds responses to at most four unique
operation IDs, and accepts single-root subscriptions only. Init has no auth
payload; ping/pong can only echo the preceding server ping. Arbitrary
REST POST, GraphQL mutation/batch/extensions/fragments/directives, untyped
outgoing WS application data, credentials and cross-origin destinations fail
closed. A GraphQL query is treated as conventional protocol reading, analogous
to conventional GET; this assumes the recipient honours that protocol. The
model cannot classify an arbitrary body/message as a read. Visible data is
not evidence of a separately requested business mutation or task completion.

The exact code-owned contract is installed only by default-disabled private
startup composition. Current owner-goal assignment and read resource scopes
are mandatory; delegation and credential references are excluded. The accepted
Input's private claim hash must remain current at claim, network checks and
result release; replacing/expiring that claim cannot inherit this operation.
Mode1/exact
approval, standing release, current purpose, source, Task/constraint/mandate
versions, explicit denies and budgets retain their ordinary owning checks.
No separate approval is invented when an existing current mandate covers this
fixed read capability. A changed filter is a different immutable action.

Broker claims one action, budget and creation lane in the existing transaction.
The resource wrapper consumes its private ticket once. During execution the
authenticated `check` operation rechecks **running**, not prepared/completed,
dispatch identity, hashes, original lease/deadline/fence, source, journal,
budgets and current assignment. The actual URL receives a scope-only projection
under that same assignment, including goal-wide deny overrides. This never
rewrites the action or extends/refunds/replays a permission. No network I/O is
performed while the authorization transaction is open.

## Page, worker and transport boundary

Playwright routing alone cannot fence every worker update. A new private Go
HTTPS relay terminates every context CONNECT locally and checks each actual
decrypted origin request. It never forwards an opaque CONNECT to a remote
server. Each remote connection repeats public DNS checks and pins the checked
IP; Go independently verifies remote TLS/hostname with normal trust roots.
The fresh Chromium process trusts only the relay's ephemeral SPKI. No blanket
certificate bypass, host trust-store change, persistent CA/private-key file,
remote debug port or additional container is introduced. The private key stays
in Go; only its public pin and private loopback proxy address reach helper IPC.

Worker registration, main-script update and worker fetch use this same path.
Worker cache responses remain local, but no cache/update/restart can release an
unapproved remote method/address. QUIC/non-proxied UDP are disabled. A private
resource callback verifies current authority before remote socket release and
before delivering response chunks; an idle watcher closes channels on denial.

Fixed limits: at most20s execution within the original resource maximum30s;
128 origin requests and separately128 CONNECT attempts,16 concurrent HTTP
operations,4 GraphQL POST queries,4 WebSockets,128 received and128 sent WS
messages,64KiB query body/frame. A combined4MiB remote traffic cap fits the
existing shared egress reservation, including TLS/handshake bytes in both
directions; redundant directional ceilings are4MiB outbound/20MiB inbound.
Counters do not reset. The relay requests identity content encoding and rejects
compressed responses rather than permitting unmetered browser decompression.
Operators cannot enlarge these limits through tool args.
Oversize, unsupported request, lost response, stream error or current-authority
denial terminates the relay. HTTP/WS clients perform no hidden request retry.

## Terminal freeze and result

One private helper phase loads the SPA and optionally changes the public
filter. Go then **terminally closes** the relay before a separate nonce-bound
visible observation. Pending query responses or lost runtime/authority/finish
acknowledgement fail; no success is inferred from a popup or DOM callback.
The helper destroys its separate browser, worker and storage before return.
There is no thaw, exposed session/page handle, restoration or queued-message
replay API for this profile. A later read requires a new ordinary action/budget.

Go supplies the fixed profile and bounded content-free network counts. Broker
rechecks actual result URL and current assignment before releasing data. Hidden
fields, cookies/storage and reflected secret values retain ordinary browser
redaction. Nonces, certificates, response headers, raw frames, cookies and
request bodies are not model output. A successful read does not complete the
Task or imply publication/delivery. Failures retain spent grants/UNKNOWN and
resource quarantine under existing rules.

The finite operation has no surviving owner-viewer context; owner Task stop/
revocation closes it rather than attaching to or restoring a page. Existing
interactive managed pages retain their independent owner-control epochs and
network policy. This profile cannot use a viewer controller to authorize input.

## Composition and qualification

Combine `docker-compose.server-browser.yml` and the opt-in
`docker-compose.browser-spa.yml`. Both services require the dedicated runtime
token; runtime additionally fixes the private Broker authority URL. Broker
checks authenticated runtime capabilities before advertising the tool. Missing
guards, wrong endpoints or disabled runtime fail closed. JSON registry/settings
cannot replace the code-owned contract. No migration/new dependency is needed.

Required: strict query/argument/proof unit tests; real PostgreSQL Broker/resource
tests for original claim and ongoing current authority, source/expiry/revocation,
path denies, restart/UNKNOWN and unchanged spending; actual HTTPS Chromium
worker/cache/update/restart, filter, WS/SSE/stream, freeze/loss and hostile network
tests; owner UI stop; then native model→Broker→runtime→result→actual delivery.
Mechanical fixture PASS is not native acceptance. Native wall time includes all
model deliberation, discovery, scheduling, owner waits and delivery. Historical
timings are not recreated by replaying old Inputs.

Until the full evidence matrix is recorded, #165 remains open.
