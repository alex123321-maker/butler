# Native Antigravity access audit (2026-10-10/11 UTC)

## Subsequent authorized proxy and native protocol verification

An operator-configured lab proxy resolved the aicode regional denial. The
production native `aicode.googleapis.com/v1:generateContent` then returned HTTP
200 for Gemini 3.8 Flash, with the exact synthetic expected reply, finish `STOP`
and modelVersion `gemini-3.8-flash`. No CLI inference, external tools, credits,
alternate model or staging service was used. This proves a real native text turn;
streaming and function-call continuation still require the Go adapter smoke.

Successful request body uses only the experience and the standard content fields:

```json
{
  "aicode": {"experience": "gemini-3.8-flash-tiered"},
  "contents": [{"role": "user", "parts": [{"text": "<synthetic test prompt>"}]}],
  "generationConfig": {"maxOutputTokens": 512}
}
```

The official CLI 1.3.3 descriptor defines `model` and `aicode` in the same
`inference` oneof. Sending both yields invalid arguments. Native config HTTP 200
maps the experience to modelId/baseModelId `gemini-3.8-flash`, version
`3.8-flash-08-2026`, input limit 1,048,576 and output limit 65,536.
The actual `v1` HTTP schema rejects the newer descriptor's `sessionId` and
`entitlement` fields with `Unknown name`; omit both. OAuth supplies account
entitlement. This is a verified compatibility difference, not evidence to try
other API versions. No explicit paid-credit enablement is sent.

Headers are bearer authorization inside the closed backend resolver,
`Content-Type: application/json` and User-Agent
`antigravity/1.3.3 butler/0.1`. The successful response is flat: `candidates`,
`modelVersion`, `usageMetadata`; it has no Cloudcode `response` wrapper.
The old Cloudcode route still returned generic 429 through the working proxy,
while its catalog advertised remainingFraction 0.9999. That route's failure must
not be presented as exhausted native subscription quota.

`native-inference-smoke.mjs` reproduces one native text-only request through an
operator-configured HTTP/HTTPS CONNECT proxy. It reads the private import and
proxy environment solely inside the backend process. `BUTLER_PRIVATE_RESPONSE_PATH`
must select a new file in a separate UID10001-owned directory with mode 0700;
write preflight occurs before the request and the response file uses mode 0600.
It prints only fixed status enums, validation fields, schema booleans, public
model version, token counts and exact-match success. It never prints proxy URLs,
tokens, project IDs, raw answers or thoughts. Mount auth read-only and never include
private input/output directories in source snapshots. The sanitized successful
artifact is `native-inference-verification.json`.

The consumer product-support limitation documented below still applies. A
successful experimental request does not establish an official supported
third-party integration.

## Initial direct-route observations

Initially no successful Gemini3.8Flash model turn was established on the lab. OAuth login
and Google AI Pro entitlement are valid, but the new native production service
rejects this lab's region. The older endpoint's generic429 does not establish
that the user's subscription quota has been consumed.

## Observed account and endpoint results

The closed backend resolver read the explicitly authorized private native import
as UID10001. Credentials, project IDs and raw errors were never printed, serialized
into this report or changed. HTTP User-Agent was
`antigravity/1.3.3 butler/0.1`; no credits, staging, model fallback, account rotation
or alternate-region traffic was used.

| Production request | Result |
| --- | --- |
| cloudcode-pa `/v1internal:loadCodeAssist` | 200; project resolved; paidTier `g1-pro-tier`; no validation error/ineligible tier |
| cloudcode-pa `/v1internal:fetchAvailableModels` | 200; `gemini-3.8-flash-tiered`, `MODEL_PLACEHOLDER_M322`, Google Gemini provider |
| cloudcode-pa `/v1internal:retrieveUserQuota` | 200; target model remainingFraction1 |
| cloudcode-pa `/v1internal:retrieveUserQuotaSummary` | 200; Gemini weekly and5h buckets both remainingFraction1, enabled |
| cloudcode-pa `/v1internal:generateContent` and stream variant | Earlier canonical probes:429 `RESOURCE_EXHAUSTED`; no concrete ErrorInfo reason |
| aicode `/v1:fetchConfig` and `/v1:fetchQuotaStatus` | 403 `PERMISSION_DENIED`; ErrorInfo `LOCATION_NOT_ELIGIBLE`, detected country `RU`, error number1008 |

The aicode API version is `v1`, whereas its protobuf package is `v1main`.
Using `/v1main:...` yields404. This was resolved using authenticated read-only
configuration/quota probes; generation was not attempted through a denied route.
The regional denial is confirmed for aicode. Whether it also explains the older
cloudcode429 is an inference, not a proven shared error cause.

## Protocol evidence

The verified official CLI1.3.3 ELF contains both native protobuf contracts:

* Cloudcode GenerateContentRequest: project, requestId, request, model,
  userPromptId, userAgent, requestType, enabledCreditTypes; nested request is
  Vertex master GenerateContentRequest. CamelCase names are the descriptor's JSON
  names; the [official Gemini CLI converter](https://github.com/google-gemini/gemini-cli/blob/main/packages/core/src/code_assist/converter.ts)
  also uses protobuf snake_case aliases for selected fields.
* GCA aicode GenerateContentRequest is a different flat contract: model, contents,
  systemInstruction, tools, toolConfig, generationConfig, sessionId,
  entitlement.userTier. Its management model metadata has modelId/baseModelId/
  version and agent/tieredFlash experiences. These two envelopes must not be mixed.
* FunctionDeclaration supports parametersJsonSchema; Part supports functionCall,
  functionResponse and thoughtSignature. FunctionCall/Response preserve id/name
  and typed args/response. A descriptor match is not proof of real inference.

[Official Gemini CLI setup](https://github.com/google-gemini/gemini-cli/blob/main/packages/core/src/code_assist/setup.ts)
prefers paidTier when it coexists with currentTier; currentTier `free-tier`
therefore does not by itself mean an unrecognized Pro subscription.

## Supported access and operational consequence

[Google's FAQ](https://www.antigravity.google/docs/faq/) limits consumer access to
approved countries, lists no Russia, and advises checking the account-associated
country when it is incorrect. The same FAQ explicitly disallows third-party
software access under consumer terms, including wrapping `agy` in third-party
agents/apps, and directs such integrations to Enterprise or AI Studio API access.
This experimental internal endpoint investigation is not an officially supported
subscription API integration.

[Current plans](https://www.antigravity.google/docs/plans) retain capacity-dependent
limits and rolling/weekly quotas. The [official plan-change announcement](https://antigravity.google/blog/changes-to-antigravity-plans/)
states that Gemini Flash and Pro share a quota pool. Here both reported Gemini
buckets are full. Preserve a safe unavailable/access error and keep the provider
inactive until an eligible, permitted real3.8Flash text/tool roundtrip succeeds;
do not present generic429 as confirmed quota consumption or silently enable credits.

`native-access-audit.mjs` reproduces only read-only account/catalog/quota checks.
Run it on the lab in a read-only UID10001 container with the private profile mounted
at `/home/butler`; its optional `BUTLER_ANTIGRAVITY_NATIVE_IMPORT` selects an
operator-owned import path. It reads auth only inside the backend process and emits
allowlisted metadata. It cannot execute CLI tools, generate content, refresh tokens
or modify authentication/settings. `native-access-audit.jsonl` records the sanitized
replay; original private files remain outside source snapshots.
