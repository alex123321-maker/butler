# Telegram queued behind a stopped request, 2026-10-25

The owner's Telegram Input `input-38ecc174ff3650e9b98b6c384950ac61` was accepted
at 02:41:17 UTC and remained queued with zero execution claims. An earlier Input
in that same session, `input-51ae3706056a7d1e9093886ce766263c`, was blocked at
`tool_dispatching` with `effect_unknown`. Session ordering correctly prevented
the newer request from executing. Only the generic accepted notice was sent.

The older Input predated the execution-notice activation watermark. Therefore
its own failure notice was excluded, and the newer queued notice did not explain
the dependency. The earlier native smoke used Web ingress and Telegram outbound
delivery; it did not prove that the Telegram session's incoming queue was clear.

## Immediate recovery

The old run had no active execution lease/claim. Its owning action journal had
seven confirmed reads and one allowed credential-control preparation, with no
executing or unknown effects. The authenticated owner `close-failed` API accepted
the exact checkpoint/error and no unknown-effect acknowledgements (HTTP 200).
The old Input/run became failed; checkpoint, error, actions and usage were kept.
No action was replayed, uncertainty reconciled or spending reset.

The original queued Telegram Input then executed automatically with its first
claim, without another user request. Run
`run-1792897275642327231-e6c205b4747d9ac1` completed at 03:01:44 UTC. Its result
publication was available through protected Web read (HTTP 200), and Telegram
acknowledged message 171. This is actual Telegram ingress through execution and
delivery, not a Web-origin replacement request.

## General fix and checks

For a current queued Input, execution notices now inspect the earliest unfinished
predecessor in the same owner's session. Blocked/waiting predecessors produce
fixed `queue_blocked` text; a running predecessor stays ordinary queueing.
Only dependency status/timestamp are used, including heads accepted before
notice activation. Historical content/errors/notices remain excluded. The
projection cannot skip, close or retry a predecessor. Send authorization checks
the dependency again and suppresses stale warnings after it clears.

Migration101 extends the finite notice code/reason constraints. Down migration
refuses to discard or relabel already stored dependency notices/receipts.

Windows narrow notice checks passed. Isolated PostgreSQL 17, Go 1.27.2 Linux
`-race -count=1` tests passed 41 cases including subtests, with no skips, plus
`go vet` and a Core build. These cover blocked/waiting/running Telegram heads,
pre-activation history exclusion, unchanged-state deduplication, stale send
denial, preserved session ordering and release of the original queued Input.
Existing owner close guards and audit/uncertainty retention tests also passed.
Warning delivery uses scripted ACKs in the isolated tests; the native recovery
above establishes actual delivery of the owner's result, not a live injected
queue-block warning.

Private reports in `/srv/storage/butler/agents/research-recovery-20261024`:
`reports/telegram-queue-notices-race.jsonl`,
`reports/telegram-queue-notices-vet.log`, and
`reports/telegram-queue-native-recovery.json`. Test containers/network were
removed after verification. No queued request or failure was manufactured in
the working service to test a warning.
