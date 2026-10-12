# Isolated Antigravity wire capture

Operator diagnostic, not an inference adapter. `antigravity-wire-lab.sh ROOT`
builds and runs the Go command on laba in an isolated agent snapshot. All Docker
runs use `--network none`, a read-only source/binary, no capabilities, and private
temporary storage. The command additionally rejects any non-loopback interface.
It imports no operator environment and reads no real profile, encrypted PG or
credential import. The module cache is reused read-only; build/temp caches and
reports belong to the isolated agent root.

The SHA256-pinned installed CLI 1.3.3 gets a new synthetic consumer OAuth record,
empty work directory and tool-free agent. An ephemeral fixture CA and fixed-host
CONNECT proxy route HTTPS exclusively to the local synthetic server. The proxy
never dials a request-selected host. Metadata responses are fixtures, including
the Flash model enum observed in the earlier safe server metadata. They are not
the real account's routing configuration or license proof.

The CLI is cancelled on its first serialized generation request, before the
fixture returns an error. The gate asserts exactly one captured CLI attempt;
CLI exit failure is expected and is not a model result. The same process then
runs the actual Go Provider with synthetic auth and the same user prompt; its
fixture SSE marker must complete. Reports project only wire keys, public routing
values, header metadata and booleans. No authorization value, token, project/
session ID, input/system body, thought or provider diagnostic is exported.

Modes:

- `default`: CLI environment without an inference route override.
- `aicode`: explicit synthetic `USE_AICODE=true` experiment. This is not a
  proposed production fallback; the captured experience must be examined before
  claiming that CLI and Butler requested the same model.
- `tests`: race tests for transport/auth/factory/config and diagnostic guards,
  plus vet, with no real PostgreSQL or credentials. Integration skips are explicit.

First sync the source with `scripts/dev/lab.ps1 -Action Sync -RemoteRoot
/srv/storage/butler/agents/antigravity-offline`, then run on laba:

```sh
bash /srv/storage/butler/agents/antigravity-offline/current/scripts/dev/antigravity-wire-lab.sh /srv/storage/butler/agents/antigravity-offline default
bash /srv/storage/butler/agents/antigravity-offline/current/scripts/dev/antigravity-wire-lab.sh /srv/storage/butler/agents/antigravity-offline aicode
bash /srv/storage/butler/agents/antigravity-offline/current/scripts/dev/antigravity-wire-lab.sh /srv/storage/butler/agents/antigravity-offline tests
```

This never opens a sealed native window or charges its ledger. Synthetic
serialization is not authenticated Google acceptance, model quality, quota
metering or an identical-context real-client comparison. See
[`antigravity-quota-diagnosis.md`](../../../docs/ai/antigravity-quota-diagnosis.md).
