# Native Antigravity access audit (2026-10-10 UTC)

No successful Gemini3.8Flash model turn was established on the lab. OAuth login
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
