# Logical model transport

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
