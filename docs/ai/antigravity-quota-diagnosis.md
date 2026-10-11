# Antigravity account and quota diagnosis — 2026-10-11

## Current conclusion after the offline CLI/Go comparison

The cause of the **last** generic `rate_limited` is still undetermined. The native
adapter maps upstream HTTP429 to that class; OAuth failure, local serialization
and SSE parsing do not generate it. Some earlier retained responses explicitly
contained RESOURCE_EXHAUSTED/HIGH_DEMAND, proving unavailable capacity for those
particular responses. Their explanation cannot be transferred to the fifth
window without its original response details.

No causal production-code defect has been demonstrated. Consequently the actual
endpoint, model, output8192 and credential flow are unchanged. This continuation
adds a working offline wire diagnostic and corrects the unsupported claim that
CLI session fields are "v1main-only". It makes **zero real model requests**.
All five reports and18 prior attempts are preserved:5 known and13 unknown usage
pairs;24921 input/547 output is only the known subtotal. Baseline13 remains SEALED.
No main checkout/deployment or other issue was changed; issue16 acceptance is
still separate.

## Source-path audit

Affected subsystems are model transport and private provider authentication.
The architecture, transport, credential, lifecycle and engineering rules were
read before work. Reviewed paths:

- [`providerfactory/factory.go`](../../internal/providerfactory/factory.go)
  composes the selected native provider with the existing auth manager and
  configured/default Flash model. There is no alternate account, API key or CLI
  inference fallback.
- [`providerauth/antigravity.go`](../../internal/providerauth/antigravity.go)
  imports the private authorization into encrypted PostgreSQL, validates the
  fixed OAuth application, serializes refresh and persists rotated credentials
  before use. Transport receives only a short-lived access token and expiry.
  Imported legacy project data is not an inference routing decision. The four
  earlier audit booleans below establish retained console credential origin;
  this continuation does not open the real credential record in model context.
- [`antigravity/provider.go`](../../internal/transport/antigravity/provider.go)
  takes the model from validated Butler-owned Start context. System/developer
  inputs become systemInstruction; user/model ordering and exact native tool
  correlation are retained. Only Butler tool schemas become declarations; no
  CLI tool execution, session ownership or orchestration authority is imported.
- [`antigravity/protocol.go`](../../internal/transport/antigravity/protocol.go)
  serializes a flat JSON body with `aicode.experience`, posts to fixed production
  AICode v1 `:streamGenerateContent?alt=sse`, supplies bearer auth privately,
  JSON Content-Type, SSE Accept and `antigravity/1.3.3 butler/0.1`. Redirects are
  denied. The exact whole serialized input is bounded before HTTP write.
  No CloudCode wrapper, alternate `model`, project, sessionId or entitlement is
  added. Output is bounded at8192.
- The same file bounds SSE total/frame size, requires a complete terminal
  candidate, rejects ambiguous/unsupported native behavior, and leaves absent
  usage unknown. Malformed/non-SSE responses become protocol errors. HTTP429
  with recognized bounded HIGH_DEMAND becomes `model_capacity_unavailable`;
  other429s remain `rate_limited`. Oversized/unrecognized error detail is not
  proof of quota or capacity. A response can originate at Google or an upstream
  proxy; the last retained class alone does not identify which.

## Implemented offline interception

[`scripts/dev/antigravity-wire`](../../scripts/dev/antigravity-wire/README.md)
executes the installed native CLI against an ephemeral synthetic HTTPS server.
The executable version is1.3.3; SHA256 is
`d97edcf90eda1799cd6cedc92616ef402f23c9752a723e8b8ae3e8f062ce458b`.
Static inspection of its public code/literals identified `USE_AICODE` and
`AICODE_ENDPOINT_URL`; the latter is not overridden in these captures. Actual
serialized HTTP, rather than a protobuf namespace, is the evidence below.

Builds and all execution occurred on laba under
`/srv/storage/butler/agents/antigravity-offline`, never on the main deployment.
Docker used `--network none`, read-only source/executable, no capabilities,
an isolated tmpfs HOME and a tool-free agent. The command also refuses any
non-loopback network interface. Its fixed-host CONNECT proxy can dial only the
synthetic loopback server. The CLI received newly constructed fake consumer
OAuth credentials, an empty work directory and an explicitly rebuilt environment;
no real profile, PostgreSQL credentials or operator `.env` was mounted.

Both clients received the user text `Return only OFFLINE_OK. Do not use tools.`
The fake model catalog contains Flash's previously observed public enumM322,
the requested wire model and quota1. License/config responses are incomplete
synthetic fixtures. The CLI was cancelled after its first serialized generation
and before a model response; its exit failure is expected. Exactly one CLI
generation was captured per mode. The actual Go Provider then completed a fake
SSE answer using fake auth. This is a serialization/parser check, **not** a
successful CLI generation, live server acceptance or equal full-context test.

