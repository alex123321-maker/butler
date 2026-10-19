# Tool discovery contracts

`tools.search` returns bounded cards from an already authorized catalog.
`tools.load` selects exact immutable contract refs; it does not execute a method.
The package owns fixed schemas, deterministic ranking, cursor identity and
selection references. Broker owns authority/dispatch, Orchestrator owns selected
model schemas, and PostgreSQL tool receipts own durable selection.

See [the contract](../../docs/architecture/tool-discovery.md). The matching
operator flag on both services defaults to false. No new dependencies are used.
