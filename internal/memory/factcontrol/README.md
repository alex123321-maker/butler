# Temporal fact read control

`memory.facts` is an optional Broker-owned DATA read of accepted facts from at
most8 exact discovered sources. Validity (`valid_at`) and recorded knowledge
(`known_at`) are independent complete RFC3339 instants; neither grants access or
revives corrected, revoked or deleted original data. Unknown intervals stay
unknown. Output caps at10 accepted snapshots and the ordinary finite response
ceiling; omitted data is explicitly truncated. Snapshots contain sanitized
bounded values and temporal provenance, without hidden source bodies or authority.

The permanent Broker validates the canonical contract, no-secret arguments,
current task/input/worker proof, shared budget and private exact dispatch. The
owning Orchestrator repeats complete current Catalog proof in the same result
transaction. Adaptive composition consumes its existing call/time/byte allowance
and grants no opened citation for generated fact snapshots. Source quotation
requires the separate current `memory.open` receipt.

`BUTLER_MEMORY_TEMPORAL_ENABLED=false` and code-owned `ReleaseQualified=false`
keep production composition off. The Broker-only operator endpoint is
`BUTLER_MEMORY_FACT_CONTROL_URL` at `/internal/memory-facts`, authenticated by the
existing private action service token. Plain HTTP is limited to loopback or the
fixed Compose owner `orchestrator:8080`; redirects and environment proxies are
disabled for this private service route. No model-selected network request occurs.
