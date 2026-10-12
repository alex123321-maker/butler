# Logical model transport

The transport proposes functions and emits normalized provider events; Butler
owns sessions, orchestration, policy, tools, durable transcript and completion.
Optional event `Usage` is one complete inference-request observation independent
of text/tool result type. Legacy final-only usage remains compatible. Counts are
observations, not permission, prices, budget refunds or cumulative run totals.

The direct Go Core budget wrapper validates this logical field and records it
once per admitted operation. No new RPC/protobuf mapping is introduced. See
`docs/architecture/model-transport-contract.md` and the native Antigravity README.
