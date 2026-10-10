# Durable execution budgets

This package owns PostgreSQL budget scopes and conservative reservations.
Migration 037 creates the tables. This foundation is not yet wired into the
production model loop, Broker claims or child admission; issue #13 stays open.

Each scope has immutable limits, an absolute deadline and spent reservations.
Kinds are root task, mandate, owner UTC day, and conversation. Scope identities
exclude run IDs and task revisions so new attempts and amendments cannot reset a
task or mandate allowance. A caller derives the root task through Task Service;
it must include every applicable parent/mandate/day scope for child work.
Identity and quote inputs come from trusted control-plane state, never model args.

Counters cover model turns, tokens, integer monetary micros, external calls,
child tasks and active milliseconds. Zero is a hard limit, not unlimited.
Unknown monetary quotes fail closed; the ledger does not guess model prices or
interpret a subscription as an API billing permission. Callers must provide a
conservative, explicit cost basis and bounded token/time allowance.

`Ensure` creates one immutable policy; conflicting defaults do not replace it.
`Reserve` charges all scopes in one transaction under sorted row locks. The stable
system operation ID makes recovery idempotent; changed scope or quote data is a
conflict. `ReserveTx` exposes an owned savepoint for atomic effect/child admission
without taking another pool connection. Failed admission rolls back its own
placeholder and charges even if a caller commits its outer transaction.

Reservations are conservative charges, not proof that a provider used every
reserved token. No timeout/crash automatically refunds a quote: an unknown
model/tool effect cannot release allowance and repeat indefinitely. Actual usage,
normalized loop detection, execution hooks, saved budget stops and owner-visible
read models will be added by the remaining issue #13 implementation.

Tests exercise deadline/zero/overflow/unknown-cost guards and a real PostgreSQL
pool with two connections: sixteen parallel child attempts share task, mandate
and day limits, restart cannot reset spending, replay cannot double-charge, and
failed nested admission cannot persist an idempotency placeholder.
