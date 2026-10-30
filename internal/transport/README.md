# Logical model transport

The API-key [OpenAI provider](openai/README.md) uses the Responses API in both
WebSocket and SSE modes, with run-local explicit lineage and no replay after
dispatch. Its optional `OutputReasoningTokens` and `CachedInputTokens` metadata
are subsets of the existing output/input counts. They do not increase total usage
or change the separate Antigravity ThinkingTokens dimension.

Optional logical `Usage.ThinkingTokens` is a separately observed subset of total
usage, not an additional charge or allowance. Missing stays unavailable; explicit
counts must fit the observed total after input/output. The issue97 native
research runner retains it directly; existing protobuf boundaries are unchanged.

Optional `ToolSetSnapshot` on the in-process Go ContinueRun/SubmitToolResult interface replaces the selected definitions; nil preserves them and an explicit empty set clears them. Revision validation and stable historical aliases preserve function/result correlation. Orchestrator owns selection. See [deferred schemas](../../docs/architecture/tool-discovery.md); the protobuf service is not extended by this logical Go change.


Optional trusted `WithInferenceAuthority` context checks are evaluated by
Antigravity before authentication and HTTP. They are private current-source/
ownership callbacks, not model options or credentials; callers retain budget,
in-flight cancellation and final-acceptance responsibilities.

The transport proposes functions and emits normalized provider events; Butler
owns sessions, orchestration, policy, tools, durable transcript and completion.
Optional event `Usage` is one complete inference-request observation independent
of text/tool result type. Legacy final-only usage remains compatible. Counts are
observations, not permission, prices, budget refunds or cumulative run totals.

The direct Go Core budget wrapper validates this logical field and records it
once per admitted operation. No new RPC/protobuf mapping is introduced. See
`docs/architecture/model-transport-contract.md` and the native Antigravity README.
`WithInferenceDispatch` is an optional trusted one-attempt marker, distinct from
repeatable `WithInferenceAuthority` checks. Native Antigravity evaluates authority
before authentication and again before HTTP, then marks dispatch exactly once
immediately before its HTTP request. The generative memory worker uses this
boundary for its durable possibly-sent attempt journal; neither hook is serialized.
