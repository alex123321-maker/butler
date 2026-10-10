# Private lab proxy

The optional `deploy/docker-compose.antigravity-proxy.lab.yml` overlay runs an
existing operator-provided Xray binary as a separate service on Butler's backend
network. It forwards the Orchestrator's HTTPS requests through `HTTPS_PROXY`.
Go's standard HTTP transport honors this variable, including provider OAuth
refresh and native Antigravity requests. Butler's model transport and Tool Broker
still own model turns and tools.

Set `BUTLER_ANTIGRAVITY_PROXY_BINARY_SOURCE` to the executable binary and
`BUTLER_ANTIGRAVITY_PROXY_CONFIG_SOURCE` to a private Xray JSON file. Keep both
outside synced source/model workspaces. The config must be mode `0600`, readable
by UID `10001`, with an HTTP inbound listening on `0.0.0.0:18080` inside the
container. Its host port is published only on loopback. Provision and inspect
subscription material through private operator scripts; never put a subscription
URL, proxy password or full config in chat, logs, source or command arguments.
Proxy logs should omit credentials, destination paths and request bodies.

Compose reads only the selected fixed config; this service does not refresh
subscriptions, select new nodes, read arbitrary secrets or alter another lab
proxy. It runs without capabilities, with a read-only filesystem and restart
policy. Changing its file requires an explicit container restart.

Use this overlay together with the base and lab Compose files. Starting only
`antigravity-proxy` leaves the running Orchestrator unchanged. Recreate the
Orchestrator with the overlay after verifying provider text/function/result
turns, and select the provider separately. Successful proxy connectivity does
not establish model entitlement or available quota.
