# Tool Browser

Revision1 `browser.login` privately fills two Broker-resolved fields and submits
one exact ordinary URL-encoded same-origin form. The helper gates the actual body,
blocks extra requests and freezes after bounded GET redirects. Go compares a
cookie-backed GET with an anonymous challenge before returning authentication
proof. Nonce/cookies never reach tool output. OTP/JS-only/popup flows, rotating
probe cookies and ambiguous results retain UNKNOWN; there is no automatic replay.

Local preparation revision4 can reject a missing or closed context before any
runner call with a closed `session_lost` pre-input proof. No helper/in-flight
error can emit this Go-only proof. The model must open a new context; this never
restores DOM or decides the outcome of an earlier action. Revisions2/3 retain
their old semantics and never gain the new reason.

Local `prepare_fill/prepare_type` revision3 adds a closed `pre_input_v1`
rejection only when Go/helper guards prove input dispatch never began. Stale
page/URL and validated pre-input argument failures return fixed diagnostics;
timeouts, generic failures and errors after input remain uncertain. Revision2
keeps its old outcome semantics. Success still proves frozen local preparation
only. Broker may record this exact new preparation as absent, retain its consumed
grant and let the model observe/reassess through a fresh authorized proposal.

`browser.freeze_snapshot` revision1 closes all proxy connections and freezes
helper routes before a non-navigating observation of the same document. It
retains redaction and permanently excludes GET form/link thawing of that
context. It accepts only its existing session ID and no credentials. This
observation never proves a previous submission/login or effect reconciliation.

Secret fill/type additionally return `credential_verified` after comparing the
actual field with the private Broker-resolved value. Mismatch fails without
exporting input values, lengths or hashes. Preparation retains its frozen-network
contract; this check is not proof of remote login. The workspace path canonicalizer
also admits normalized slash paths on Windows for the same checked helper bytes.

Browser runtime service for Butler browser automation tools.

Current state:
- `browser.submit_get_form` supports public same-origin GET search forms without click/input/submit handlers; two private phases freeze planning, independently check the public target, recheck live state and permit GET/HEAD navigation only, then freeze again
- `browser.follow_link` uses the same GET path for public pagination/result anchors without click handlers; neither read tool can thaw a context containing any prior input draft or owner takeover, even without credential references
- both GET tools require revision2 / `guarded_browser_get_v1`; fixed precondition failures before navigation emit a bounded non-dispatch proof, while timeout/lost response/started navigation never claim absence. Broker verifies the proof and preserves spent grants.
- observations include bounded form selectors/methods/actions/field names/types, with no input values; GET-form navigation refuses credentials, password/hidden/file controls, sensitive/extra field names and POST forms
- runtime exposes the internal `ToolRuntimeService` gRPC contract
- `browser.read` independently reads and destroys a temporary context; `browser.open` creates an explicit run-owned session for sequences, and `browser.close` destroys it
- navigate/click/fill/type/wait/auth actions require the returned session ID, latest observation version and expected current URL; snapshot/extract never navigate
- `browser.prepare_fill/prepare_type` freeze all routes and private HTTP/CONNECT connections before input; snapshot/extract/wait keep the freeze, including delayed autosave
- a separately authorized click opens a bounded window only after live guards, then freezes again; navigation/live fill/type cannot thaw a prepared draft
- revision 3 read/open/navigate enforce GET/HEAD; local preparation proof confirms only local state, while a DOM submit without recipient evidence remains an unknown external effect and blocks later effects
- each context permits public HTTP(S) origins, with a 15-minute TTL and exclusive operation lock; 32 live contexts maximum, with no cross-run transfer or implicit crash recovery
- one long-lived Playwright helper keeps contexts between calls through private stdin/stdout messages; resolved secrets are never process arguments or diagnostics
- URL and selector validation are enforced before execution, and extracted text is truncated to safe response sizes
- password inputs require broker-resolved refs; cookie/storage inputs accept refs only, retain separate approval and cannot enlarge the session origin
- auth persists within its live context; cookies/storage/password values are never exported and reflected secrets are masked in observations
- credential actions use the verified exact-version Broker approval handoff; credential resolution rechecks the actual current URL and current origin before injection
- Playwright helper resolution is deployment-safe through `BUTLER_TOOL_BROWSER_SCRIPT_PATH` instead of `runtime.Caller` path discovery
- Docker Compose deployment must attach the runtime to an egress-capable network in addition to the internal tool network so outbound site navigation can resolve public DNS
- HTTP health endpoint and unit tests are in place
- real Chromium e2e checks form continuation, auth persistence/masking, stale observations, expiry and process loss via `scripts/dev/lab-browser.sh`; use only the isolated lab workflow
- public redirects and CDN resources work within contract/operator caps; native SOP, public DNS/IP checks and credential scope stay enforced; popup-only SSO remains unsupported

