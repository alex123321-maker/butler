# Native Antigravity with Google AI Pro

Butler's `antigravity` provider uses Go HTTP/SSE directly against the production
Cloud Code subscription endpoint. The default requested model is
`gemini-3.8-flash-tiered` (Gemini 3.8 Flash). Every model turn stays inside Butler's
transport and execution loop. The official CLI is optional for initial interactive
Google account sign-in; it is never launched for inference or mounted into the
Orchestrator. No API billing, paid-credit overage, account rotation or staging
fallback exists.

This is an experimental, opt-in third-party integration. Google's
[Antigravity FAQ](https://www.antigravity.google/docs/faq/) says third-party
consumer integrations, including CLI wrappers, are unsupported. This adapter is
not an officially supported Google AI Pro integration. User authorization to test
the adapter does not establish Google backend entitlement or product support.

## Current verification status

Private lab probes confirmed OAuth refresh HTTP 200, subscription project discovery
HTTP 200 and the account's Google AI Pro paid tier. The model catalog advertises
`gemini-3.8-flash-tiered`. Native generation currently returns HTTP 429 despite
available catalog quota. A separate production native entitlement probe returns
403 `LOCATION_NOT_ELIGIBLE`; that is not proof of the canonical generation 429's
cause. Catalog availability and stored credentials do not prove
model access. Keep this provider opt in and leave the deployment's active provider
unchanged until a real text, function proposal and continuation smoke passes.

Lab tests use an HTTP fixture to verify native request/stream normalization,
function schema guards, thought-signature replay, cancellation, isolated runs,
redirect rejection and masked HTTP failures. PostgreSQL integration verifies
encrypted import, restart and revocation. These tests do not assert Google accepted
an inference request. The operator smoke uses a synthetic tool result and executes
no external tool; deployed Broker validation remains a separate check.

## Authentication boundary

`providerauth.Manager` owns import, refresh and subscription project discovery.
Credentials live in the existing encrypted PostgreSQL SecretStore, under the
hidden `BUTLER_PROVIDER_AUTH_ANTIGRAVITY` key. Provider OAuth uses a strict encrypted
store even when unrelated legacy settings allow plaintext. An encryption key is
required. The transport sees only `AntigravityTokenSource`, returning a short-lived
access token, subscription project and expiry. It does not read files, launch a
CLI, receive refresh tokens, or resolve arbitrary secrets.

The private operator import accepts exactly this flat JSON shape on stdin:

```json
{
  "access_token": "<private>",
  "refresh_token": "<private>",
  "client_id": "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com",
  "client_secret": "<private OAuth application credential>",
  "expires_at": "<RFC3339 timestamp>",
  "project_id": "<subscription project, or empty for discovery>"
}
```

Import rejects unknown/duplicate fields, wrong application IDs and missing token
material. Do not retain `id_token`, email or account identity. Do not paste the
import, Google auth code or OAuth application credential into Butler chat, Web UI,
source control, command arguments or logs. Build the input in a private operator
location with mode `0600`; it must be outside synced source and model workspaces.

Refresh runs 60 seconds before expiry using the fixed Google OAuth token endpoint.
Rotated credentials are encrypted before another network request can fail. Missing
projects resolve through `loadCodeAssist` using explicit Antigravity metadata and
the versioned HTTP User-Agent. HTTP redirects are refused, bodies are bounded, and
raw provider errors never cross the transport/settings boundary. There is no
implicit request retry. The settings UI reports stored credentials and allows
disconnect; that state does not claim that model quota is available.

## Private operator utility

Build `./cmd/provider-auth` on the lab. Give the utility a private environment with
`BUTLER_POSTGRES_URL` and `BUTLER_SETTINGS_ENCRYPTION_KEY`; use the same encrypted
database as the intended Orchestrator, or a dedicated test database. It supports:

```sh
provider-auth -action import < /private/butler-native-import.json
provider-auth -action resolve
provider-auth -action state
provider-auth -action smoke
provider-auth -action disconnect
```

Only `import` reads stdin. No credential is accepted as a flag. Output contains
safe status/errors, no model answer, account identifiers or credential material.
`resolve` refreshes and discovers the project but does not spend an inference turn.
`smoke` requests Gemini 3.8 Flash text, one native `web.fetch` proposal, and model
continuation after an explicitly synthetic observation.

On the existing lab, `deploy/antigravity/smoke-lab.sh /private/provider-control.env`
builds and runs this utility in disposable containers against the backend network.
The environment file must be private and contain the two variables above. The
script does not change running Compose services or the shared source pointer.
For one-off import, run the built utility with stdin redirected from the private
input; keep that input outside container/source snapshots.

After the real smoke succeeds, the operator can select `antigravity` through the
existing provider setting and use `deploy/docker-compose.antigravity.lab.yml`.
The overlay sets only provider/model; it mounts no CLI binary, profile or token
file. Preserve the user's requested Gemini 3.8 Flash model.

## Native function protocol

The adapter sends the protobuf JSON Cloud Code envelope to
`https://cloudcode-pa.googleapis.com/v1internal:streamGenerateContent?alt=sse`.
`enabledCreditTypes` is an explicit empty array. Only Butler-supplied schemas become
`functionDeclarations`; stable per-run aliases avoid tool-name collisions. There
are no provider native code, browser, shell, delegation or executable toolkits.

A complete stream may yield one schema-valid native `functionCall`. Unknown names,
invalid/duplicate JSON, schema violations, multiple calls, automatic execution
history, unsupported content parts, and truncated functions fail before a proposal
is emitted. Orchestrator/Tool Broker retain approval, policy, credential and action
authority. A matching `functionResponse` uses the original native name/ID and
exact returned model parts, including opaque thought signatures. Thought text and
signatures are never emitted as assistant output or persisted in Butler transcript.

Provider context is transient and run-local. Foreign refs, result replays and lost
contexts are rejected. A restart uses a fresh Butler-authoritative context; it
cannot reconstruct native signatures from caller metadata or a provider ref.
Initial capability supports text streaming, one function proposal per turn,
cancellation and usage. Batch functions and provider stateful restart are unsupported.

## Primary protocol references

- [Google Gemini CLI Cloud Code server](https://github.com/google-gemini/gemini-cli/blob/main/packages/core/src/code_assist/server.ts)
- [Google Gemini CLI request/response converter](https://github.com/google-gemini/gemini-cli/blob/main/packages/core/src/code_assist/converter.ts)
- [Google thought-signature requirements](https://ai.google.dev/gemini-api/docs/thought-signatures)
- [Optional official CLI sign-in](https://www.antigravity.google/docs/cli/install/)
- [Antigravity integration support policy](https://www.antigravity.google/docs/faq/)

The verified official Antigravity CLI 1.3.3 public protobuf descriptors confirm the
camelCase envelope and Vertex `parametersJsonSchema`, `functionCall`,
`functionResponse` and `thoughtSignature` fields. These are internal subscription
APIs and can change; unknown protocol behavior fails closed.