| Wire property | CLI default fixture | CLI explicit `USE_AICODE=true` fixture | Actual Go Provider, fixture backend |
| --- | --- | --- | --- |
| Generation host/path | `daily-cloudcode-pa.googleapis.com/v1internal:streamGenerateContent` | `aicode.googleapis.com/v1:streamGenerateContent` | `aicode.googleapis.com/v1:streamGenerateContent` |
| Query/method | POST, `alt=sse` | POST, `alt=sse` | POST, `alt=sse` |
| Routing field/value | `model=gemini-3.8-flash-tiered` | `aicode.experience=chat-gemini-3.1-pro-low` | `aicode.experience=gemini-3.8-flash-tiered` |
| Body | project/request wrapper; requestId/requestType/userAgent | flat aicode/contents/systemInstruction/sessionId/generationConfig | flat aicode/contents/systemInstruction/generationConfig |
| Session ID | inside request | top-level | absent |
| generationConfig keys | thinkingConfig | responseMimeType, thinkingConfig | maxOutputTokens=8192 |
| Content-Type / Accept | JSON / absent | JSON / absent | JSON / text/event-stream |
| User-Agent | `antigravity/cli/1.3.3 (aidev_client; os_type=linux; arch=amd64; cl=996823801; auth_method=consumer)` | same | `antigravity/1.3.3 butler/0.1` |
| Same user prompt observed | yes | yes | yes |

The explicit AICode capture selected **another experience** under the fixture
licenses. It must not be described as a successful same-Flash comparison or as
proof that the real account silently falls back. The real profile's current
model alias, route and entitlement must be checked privately before a later
comparison. Likewise the default fixture's daily host does not establish the
real profile's current production route.

HTTP **v1**, including a sessionId in the CLI's AICode mode, is now directly
observed. This disproves the old namespace-based "v1main-only fields" statement.
It does not prove that adding these fields to Butler's current experience would
be accepted; earlier production probes rejected additions. Endpoint, headers,
thinking configuration, wrappers and model mapping differ, but no observation
isolates any one difference as the cause of the real429.

Safe generation projections, remote report hashes, source snapshots, test counts
and all five historical report hashes are retained in
[`offline-wire-20261011.json`](../../deploy/antigravity/offline-wire-20261011.json).
Capture source snapshot:20261011-215258-010. Expanded test source:
20261011-215653-732. Reports:

- `wire-20261011T145513Z-1482839-default.json`, SHA256
  `d9e4c56f6a28d0876e3368be72e8f1e3e2b553e40430de20fba935b065b2a10e`.
- `wire-20261011T145516Z-1483137-aicode.json`, SHA256
  `cab53c4a3df292f924e3f32c7fab632cb401edffe4a475cc91055a6344b5816a`.
- `wire-20261011T145657Z-1488477-tests.jsonl`, SHA256
  `1aa1cd28a0fef81f712c30918d52d591ef1af1d935daa2208de5fd29dc776d03`.

The reports reside under the isolated agent's `reports/`. Narrow transport and
diagnostic tests passed first; expanded `-race -count=1` tests passed110 distinct
test/subtest names across four tested packages. providerfactory has no tests.
Two real-PG integration tests were explicitly skipped (no PG configured); this
is not a new live-store integration proof. Vet passed for all five selected
packages. Diagnostic tests exercise external-interface denial, exact host
allowlisting and omission of private bodies/identity/auth from projections.
Both actual CLI captures and the Go fixture completion passed with zero real
model calls. No dependencies were added.

## Hypotheses and subscription accounting

| Hypothesis | Evidence and present status |
| --- | --- |
| Wrong account / imported credentials | Owner confirms same account; previous private encrypted-PG audit returned all four matches. No account substitution found. Audit equality is scoped to observation time. |
| Wrong HTTP version | Installed CLI actually serializes AICode v1 offline. No evidence justifies changing Butler to HTTP v1main. |
| Wrong generation contract or client/model routing | Real differences are measured offline. Live acceptance/causality remain unproven, and explicit AICode fixture model differs. Production change is not justified yet. |
| Exhausted paid quota | Pro membership and backend Flash remainingFraction1 are confirmed. No retained response proves subscription quota exhaustion. Coexisting free-tier is not inference accounting. |
| Available model capacity | HIGH_DEMAND proves this for particular earlier responses, not the last generic failure. Official Plans also describe capacity-dependent rate limits. |
| Wrong local classification | Current code distinguishes recognized HIGH_DEMAND and general HTTP429; local OAuth/SSE/JSON failures use other types. The fifth response lost specificity and cannot now be reconstructed. |

