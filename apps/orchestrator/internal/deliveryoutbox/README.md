# Delivery outbox

PostgreSQL-backed final notifications with stable IDs, per-chunk ACKs, bounded
claims/retries/deadlines, and explicit unknown send outcomes. The worker sends
only stored text; it cannot call a model or tool. Telegram unknown outcomes and
expired sending claims are never automatically retried. Web readiness means a
stored result is accessible, not that a client read it.

The Telegram adapter performs one plain-text network attempt and returns a typed
safe outcome. See [atomic result/delivery contract](../../../../docs/architecture/atomic-result-delivery.md).

Task supplies a separate `VerifiedNotificationStore` to the same channel-only
worker for opted-in verified task notices. The store rechecks current owner,
result/privacy/assignment and the exact live claim. Delivery reconstruction has
no model/tool capability. Lost ACK becomes UNKNOWN; only confirmed absence can
retry the same immutable notice.
