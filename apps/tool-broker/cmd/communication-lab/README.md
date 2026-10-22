# Isolated communication Broker

Uses the normal Broker policy, authorization, budgets, effects and exact private
dispatch, adding canonical message/photo/report contracts through
`app.NewForCommunication`. Requires a disposable `_test` PostgreSQL database,
the normal private action token-file reference and authoritative Core context
endpoint. Start with `communication-lab -control-url
http://127.0.0.1:PORT/internal/communication` using the actual fixed
`communicationcontrol.Path` from the source; HTTPS is required outside supported
loopback/Compose endpoints. The default binary/config remains unchanged.

Communication contracts in static registry files are rejected; the model cannot
install or replace them. The injected executor still requires dispatched exact
action identity and owning Core revalidation. Widgets are not registered here.