Local run:
- `go run ./apps/tool-browser`
- requires `npm ci --ignore-scripts` with the committed package-lock in `apps/tool-browser/`
- service exposes HTTP health on `BUTLER_HTTP_ADDR` and runtime gRPC on `BUTLER_GRPC_ADDR`
- optional overrides: `BUTLER_TOOL_BROWSER_NODE_BINARY`, `BUTLER_TOOL_BROWSER_SCRIPT_PATH`

Intended responsibilities:
- execute browser-class tools only
- stay behind Tool Broker routing and policy checks
- never receive raw secrets directly from model-visible context

Related docs:
- `docs/architecture/tool-runtime-adr.md`
- `docs/architecture/browser-session-contract.md`
- `docs/architecture/credential-management.md`
- `docs/planning/butler-implementation-roadmap.md`

The reviewed helper is embedded in Go and checked against its workspace file; arbitrary helper updates fail before Node startup. Private expiring proxies pin public DNS and retain operator domain caps, share a 4 MiB outbound counter, deny WebSockets/service workers and allow writes only during authorized interactions. The image is read-only with temporary mounts and receives no model/PG credentials. Real Chromium tests include malicious POST/private iframe/foreign fetch/WebSocket attempts with zero effects, alongside permitted forms and auth. See [security boundaries](../../docs/architecture/agent-security-boundaries.md).

The optional `resourceexec.NewRuntime` wrapper adds private one-use Broker tickets,
online current authority/source checks and a bounded local resource lane before
this existing backend. It receives no PostgreSQL access or owner policy. Known
local completion returns a hashed receipt; a missing receipt keeps the durable
inflight resource quarantined and returns an unknown effect. Current-origin credential checks and page CAS and
credentials remain enforced by this backend. The constructors are not enabled by
production bootstrap/Compose in this change. The actual isolated Chromium proof
demonstrates controlled open plus stale-page click refusal with zero recipient
POSTs. It does not establish user's legacy `single_tab` arbitration. See
[resource adapter and required composition](../../internal/resourceexec/README.md).

Owner viewing/control is available through the optional private owner channel in
`deploy/docker-compose.server-browser.yml` and Web `/browser`. Set
`BUTLER_BROWSER_CONTROL_TOKEN_FILE` to its dedicated service token. Broker owns
owner/run mapping and durable control epochs (migration086); the runtime owns
one exclusive context mutex and private rendered frames. Pixels/manual input
never become model results. See [the owner protocol](../../docs/architecture/browser-session-contract.md#owner-viewing-and-manual-control).
`single_tab.*` is disabled in the standard tool registry.
When the owner channel is enabled, session tools also require Broker's private
dispatch epoch; delayed calls cannot cross a takeover/release cycle. Manual
frame guards include cross-origin child documents, not just the main page.
Masking merges secret spans in the original text, including successive typed
password prefixes; shorter values cannot expose a longer value's suffix.

Navigation revision4 can reject a Go-observed lost/closed context or a previously
observed frozen network before invoking the runner. Only that pre-dispatch guard
emits the new `session_lost`/`network_frozen` absence proof. In-flight/helper errors
and old navigation contracts retain uncertainty; recovery requires a fresh open.

## Public form constraints and local validity observations

Ordinary form fields may include bounded `constraints` metadata: required,
numeric min/max/step and text length bounds. When the network is frozen, optional
`validation` contains only `valid` and a closed list of native validity flags.
The observer reads ValidityState properties; it never calls checkValidity,
reportValidity or submit and never returns values, patterns or validation text.
These are page DATA for deciding the next proposal, not external effect evidence.
A successful local correction does not confirm a later POST.

Password, hidden, file/unknown controls suppress this metadata for the entire
form. Known cookie/storage/credential state suppresses it for the context; a
Broker-resolved credential makes Go's suppression sticky for that context.
Secret state is collected before/after capture to suppress late discoveries.
Go independently bounds forms/fields/constraints and rejects unknown or
contradictory validity flags. Normal action, approval and uncertainty policy stays.
