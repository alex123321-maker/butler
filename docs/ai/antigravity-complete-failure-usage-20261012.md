# Complete rejected Antigravity request measurements

This is a forward-only transport accounting regression fix, not another model
observation window or a rescore of historical evidence. Native calls, imports,
real credentials, loads, messages and historical backfills: zero.

The parser already consumed and validated all SSE candidates and trailers before
rejecting a `MAX_TOKENS` function proposal. It then discarded the complete
request's measured usage/version. The invoke-error branch also emitted an error
without the returned measurements. In contrast, function-schema rejection after
successful parsing already preserved complete measurements.

The post-clean-EOF truncation rejection now returns a metadata-only internal
answer. The provider attaches its typed usage and optional observed version to
the single safe terminal transport error. Rejected parts, text and signatures
never accompany the error. The failed run is removed, no tool/final is released,
and submitting a forged continuation cannot issue another request.

The change does not stop parsing at a candidate terminal or `[DONE]`. Missing,
partial and provisional usage stays unavailable; a reported zero triple remains
known zero. Conflicting, aliased or invalid metadata, incomplete frames and body
reader failure still discard measurements. No partial answer is retained across
a read failure. No retry, fallback, model profile or budget policy changes.

The actual V2 timeout's packet stage remains unknown. A safe timeout code alone
cannot distinguish authentication resolution, HTTP acquisition, body reading or
waiting for EOF/trailers. The sampler can also reach its deadline before observing
a complete event. This fix covers a clean-EOF truncation failure, and supplies no
evidence that the historical timeout took that branch. Earlier unavailable usage
and sealed reports remain unchanged.

## Offline proof

Base `2c15551f9297c02d341a8287bb18ee22e2412f6a`; isolated branch
`codex/antigravity-complete-error-usage`. Builds/tests ran only on laba in
`/srv/storage/butler/agents/antigravity-complete-error-usage`, using the existing
`tool-turn-usage-lab.sh`, cached dependencies and a disposable internal-network
PostgreSQL database. HTTP/SSE provider requests used an in-process synthetic
transport; no external provider network or authorization material was supplied.

The RED proof on snapshot `20261012-212327-730` is retained at
`reports/tool-turn-usage-20261012T142355Z-1915280`. Complete-known/zero/version and
metadata-only parser assertions failed on the unchanged production code.
Events SHA256:
`afec249db7f50be146ffb033c26e8d14c7d2aa659038151f39b1968c651f9f50`.

Final tested executable/source bytes are snapshot `20261012-212640-126`. The
High fixture wrapper preserves missing frame delimiters instead of repairing
them. Narrow race/vet proof `reports/tool-turn-usage-20261012T142649Z-1924667`
passed 77 named tests / 9 top-level tests / 0 failures / 0 skips. Events SHA256:
`4b720b00d4ddce5fe5ed68b0dce3fac0fc2d2967f4328cc23764f24190e258d7`.

Compatibility proof `reports/tool-turn-usage-20261012T142810Z-1929474` passed
161 named tests / 40 top-level tests / 0 failures / 0 skips, with race and vet.
It covers the whole logical transport and Antigravity packages, actual PG
model-turn observations/reservations, and owning source verification/happy Core
execution. Known failed measurements remain one historical observation; NULL is
not zero, conservative spending is retained and stale task authority cannot admit
another provider entry. Events SHA256:
`2db57cb0d48dd7906b0bf816c490dac3fc4f61b9c32984e3b50e89cc08a295a0`.
Complete tested source manifest SHA256:
`5f9cb6ab0124ebe6421b9f1248d4f40d275542a47c6871d14bb11c25d1732cbf`.
This evidence document was appended after those runs; no executable logic changed.
