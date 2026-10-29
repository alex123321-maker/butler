# Native tool-call diagnosis and login continuation

Follow-up to #163 / PR #177. These are additional prospective trials, not
replacements for any of the original 15 matrix slots. Their wall times include
model deliberation, context/queue/operator/owner waits and tools from accepted
Input through its actual terminal state. Delivery acknowledgement is reported
separately and overlaps that interval. No old scenario was rerun to fill metrics.

## Diagnosis and implementation

The old login-01/consent-03 invalid-tool failures retained only the coarse code;
their rejected native payloads are unavailable and their precise causes remain
unknown. Fixed future diagnostics distinguish malformed envelopes, unselected
functions and argument type/required/unknown/union/enum/cardinality failures.
Names, paths, values, signatures and provider bytes remain excluded. Core's
run-error event/log preserves the allowlisted reason. The original invalid
proposal remains terminal, with no tool dispatch or automatic retry.

Broker preparation/authorization now distinguish fixed gRPC status classes
without copying RPC messages. A private fixed-string comparison classified
consent-02's old failure as the prior generic `broker_transport` wrapper; the
underlying status/cause was not retained. This is not proof that its network
failed or that any credential request/field was dispatched.

New diagnostic-only `login-fix-01` failed after four catalog searches with
`native_arguments_array_bounds`, before any browser/vault action. Only the
selected catalog loader had an array cardinality bound. Its native declaration
had removed `minItems/maxItems`, leaving the model unable to see the required
1..8 limit while local validation still enforced it. The exact rejected array
and whether it violated the lower or upper limit were not retained. The fixed
reason was observed in the exact-run Docker log before the next rollout;
the checked-in reason artifact explicitly labels its operator transcription.

The adapter now appends the exact six omitted bound annotations as description
DATA at their original schema node. Existing descriptions, keyword-like property
names, nested arrays/unions and current alias selection survive. Original
logical definitions, Broker hashes, validation, selected permissions and response
schema dialects are unchanged. Invalid batches are never split or retried by the
adapter. A new `login-fix-02` prospectively tests that change.

## Verification and lab composition

Windows: transport package suite, targeted Core failure sanitization, Broker
client diagnostics and the updated Antigravity suite passed. An initial broad
transport run exposed an old catalog test using the response-schema validator
for tool declarations. The test now uses the actual native tool converter;
the response-schema dialect itself was not widened.

Actual minimal Core variant on laba: diagnostic changes passed 311 test/subtest
events with race; bounds presentation passed 227. Both windows had zero failures
and no required skips. Five and two explicitly listed optional external/live
fixtures respectively were skipped; these are not native successes. Vet and
Core builds passed. Their summaries are retained separately, not added as a
count of unique tests.

Raw proof roots:

- `/srv/storage/butler/agents/browser-login-fixes-20261029/reports/build-v1`
- `/srv/storage/butler/agents/browser-login-fixes-20261029/reports/build-v2`

The initial builder stopped on optional external-fixture skips despite passing
tests. A separate qualification step checked the exact skip-name allowlist and
mandatory passing names before vet/build; it did not rerun inference or label
the optional skips as passes.

The lab received only these changes on its existing reconstructed Core:
`browser-login-diagnostics-20261029-v9`, then
`browser-login-bounds-20261029-v10`. The latter Core binary is
`7fc4d385909430cbde5928b30446df9bdd14a07acdc95f8ed864699d16755296`.
Health, binary identity and unchanged private environment were checked.
Broker/browser/PG/Redis/Web containers were unchanged. Existing release files
remain for rollback. No migration, grant, PIN, budget or provider/model reset
was introduced; this is not deployment of the entire PR HEAD.

Each new login trial has a fresh synthetic receiver configuration/journal and
account record. The ordinary owner task names its account title to identify
which saved account to use. Official Vault ingestion creates the fixture only;
it creates no use grant or PIN unlock. The model still searches, requests
protected owner consent and supplies deferred credential references through
the ordinary Broker/runtime path. Secret material and expected private answers
never enter the operator report or task prompt.

## Native observations

`login-fix-01` remains a failed diagnostic trial; recipient journal is empty.
Its full Input wall time was **96.699s**; no login or use grant was created.

`login-fix-02` completed the additional protected login/private-data/delivery
scenario under v10. Its single successful `tools.load` selected exactly eight
refs. Real owner consent approved the expected fresh record. Two deferred field
preparations returned `credential_verified=true`; the operator never received
their values. An exact submit approval was granted separately. The receiver
recorded exactly one accepted login at 04:55:38.712697991Z and one authenticated
account read at 04:55:38.784782973Z.

The opaque click stayed UNKNOWN and the kernel froze the browser. After the
test operator independently checked the fsynced synchronous recipient journal,
a protected factual outcome report continued the same Input/run. The original
UNKNOWN click stayed in its transcript; no input/action was replayed. Assistance
was three decisions: real owner consent, exact submit approval and factual
operator report. The approval helper first refused a composite selector outside
its whitelist before sending any request; review of the actual fixed form allowed
that one exact submit selector in a separate helper. Product policy was unchanged.

The agent closed the browser, corrected one locally rejected communication draft
without dispatch, then published the requested private reference:
`publication:bc3e6e1c8d0da8753892af8bd9c0fd6e5ce435a33febf7d571837c0aa325a58e`.
Protected owner GET/private comparison confirmed the **exact private answer** and
absence of username/password canaries. Web has `result_ready` and Telegram has a
persisted `sent` acknowledgement. This proves channel availability/API delivery,
not that the owner read the message. The private reference itself is not exported.

Full Input wall time: **358.553s**, including model deliberation/context/queue/
owner/operator waits. Its complete partition is 305.378s of inter-tool gaps,
19.565s of tool spans and 33.610s after the final tool. Telegram sent occurred
328.823s after acceptance and overlaps that interval. There were 13 recorded
tool/local-validation spans and 14 model attempts. Inter-tool gaps do not isolate
model-only CPU time or establish a latency SLO. The old failed slots and their
measurements are unchanged.
Recorded usage is 154,298 input and 1,639 output tokens across those attempts;
there are no unknown usage pairs. Monetary cost and isolated thinking time are
not established by these observations.

A separate read-only private audit checked all 13 current-run tool argument/
result records and three message rows for exact synthetic username/password
canaries; none were found. The published answer also passed its independent
canary check. Provider wire payloads and system logs were not audited by this
bounded check, so it is not a universal privacy certification.

Evidence: [native metadata](browser-baseline/login-fixes/login-fix-02-metadata-final.json),
[load/preparation/record check](browser-baseline/login-fixes/login-fix-02-load-preparation-check.json),
[receiver](browser-baseline/login-fixes/login-fix-02-recipient-final.json),
[private answer verification](browser-baseline/login-fixes/login-fix-02-verification.json),
[transcript canary check](browser-baseline/login-fixes/login-fix-02-transcript-canary.json)
and [inclusive action timings](browser-baseline/login-fixes/timings-inclusive-v1/README.md).
The two new trials and 17 spans stay separate from the original matrix.

This qualifies one complete **assisted** login/private-data/delivery scenario and
the prospective catalog-bound fix. It does not retroactively identify the old
invalid requests, prove that the old consent-02 Broker failure is fixed, or prove
autonomous outcome resolution. Original #163 restart/cancel/revocation and other
unfulfilled criteria remain open; general browser acceptance remains NO_GO.
