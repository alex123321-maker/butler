# Core/Broker utility trace observer

Read-only research adapter for `memory-utility-eval`. `Capture` takes a single
repeatable-read snapshot of the owning task/run/input, all calls, intents,
effects, dispatches, effect evidence, source pins/heads, model attempts and final.
It also checks current task authority and Catalog source fences. Closed-run
inspection never forges a live ownership token: current captured source grants
are checked separately against task/source/access revisions and expiry.
An explicitly blocked input with an unresolved effect can be observed before
run closure: its UNKNOWN receipt is preserved and completion stays false. It
cannot become a successful or verified-negative observation.

`Binding` is frozen before execution: suite/current-input projection, task,
constraints/owner/session, original recipient files and an exact one-to-one
mapping of synthetic source IDs to materialized Catalog references. Capture the
recipient's current files independently; never use a model's claim of unchanged
state. `Project` requires the original snapshot seal from the collector. It
rejects ambiguous ownership, omitted/extra evidence, foreign runs, changed args,
results that disagree with permanent Broker normalized-result digests, missing
dispatches, stale source pins, partial reads and unrelated citations.

Actual Catalog source IDs/hash/revisions in a strictly typed final are translated
through the frozen mapping into the utility oracle's source IDs. No answer value,
effect/check claim or free text is repaired. The raw final stays intact in the
sealed snapshot. Catalog candidate discovery is distinct from a complete pinned
open. The adapter supports memory.search/open and the reviewed project recipient's
inspect/prepare/check protocol plus exact `skills.load` corroboration. Other tools
fail closed, including `skills.search` until its evidence mapping is implemented.
Generic rejected mutations retain
UNKNOWN; no typed negative reason is invented from HTTP400.

`SkillBinding` pins the reviewed digest and Catalog body reference before execution.
The collector retains owning skill usages and current version/body heads. Core's
ref-only durable observation is checked against these and the Broker's digest of
the reconstructed exact original wire result; procedural bytes are never copied
back into product persistence. Current lifecycle checks include skill revocation
and mutable installed dependencies. C alone may load; missing, foreign, revoked
or changed evidence fails. Loading grants no answer/citation/effect credit by itself.
`TestReviewedSkillRealBrokerDurableApprovalE2E` exercises actual stage/test/activate,
Broker load, live context and durable approval recovery with scripted transport.

Prospective utility v2 uses `@current-verified-receipt` only in the verifier's
prepared-artifact oracle. Actual final/receipt IDs must match and a current
preparation/check chain must exist. V1 fixed-artifact behavior remains supported;
existing suites, snapshots and scores are not rewritten.

`TestMemoryUtilityActualCoreBrokerTraceIntegration` drives actual Core and compiled
Broker/extension/HTTP runtimes with scripted transport events in isolated PG.
It is not native inference or model-quality evidence. Outputs and synthetic model
attempt rows are retained in an exclusive private directory; they must never be
added to the historical Gemini spending ledger. Native use still needs a frozen
collector/denominator, original ledger correlation and independent review.

The seal establishes correspondence to trusted collected files, not a cryptographic
signature against an attacker who controls both files and their expected seal.
Do not accept observation/authority booleans or recomputed seals from model output.
