# Exact action authorization

Tool Broker authorizes every dispatch, including single calls, batches and
direct Broker RPCs. Missing policy, incompatible contract, expired approval,
failed authority request or missing grant denies execution before a runtime is
called. Models provide proposals and typed credential references; tool args
cannot supply owners, task versions, decisions, worker fences or grants.

## Preparation and dispatch

1. Orchestrator submits the complete action to `PrepareToolCall`.
2. Broker validates the current registry schema and builds a canonical binding.
   Duplicate JSON keys, multiple values and excessive nesting are rejected.
   Object ordering/whitespace are normalized; number precision is retained.
3. Broker reads run/session/task/lease state from orchestrator's authenticated
   private `/internal/action-context` endpoint. Session Service supplies the
   owner and current PostgreSQL lease/fencing epoch. Task Service supplies
   current versions; Run Service retains the pinned admission version.
4. Credential metadata is checked without resolving secrets. The binding pins
   a digest of alias policy, scope, status, reference locator and record version.
5. PostgreSQL stores the exact intent. Required owner approval uses the durable
   approval/continuation service. A bounded mandate is an alternative only where
   autonomy mode, contract and credential policy permit it.
6. `AuthorizeToolCall` verifies the current exact decision or mandate and creates
   a short-lived private grant. `ExecuteToolCall.authorization_id` is system
   metadata outside the model's tool schema.
   Its deadline also cannot outlive the verified exact approval. This bound is
   persisted with the grant and survives a wait between verification and claim;
   approval expiry cannot become a longer dispatch window.
7. Broker revalidates before each secret release, then again directly before
   dispatch. The grant is claimed atomically immediately before runtime entry.

The binding includes action/run/tool-call identity, canonical argument hash,
relevant full registry contract hash, concrete resource, credential alias/field
references and metadata digest, task/constraints versions, owner and effect
class. Any change invalidates earlier approval. The same run/tool-call identity
cannot be reused for different args. A resumed worker retains the approved
intent and acquires a fresh grant under its new lease/fence. An unused grant
may be refreshed; a dispatched grant cannot be refreshed.

Prepare/authorize/execute RPCs require a private service token. Knowing `run_id`
cannot impersonate a worker. Read-only catalog inspection remains available.
Both gRPC and private HTTP tokens come from a mounted secret file. HTTP
authority calls reject redirects, bound response size and time out after three
seconds. Failures deny actions. The baseline uses the trusted Compose backend
network; externally exposed deployments must use encrypted transport.

## Effects and autonomy

Effect classes are `read`, `prepare`, `commit`, `unknown`. Fixed read tools and
conventional HTTP GET/HEAD/OPTIONS are read effects; HTTP mutations are commits.
Credential storage installation is preparation. Browser interactions and
Chromium navigation, including independent `browser.read`, are unknown because
website JavaScript can submit, purchase or autosave. Model descriptions cannot
downgrade an effect.

`BUTLER_AUTONOMY_MODE` is explicit, defaulting to `mode_1`:

- Mode 0 permits verified reads without credentials.
- Mode 1 requires exact owner confirmation before every dispatch.
- Modes 2/3 may execute compatible reads; other effects require exact approval
  or an active bounded mandate covering the actual effect.

Contract-required approval, including `single_tab.bind`, remains mandatory;
mandates cannot bypass it. `always_confirm` credentials require exact approval.
`auto_read_only` forbids mutation even with approval; `manual_only` cannot be
used by the agent. An approval authorizes the exact alias/field/resource/action
combination and never changes the alias's global policy.

## Bounded mandates

Mandates bind an owner/task/task version/constraints version to exact tool
names, exact HTTP(S) origins or non-URL tool resources, explicit effect classes
and credential alias/field references. Wildcards are rejected. Expiry is at
most seven days. Scope changes require the next revision; revoked mandates
cannot be reopened. Grants recheck current revision/revocation; dispatch CAS
checks the mandate again. Child execution receives no independent permission.

