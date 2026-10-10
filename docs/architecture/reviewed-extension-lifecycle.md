# Reviewed extension lifecycle

Butler accepts local `butler.extension.v1` HTTP-JSON manifests as data. A manifest
cannot add code, installation hooks, processes, filesystem mounts, runtime
targets, secret-store access, Docker access or provider credentials. The operator
deploys a fixed Go executor profile first. No package source is downloaded.

## Authority and durable state

Migration 050 keeps exact manifest bytes, SHA-256, source/revision, compatibility,
dependencies, immutable reviews and lifecycle events in PostgreSQL. Review pins
the operator profile and its dependency descriptor. This records provenance;
it does not claim a declared hash measures a running image.

Installed, reviewed, enabled, healthy and available are distinct. Staging grants
none of the other states. Configuration may omit required settings and reports
their names without a request. Configuration/version changes disable the
capability, increment generation and invalidate readiness. Enable/disable also
increment generation. Rollback selects retained exact bytes and needs fresh
readiness/permission. Goals, tasks, effects and history remain intact. Database
rollback refuses nonempty history.

Readiness is a private authenticated owner action. Broker derives one fixed GET
from the reviewed manifest and approved configuration, reserves the existing
owner-day/admin-conversation budget and persists the attempt before egress.
There are no credentials, body or redirects. Only a verdict/digest for the same
generation/configuration is published. Health expires after 15 minutes.
Lost/interrupted responses remain unknown and are never replayed. Readiness
never proves task completion or an external effect.

Ordinary task attachment pins task/constraint revisions, manifest hash,
generation, exact tools, bounded origin/path scopes, credential aliases/fields,
permission revision and finite expiry. Goal-backed tasks require the exact
existing owner-mandate `ExtensionPermission` source/ID/hash/tool subset plus
current purpose/resource/effect/credential/owner-policy bounds. Package text,
model arguments and semantic similarity cannot grant attachment permission.

Capability admission runs before credential metadata inspection and again in
the same dispatch transaction after ownership/run/task/goal locks and before
effect/budget writes. Current install, review/profile, health, permission and
contract hashes are rechecked. Stale capabilities fail before networking.
The private scoped catalog accepts only the authoritative run ID. Filtering
precedes deterministic lexical search; future semantic ranking may only reorder
the allowed set under separately admitted inference.

## Runtime contract

The initial ABI is `butler.http-json.v1`. Fixed reviewed methods operate within
public configured origins and canonical path prefixes. GET/HEAD are reads;
POST/PUT/PATCH/DELETE are commits. Root args are exactly `url`, optional JSON
`body`, and optional declared `authorization` credential reference. No raw-secret
setting or arbitrary header exists. Output is bounded `{status_code, body}`;
the response body remains untrusted data.

Broker supplies a separate runtime service token and private action/deadline.
The runtime has no PG, model auth, secret store, package loader or general
file/process tool. DNS, IP literals and connections retain public destination
policy; redirects are denied. Exact request bytes and output are bounded.
Secret values and encoded variants are masked before observations leave it.
Provider declarations use core-owned descriptions and schemas without package
prose; package documentation is separate provenance-tagged DATA.

Optional reviewed commit `receipt` declares only existing `external_receipt_v1`,
a fixed idempotency header and same-origin bounded verification path with one
`{idempotency_key}` placeholder. Broker owns the durable key/fingerprint. Generic
HTTP success remains effect_unknown. A matching recipient key/fingerprint/artifact
receipt can confirm through the existing verifier. A prepare-patch name is still
a commit when it creates an external artifact; its name gives no retry safety.

Fresh model recovery reloads the current scoped catalog and replaces old package
descriptions/definitions. Recorded tool observations survive as DATA without
redispatch. Native signatures remain private live-transport state; restart does
not manufacture orphan function responses.

## Owner control and deployment

`GET/POST /api/v4/extensions` requires configured owner-control authorization.
Orchestrator forwards typed verified owner provenance through existing private
backend service-auth to `/internal/extensions`, without the channel token.
Operations: list, stage, review, versions, configure, rollback, probe, enable,
disable, grant_task, task_access. There is no generic URL/tool control action.
Mutations use exact installation generations/task permission revisions; JSON
rejects duplicate/unknown members. UI retains owner token only in page memory.

The optional Compose overlay deploys a separate executor with read-only root,
non-root user, no capabilities, no-new-privileges, bounded RAM/processes, only
tool networks and one runtime-auth file. Installing data starts no executor.
Production has no localhost bypass. Test binaries may use a trusted public
resolver/fixed fixture dial while retaining the same policy validation.
