# OpenAI API provider

The `openai` provider implements Butler's logical Go transport with the Responses
API. It is distinct from the ChatGPT OAuth `openai-codex` provider. Core owns
orchestration, sessions, memory, tools, admission and completion; there are no
OpenAI-hosted tools, sandboxes or agent loops.

Set `BUTLER_MODEL_PROVIDER=openai`, `BUTLER_OPENAI_MODEL=gpt-6-luna` and configure
`BUTLER_OPENAI_API_KEY` in the existing encrypted settings/environment secret path.
Never put the key into prompts, tool arguments, checked-in env files or logs.
Luna supports `BUTLER_OPENAI_REASONING_EFFORT=high`; empty retains the model default.
The application does not import Codex/ChatGPT authentication for API billing.

`BUTLER_OPENAI_TRANSPORT_MODE=ws-first` sends `response.create` to
`wss://api.openai.com/v1/responses`. `sse-only` uses POST `/v1/responses`.
`BUTLER_OPENAI_RESPONSES_WS_URL` optionally overrides the URL derived from
`BUTLER_OPENAI_BASE_URL`. The legacy `BUTLER_OPENAI_REALTIME_URL` setting name is
deprecated: its old official default maps to Responses; custom Realtime endpoints
are rejected. This is a protocol correction, not support for audio Realtime models.

Only a handshake rejected with 404/405/426/501 can fall back to SSE, before
generation dispatch. Auth, quota, timeout, ambiguous network errors and all failures
after a frame write never replay. Every inference marks trusted dispatch once and
rechecks current authority before release. Requests use the standard service tier
unless trusted caller options explicitly select another tier. HTTP redirects are
disabled; errors expose fixed diagnostics rather than provider bodies.

Sockets are scoped to runs, with no implicit cross-run conversation. Continuations
send the actual `previous_response_id`, current instructions, the selected tool
snapshot and string-encoded `function_call_output`. Responses retain the API's
default storage behavior. Butler PostgreSQL remains authoritative. A missing
locally measured prefix returns `stateful_session_lost`; recovery must rebuild
context through Core. A final retains run-local continuation state for one minute
for the kernel's bounded correction; cancellation/shutdown removes it sooner.

Optional tool fields remain optional (`strict:false`). Stable readable aliases
encode logical names such as `memory.search` without changing the Broker contract.
Argument fragments never execute. Complete function proposals are emitted once,
after the terminal response and its usage are decoded. Incomplete/failed responses
return failure with any valid observed usage, never executable partial tools.

The trusted ceilings `BUTLER_OPENAI_MAXIMUM_INPUT_TOKENS` (65536 by default) and
`BUTLER_OPENAI_MAXIMUM_OUTPUT_TOKENS` (8192) are declared to admission. Complete
context is bounded using observed previous input/output plus conservative UTF-8
request bytes. Output ceilings include reasoning; request options cannot raise
them or replace model/input/instructions/tools/lineage. Timeout uses
`BUTLER_OPENAI_TIMEOUT_SECONDS`. No model or account fallback is performed.

`Usage.OutputTokens` includes OpenAI reasoning. Optional `OutputReasoningTokens`
and `CachedInputTokens` are subsets, never added again to `TotalTokens`.
`CacheWriteInputTokens` and its `CacheWrite30mTokens`/`CacheWrite12hTokens` split
retain separately priced input subsets; reads and writes cannot overlap.
`ProviderServiceTier` records a recognized observed serving tier. Missing counts
or tier remain unavailable. Provider usage is not an invoice; account adjustments
still require billing evidence.

Offline tests use local fake Responses HTTP/WebSocket endpoints:
`go test -race ./internal/transport/openai ./internal/transport ./internal/config ./internal/providerfactory`.
Live tests require both an API key and `BUTLER_OPENAI_LIVE_TEST=1`.

Official contracts checked on 2026-10-30:
[Luna](https://developers.openai.com/api/docs/models/gpt-6-luna),
[Responses WebSocket](https://developers.openai.com/api/docs/guides/websocket-mode),
[function calling](https://developers.openai.com/api/docs/guides/function-calling).