**Confirmed:** the original console authorization and the account's Google AI Pro
membership, including server Flash remainingFraction1. **Unconfirmed:** which
subscription bucket, if any, accounted for each direct AICode generation, and why
the meter stayed100%. The official [Plans](https://www.antigravity.google/docs/plans)
describe capacity-dependent limits and credit overage controls; the official
[/usage documentation](https://www.antigravity.google/docs/cli/commands/usage)
describes backend quota refresh. Neither provides a mapping from Butler's direct
AICode request to the desktop percentage. A full meter is compatible with
capacity failure and is not proof of free routing, no billing or unlimited
generation. No new model consumption was added to investigate it here.

The concrete [next experiment](antigravity-next-experiment.md) is prepared only:
at most two separately admitted generations, fixed prompt/account/Flash, bounded
payload/output/time, no retries or extra credits, and fixed safe failure metadata.
It requires separate explicit owner approval and a verified private dispatch
gate; no runnable live command or approval is implied by this document.

## Retained credential audit and server observations

The owner reports desktop version2.22 and Flash remaining100% for the same
account used in the lab CLI. Installed official CLI is1.3.3. Codex reasoning and
critics use Codex, while the Butler native experiments use Antigravity OAuth
from that initial console login. No second Google login was performed.

The earlier private backend audit compared the saved CLI refresh authorization
with the prepared operator import and returned match booleans only. The new
`provider-auth -action audit-origin` read the current live encrypted PostgreSQL
record and that same private import. Its actual result was:

```json
{"stored_credentials":true,"oauth_application_matches":true,"oauth_credential_matches":true,"refresh_credential_matches":true}
```

Together these checks establish the same retained console authorization in the
live Butler store at observation time. The audit did not refresh, write, request
OAuth or infer. It returned no token/hash/account identifier. A future mismatch
could be rotation; equality does not establish paid inference quota accounting.

Read-only metadata queries using the saved CLI access token returned:

- AICode production `GET /v1:fetchConfig`:200; requested
  `gemini-3.8-flash-tiered` exists, base `gemini-3.8-flash`, configuration version
  `3.8-flash-08-2026`, input1048576/output65536. Butler retains its8192 bound.
- Cloud Code `loadCodeAssist`:200; base tier `free-tier` named Antigravity and
  paid tier `g1-pro-tier` named Google AI Pro coexist. A base-tier field alone
  does not prove that a generation used a free quota.
- Cloud Code `fetchAvailableModels`:200; the requested Flash entry reports
  remainingFraction1. This agrees with the desktop100% and is a server response,
  not merely a stale UI display. Account/project details remained private.
- Correct GET `v1:fetchQuotaStatus`:500 UNKNOWN; production discovery:404.
  Neither is usable evidence about exhausted quota or request-schema support.
- Separate version comparison: `v1:fetchConfig`200 with the requested experience;
  `v1main:fetchConfig`404 and `v1main/remote:getUserQuotaSummary`404. The CLI
  protobuf namespace `google.gca.aicode.v1main` is not evidence that `v1main`
  is a supported production HTTP version. Earlier actual production requests
  rejected additional session/entitlement fields, so they were not blindly added.

Official CLI `models` is confirmed by offline `--help` as a metadata subcommand;
it was stopped after its bounded metadata deadline and is not a completed
same-prompt inference comparison. These diagnosis probes added zero actual model
requests. No automatic fallback, extra credits, API key or account rotation was
enabled. The sealed fifth native generation window remains immutable.

Prior actual Go and Node requests yielded429 RESOURCE_EXHAUSTED/HIGH_DEMAND,
identifying capacity for those particular failures. Old generic `rate_limited`
records, including the fifth window, cannot be reclassified without their
original evidence. The transport now preserves recognized HIGH_DEMAND as fixed
`model_capacity_unavailable`, discarding raw provider details. Google documents
capacity-dependent rate limits in [Plans](https://www.antigravity.google/docs/plans)
and backend refresh of quota in [CLI usage](https://www.antigravity.google/docs/cli/commands/usage).

Unresolved: whether the official client and Butler receive the same response to
an identical generation, and how these direct AICode generations enter the
desktop quota meter. A full meter alone proves neither a free route, a broken
generation contract nor freedom from capacity429s in the official client.

Actual audit validation used immutable source snapshot20261011-155541-905,
disposable encrypted PostgreSQL, race checks and vet on laba. The narrow report
`/srv/storage/butler/agents/provider-origin-audit/reports/provider-origin-20261011T085545Z-335155/tests.jsonl`
has SHA256 `0b68dcedbaa196037dea09f25112e990b5f1e15b6659b1d28a524f57a0afafbc`.
The expanded report
`/srv/storage/butler/agents/provider-origin-audit/reports/provider-origin-20261011T085605Z-336598/tests.jsonl`
has SHA256 `dffd92922bfbecc2169451c7609e55cc6f9583094081da0429b9a7103103514b`:
61 passed test/subtest names, three packages and no skips. It also built the
private static operator used for the live audit above. All checks added zero
actual model calls. The first failing duplicate-case fixture was preserved;
the importer now rejects case aliases rather than silently accepting them.
