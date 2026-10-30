# OpenAI Luna on the frozen V3 A/B/C corpus, 2026-10-30

The requested target is `gpt-6-luna`, reasoning High. The API-key OpenAI provider
is composed by the ordinary Butler provider factory, with real Core, PostgreSQL,
Tool Broker, memory reads and reviewed skill loading. It uses Responses WebSocket
mode; unsupported handshakes alone can fall back to HTTP SSE before dispatch.
There is no hosted OpenAI agent loop or change to production deployment.

The native producer is `157673530f7666ee51079663dee13ade60a0e310` in the fresh root
`/srv/gitea/butler-acceptance/issue115-utility-native-openai-20261030-r1`.
The final result is published in
[the evidence directory](../../experiments/memory115/2026-10-30/openai-luna-v3-v1/).
The earlier [Gemini V3 comparison](issue115-utility-native-v3-2026-10-30.md) and
all its original scores, receipts and unknown historical spending remain unchanged.

## Result

All 72 positions are observed: Luna **A=7/24, B=8/24, C=5/24**; the paired
Gemini result is **7/24, 20/24, 20/24**. Luna used 203 calls and 1093764 full
tokens, versus 281 and 2466920. Known Standard tariff cost is
USD **0.083172960–0.12937046**; cache-write lifetimes are unreported, so this is
a range, not an invoice. There is no unknown token usage or transport failure.

The producer exit is 1 because nine source-citation mappings fail projection.
All 203 raw/PostgreSQL/Core attempts correlate with no missing usage or token
mismatch. Retrieval returned empty in 37/38 searches, and only one source was
opened; skill loading genuinely occurred 21 times. Factual value diagnostics
remain lower than Gemini in B/C, and `check` reporting adds further failures.
All three arms actually prepared and checked the requested unapplied artifact,
but failed its fixed answer-value contract. The unchanged primary score and
separate physical/field diagnostics are published together in the evidence README.

After the paid window, `c5009153` adds a privacy guard denying unsupported
per-request storage overrides before dispatch. Its 92 offline race tests and vet
passed with network none and zero new model calls. It does not rewrite or replay
the native source157 window.

## What changed in the provider

- Both transports implement Responses. The old WebSocket implementation used the
  unrelated Realtime protocol; foreground cancellation now aborts the owning
  request rather than calling the background-response cancel endpoint.
- Tool outputs are API strings. Complete calls are released once after terminal
  completion/usage; failed/incomplete responses cannot execute partial calls.
  Native function names use stable readable aliases, restored to canonical Broker
  names. Optional schema properties remain optional.
- Current instructions and selected tools survive tool continuations. Explicit
  empty tool snapshots clear schemas. Run-local sockets do not borrow another
  run's history. Limits, source authority and one-use dispatch are checked before
  inference; no replay follows an ambiguous frame write or stream error.
- Output includes OpenAI reasoning already. Cache read/write counts and observed
  service tier are retained separately. Optional counter equality compares values,
  including unknown versus zero, rather than pointer identities. HTTP rejections
  retain bounded numeric status for research-global auth/quota stops.

See [provider configuration and contract](../../internal/transport/openai/README.md).
The owner's existing key is read from its selected private Windows env file and
passed through SSH to the isolated test process's stdin. It is not persisted on
laba, inherited by runtimes, put into model input, or recorded in the journal.
Read-only model-access probes returned 200 locally and on the established lab
proxy route before any generation.

## Comparison controls

The 24-case V3 suite is reused byte for byte, SHA256
`8fef1a32d521882cc751c595261dc9dc6ae1ff75e48ef239c77bb86ad4036919`.
All 72 A/B/C positions retain the previous Latin order, questions, synthetic
values, source states, instructions, scorer V3 and read-stop policy. A has no past
experience; B has prepared notes; C has the same notes plus the verified skill.
Fresh task/session/schema/runtime bindings prevent reuse of old execution state.
The reference is pinned in the new plan and historical receipts.

The profile is standard service tier, 65536 input/8192 output tokens, 90 seconds
per method, 13 methods per variant, 420-second authority and 480-second case
deadlines. The 4-hour window admits at most 936 methods. The operator's integer
monetary reservation rates round input/output up to one USD micro per token;
these are conservative admission ceilings, not measured API charges.

Notes and skills are scripted synthetic preparation, with genuine Broker
validation/activation. Preparation inference is zero; authoring labor, autonomous
learning costs and independent holdout performance remain unmeasured. Native
request usage, execution and preparation time, retrieval and skill calls are
reported separately. The owner oracle is not provider input.

## Validation and retained failures

The final controlled root is
`/srv/gitea/butler-acceptance/issue115-utility-trace-openai-20261030-r6`.
All 72 controlled positions and the real Broker durable-approval skill test passed
with no required skips. Source verification covered all 6784 Git blobs, accounting
for Git-declared CRLF export of Windows-only scripts. The exact binary and frozen
plan hashes are bound to the observed Linux cgroup before key input.

The same source separately passed OpenAI/config/factory race tests and vet,
22 actual PostgreSQL usage checks, and 325 tests across all transports. Eight
explicit live/external fixtures were skipped without their separate credentials
or opt-in; they are not claimed as native acceptance. Those independently passing
unit/PG reports are inherited from r5 with explicit provenance in the r6 gate.

All six controlled roots remain retained. r1 stopped before integration because
Docker's automatic address pools were exhausted; fresh roots use explicit private
subnets. r2/r3/r4 passed their then-current source. r5 passed all 72 controlled
positions but failed the separate skill test after a 750ms lease-observation
deadline and terminal-block persistence failure. Its cause is not proven; the
fresh sequential r6 passed with the same deadlines. No failed result was promoted,
no timeout was relaxed, and these gates made zero model calls.

## Accounting and interpretation

The read-only report checks every raw method against its PostgreSQL journal and
Core operation/run/reservation usage. Provider comparison requires exact suite
bytes and the same paired slot identities, without rescoring either provider.

OpenAI Standard tariff estimates use separately measured ordinary input, cache
reads, cache writes by lifetime, and full output. Missing cache lifetimes retain
a price interval; unknown usage is never priced as zero. Estimates are not
invoices or account-specific adjustments. Gemini subscription allocation is not
converted to an invented API token price. See
[API pricing](https://developers.openai.com/api/docs/pricing) and
[cache accounting](https://developers.openai.com/api/docs/guides/prompt-caching).

This compares two complete provider/model paths on one public development corpus.
High is not equal compute across providers; tokenizers, native schema/name
encoding, transport and cache behavior differ. Strict source/effect/coverage
failures remain failures even when a posthoc value appears correct. This does not
establish autonomous learning, qualify production memory, or close issue115.