The operator endpoint `/internal/action-mandates` on Broker accepts POST to
create/update and DELETE to revoke. It requires the private service token and
authoritative active owner/task versions from the supplied run. Request owner
fields are compared to current state, never trusted as identity. The payload
contains `run_id`, `owner_id` and typed `mandate`. Assign its ID with
`BUTLER_ACTION_MANDATE_ID`. This endpoint is not a model tool or unauthenticated
public API. Amendments, cancellation, owner changes, expiry and takeover
invalidate incompatible permissions before the next secret/effect boundary.

## Secrets and uncertain outcomes

Broad `owner_goal` mandates are a separate authenticated owner-service contract,
specified in [owner goals](owner-goals-mandates-spec.md). They do not use Broker's
legacy task-mandate writer. Trusted task assignments pin goal, mandate and
purpose-policy versions; canonical bindings retain those versions and exact
purpose. Scope/status/hours and deny checks occur before credential inspection;
actual dispatch rechecks current authority in its ownership/run/task transaction
before atomic budget reservation and grant spend. Models cannot choose an
arbitrary broad mandate through args. Original finite limits/deadline survive
edits and fresh runs. An expired, paused or revoked assignment denies exact
approval reuse as well as autonomous execution.

Secrets remain outside bindings/grants/notifications/model args. Only metadata
digests and references cross these boundaries. Credential Broker checks
metadata again before resolution. Browser runtime checks actual current URL,
fixed origin, observation version and owner before injection. Broker masks
known resolved values in runtime results/errors, including JSON strings,
common URL encodings and base64. Browser contexts never export raw
cookies/storage/passwords.

Locator/alias metadata changes invalidate approval. Rotating a value behind an
unchanged `env://` locator must also update the credential record's version
(`updated_at`); preparation does not read secret material to fingerprint it.

Grant states are `authorized`, `inflight`, `dispatch_completed`, `unknown`,
`revoked`. Claim happens after final checks, not at preparation/secret resolution.
`dispatch_completed` means a completed runtime response; it does not prove a
verified external effect. Timeout, runtime failure, missing result or outcome
write failure leaves a non-reusable grant and returns `effect_unknown`.
Orchestrator records this result then stops before model continuation with
`ErrActionOutcomeUnknown`. Owner verification is required before a new attempt.
Dispatch/result checkpoints preserve this boundary during recovery. Migration
035 adds the [external effect journal](external-effect-journal.md), permanent
trusted intent slots, declared recipient protocols and independent evidence.
Technical completion without supported evidence remains unknown. This contract
does not claim exactly-once effects for arbitrary remote services.

Revocation cannot undo effects already sent to an external service. Browser
sessions remain transient/run-local across process loss; approval cannot
resurrect a lost browser context.

## Operations and validation

Migration `031_action_authorization` creates intent/grant/mandate tables. Durable
approval bindings use the approval service/migration. Compose mounts a private
authority token into orchestrator and Broker. Generate
`.secrets/action-authority.token` (at least 32 random characters, private file
permissions) or configure another secret source. The lab helper generates it
outside snapshots under the lab's secrets directory. Missing auth wiring fails
closed, without a legacy bypass.

Lab tests cover hashes, changed resources/accounts/tools/versions, expiry,
revocation, outage, lease takeover, credential metadata changes, preparation
versus submit, reflected secrets and uncertain outcomes. Isolated PostgreSQL
tests use `butler_actionauth_test` with disposable schemas: 24 concurrent claims
produce one winner. Real e2e runs actual Broker/browser processes and real
PostgreSQL owner approval through the authenticated private reader. It injects
an approved password in Chromium, submits a multi-field form once, rejects
changed args/replay and verifies absence of secrets from approval state.
The boundary fixture uses inert checkpoint hooks; full ingress/restart recovery
belongs to the durable lifecycle suite.

`scripts/dev/lab-browser.sh` runs browser/runtime/Broker/approval e2e in existing
Playwright and disposable Go containers under an isolated agent root. It does
not change running Compose services or production database tables.
