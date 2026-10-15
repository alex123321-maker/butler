# Independent memory-eval v2 runner first-pass rejection, 2026-10-14

**Decision at first pass: NO_GO for native entry.** Golden/data/criteria approval
remains `PASS_GOLDEN_REVIEW_ONLY` for its exact approved pins. This separate
review records substantive collector/auditor findings detected while the root
was still editing the working tree based on commit
`4796775bc724cf24c5ede0c9bcb395b8bb50d019`. It is preserved as the initial review
trail. A later corrected source needs a distinct stable byte-pinned review and
targeted negative/race proof before the root can admit a new native window.

Reviewer: `codex-independent-memory-review-20261014`, task
`/root/memory_independent_review`, a separate human-authorized agent who authored
neither candidate implementation nor dataset. Reviewed code was evolving and
has no final source/compiled-binary pin in this initial declaration. The reviewer
performed zero native calls, accessed no private credential or signing material,
and changed only this review document.

## Inspected boundary

The reviewer read the collector, native entry, safe-event/export helpers and
component fixtures in `apps/orchestrator/internal/app/memory_eval_*_test.go`;
`internal/acceptance/memory_runner_audit.go`, `memory_runtime_pins.go`,
`memory_v2.go` and `memory_score.go`; audit CLI; the external data seal helper;
and one-use preparation/execution/audit script. Relevant source/catalog/budget,
native protocol and existing current-source guard code were followed to check
their actual ownership semantics. Architecture lookup included memory, model
transport, run lifecycle and provider credential boundaries.

## Findings delivered to the implementing root

| ID | First-pass finding and required proof |
| --- | --- |
| N1 | Initial source check and model admission were separated from actual HTTP dispatch; final check occurred only after received output. Require an owning dispatch callback plus bounded cancellation/recheck during inference and before accepted final. Test task amendment, source deletion and lease loss at those boundaries. |
| N2 | Generation expense was recorded after exclusive final/event writes and the final source check. Any intervening failure could leave a charged model admission without its stage expense. Persist usage/attempt/expense first and retain admitted unknown prefixes on every failure. |
| N3 | Capture wrote sources and spent local inference before success-only write/background/read records; missing records could later be marked unused. Retain actual or explicit unknown failed-stage prefixes and every owning auxiliary reservation, request and response. |
| N4 | Auditor hashed original measurements without comparing their decoded content to the scored import; it merely checked that pre-native freeze existed. Verify exact original measurement content and the whole frozen plan/input/capture identity. |
| N5 | Original event audit accepted one matching usage event while ignoring conflicting others, did not require observed model version, and could accept final after terminal. Require matching run/provider/event IDs and timestamps, consistent complete usage/version, one STOP final followed by one clean terminal, and no extra/tool/error lifecycle. |
| N6 | Admission/reservation/result searches found any match rather than exact complete unique coverage. Require one-to-one native and auxiliary ownership/expense joins, reject extra/dropped/reused records and preserve failures. |
| N7 | Exported source/revision/object/index/task/run/lease rows were not inspected by the auditor. Correlate original permitted bodies, aliases, query, bundle and activated generation with each case owner; validate protected current task/constraint revision and historical lease/guard times. Original IPC hash alone does not identify permitted evidence. |
| N8 | Event record dropped safe run/provider/event/time metadata; request correlation used input digest without preserving the exact instructions/model projection/native schema. Retain this permitted request/event metadata and validate it against the frozen arm and admission. Keep raw provider frames, headers and auth outside evidence. |
| N9 | Unconditional JSON-string unwrapping confused genuine valid-string model output with a retention wrapper for malformed output. Use explicit observation encoding and compare/hash/score exact original bytes. Apply output bounds to original bytes rather than escaped retention size; retain schema-invalid answers as negative observations. |
| N10 | Full-context/current source reads and serialized input preparation were marked unused; native latency excluded all prior retrieval/index capture. Account per-arm reads and shared preparation explicitly, and retain the declared native and retrieval pipeline latency boundaries separately. Adding retrieval preparation to every baseline would count unrelated work repeatedly. |
| N11 | Conservative native input reservation omitted response schema, JSON escaping and request framing. Bound the complete actual serialized provider input, output and hidden-token total before dispatch; use the same cap in transport and ledger. Fixed64KiB reservation for all64 possible native turns alone exceeds the reviewed4,000,000-unit window, so compute a safe smaller bound or obtain a new policy review. |
| N12 | Owner actor context replaced the experiment deadline with Background for catalog/index write/review operations. Carry the bounded context through those operations; close the window and persist completion/failure artifacts successfully before claiming complete collection. |
| N13 | Hardware recorded image identities but commands used mutable tags; resource inspection file was collected without enforcement. Execute the exact inspected image IDs and gate typed CPU/RAM/PID/UID/read-only/mount/network metadata before private provider state. |
| N14 | `timeout docker start -a` can stop only the attached client, leaving the runner active. Explicitly stop the runner on timeout/failure and retain final container state. Native/auxiliary work must remain within the one-use durable entry and deadline. |
| N15 | Private bind-mounted report volume and native event count lacked finite bounds. Bound report bytes and retained event count, preserving owning spend before stopping on the bound. |
| N16 | The new dispatch helper returned parent cancellation as source-stale even without an installed callback, changing other Antigravity callers. Preserve unhooked behavior and ordinary cancellation; distinguish bounded source-check timeout from stale source. Add meaningful transport tests and corresponding spec/README changes. |

These findings were communicated as they were detected. Several corrections
were visible during subsequent reads, but this evolving source was not admitted
by a stable final review. Passing existing component tests before those new
paths are exercised does not resolve the findings by itself.

One initial caveat was explicitly withdrawn after source inspection:
`catalog.AccessTx` does check current ownership/run/task authority even when
the refs array is empty. Therefore `checkSources` covers no_ltm authority too.
N1 concerns dispatch/event timing, not absence of that underlying catalog check.

## Methodological limits to preserve

The configured collector composes the actual owned indexing, hybrid reader and
bundle builder for user-scoped semantic chunks. Profile, episode, working and
summary stores are unconfigured, and extraction/summary/consolidation are off.
State this bounded current-local-retrieval configuration explicitly; any future
nonempty class needs every backing original and claim/provenance.

Source status and old/current preference evidence remain DATA; synthetic catalog
imports and index activation do not establish general owner facts or authority.
Proposed next actions and narrow synthetic QA assessments do not complete a
file, repair, catalogue, delivery or long-lived user goal. Broader effect success
must stay null without independent runner evidence. Unknown cloud invoice,
embedding tokenizer and host energy remain unknown; measured amounts are
subtotals, not proof of complete cost or monetary cap compliance.

The once-only signer was reviewed as a data/criteria seal helper: exact external
review document/declaration/raw/semantic/criteria pins must match before key
generation; output files are exclusive/private; private signing bytes are not
persisted or exposed. That seal does not approve this collector, admit inference
or reopen an old window. Shared-host role independence and the public/nonblind
small corpus remain explicit.

## Disposition

Retain all original failures and this initial NO_GO. Re-review a fixed final
source, exact compiled runner/CLI/helper and enforced hardware/window metadata;
require targeted corruption/order/alias/expense-loss/current-authority/resource
tests and separate operator-owned original trace/ledger correlation. The root
must make a fresh finite native window only after that independent review.
No quality, promotion, merge, deployment or issue closure is approved here.
