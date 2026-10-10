# Reviewed HTTP-JSON executor

This Go runtime interprets reviewed `butler.http-json.v1` contracts from Tool
Broker. It is separate from orchestration, credential storage and owner control.
It cannot install code, run commands, fetch secrets directly or mount a workspace.

Operator configuration supplies `BUTLER_EXTENSION_PROFILES_JSON` and dedicated
`BUTLER_EXTENSION_RUNTIME_TOKEN_FILE`. The same immutable profile descriptor is
configured on Broker. Every Execute/ProbeExtension RPC requires this narrow token,
distinct from provider, owner and core service tokens. No PG/Redis/model auth is
passed to the executor.

Calls need private action metadata, public origin/path bounds, fixed methods,
deadlines and byte limits. Private DNS/IPs and redirects are denied. Only the
fixed declared Authorization slot accepts Broker-resolved refs; observations
mask the values. Reviewed recipient receipts use Broker-owned key/fingerprint.
HTTP 2xx does not claim semantic completion. Fixed credential-free readiness
produces only health/digest; the runtime makes no orchestration decisions.

Tests exercise malicious proposals, private/off-scope destinations, duplicates,
raw/unsolicited secrets, changed receipt contracts, byte/output bounds, redirects
and read-only readiness. Resolver/dial fixture injection exists only in test
construction and is not selectable by environment, model args or packages.

See [the lifecycle spec](../../docs/architecture/reviewed-extension-lifecycle.md)
and optional installation instructions under `deploy/extensions`.

The optional container uses UID10001, a read-only root, no capabilities,
no-new-privileges, fixed process/RAM bounds, no host/Docker mount and no backend
PG network. The only secret mount authenticates private Broker RPCs. An isolated
real-process acceptance uses `scripts/dev/lab-extensions.sh`; the separate
`lab-extension-container.sh` checks actual image confinement with networking
disabled. Those tests do not provide a production private-address bypass.
