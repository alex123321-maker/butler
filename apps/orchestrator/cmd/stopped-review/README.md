# Stopped local outcome review

Operator-only Go command for #163. It uses the owning Input/Communication/Action
journal review, with exact Input IDs, existing locks and current database fences.
It invokes no model, tool runtime or channel, changes no credentials and grants
no execution permission. `BUTLER_POSTGRES_URL` is resolved privately from operator
configuration; never pass a DSN or raw secret in command arguments/logs.

Default execution validates and **rolls back** every journal/Input change.
`-apply` commits the same verified review. At most eight named Inputs and twenty
unknown effects per Input are considered, under finite deadlines and NOWAIT
locks. Explicit operator review skips the maintenance cooldown, but retains
all inactive-executor, owner, Input, session and journal checks. The normal
maintenance path retains its five-second cooldown and bounded batch.

Only reviewed historical contracts are currently supported: atomic
communication.message revision1 at commit 4dff78b5, and that revision's fixed
read-only web.fetch. A local publication is confirmed only from its exact
immutable stored receipt; terminal fenced absence proves only that local
publication did not commit. Neither outcome proves channel delivery. A pinned
read's result remains UNKNOWN; its no-write proof may release write isolation.
Unknown contracts, arbitrary writes, active claims/leases, foreign/malformed
receipts or missing dispatch evidence remain blocked.

Old authorization/dispatch ACK uncertainty, tool errors, failed Input/run,
model charges and original evidence remain. No old attempt or grant is revived.
Review one exact scope, inspect the dry-run report, then apply only if appropriate.
Do not run another browser mutation until all applicable blockers are resolved.
