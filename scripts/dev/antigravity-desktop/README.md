# Desktop / native request interception

This private operator diagnostic captures requests before Google. **No mode
forwards inference.** It is outside Butler production transport and tool execution;
it does not provide CLI/Desktop fallback, change the model, or close issue16.

The installed Windows Desktop2.22.0 frontend was operated through local CDP in a
separate user-data directory and fake Windows HOME. Its `DEV_URL` loads the
official Linux2.22.0 language server, obtained from Google's release artifact,
at the same build CL995734886 as the installed Windows server. This is the
Desktop's supported Linux/WSL backend arrangement, using a development URL for
isolation. It is **not** a capture of the original running Windows backend or
the owner's earlier successful dialogue. The Linux and Windows binary hashes
differ; preserve this qualification when interpreting routing.

Builds, vet, race tests, Docker and credential handling run only on laba beneath
`/srv/storage/butler/agents/antigravity-desktop`. Sync the agent worktree with
`scripts/dev/lab.ps1`. The synthetic runner supports `tests`, `build`, `native`,
`fixture`, `report`, and `stop`. `native` uses the real Go Provider, synthetic
OAuth, exact default URL/model/output8192, loopback TLS and Docker `--network none`;
it checks an actual SSE final. Its synthetic usage never enters real totals.

`fixture` checks the pinned official Linux hash and CL, starts an empty synthetic
profile and replaces Google with local HTTPS. Its Docker network is internal:
there is no outside gateway. Fake token files probe the Desktop standalone
filename, which differs from CLI. Do not infer live routing from incomplete
synthetic feature/config/license responses. In the retained preparation,
GetSlashCommands/default conversational config failed before a primary High
request; a synthetic `tabChat` write was seen. That is a fixture limitation,
not a Butler or live Desktop failure.

After those offline controls were tested, `antigravity-desktop-metadata-lab.sh`
started a **metadata-only** private gateway. It reads the existing private
operator environment and original CLI/import references inside the backend,
requires all four encrypted-PG origin matches, and uses the ordinary Manager
for short-lived access and encrypted refresh persistence. No new Google login
or account import occurs. Only the gateway has the existing backend/proxy
network; the Desktop server stays on the separate internal network. It gets
only a new private profile and CA, not the production operator environment.

Metadata host/path/methods are explicitly allowlisted, with40 requests,64KiB
input,2MiB response,10s/request and a180s forwarding deadline within a300s
process lifetime. Generations, unknown routes, refresh, onboarding, settings,
purchase, telemetry and credit writes cannot be forwarded. Matching original
bearer/access query is required for Google metadata. Redirects are refused.
The exact avatar URL registered from Google's userinfo may be fetched without
bearer or cookies; other URLs are denied. Initial avatar denial caused a local
auth failure; permitting this exact read resolved it. Tokens, userinfo, avatar
URLs/images and raw native logs never appear in safe reports or Git.

`launch-fixture.ps1` starts the installed frontend, an SSH tunnel and a Chromium
proxy allowing only the local tunneled UI. Child processes are hidden; system
proxy/trust, normal Desktop profile and the owner's main checkout are unchanged.
The local driver exports fixed DOM fields only. Broad fixture control labels
are allowed only after a synthetic-only gateway check. `metadata-send` requires
a live report proving metadata-only mode, origin audit and disabled generation,
then inserts the fixed text into the real editor and clicks Send once. It has
no inference approval/admission mode. AI Credit Overages was observed OFF both
in the original Desktop and the diagnostic profile, without changing the switch.

The real-metadata capture reached primary Flash-high/daily Cloud Code v1internal,
65536 output,61694 serialized bytes,21638 system bytes and15 declarations.
The asked49-byte text is also present in the native195-byte request; the full
context differs. LOW auxiliary requests were additionally observed and denied.
Their exact model and whether repeated writes represent retries remain UNKNOWN.
No retry setting was enabled. No Desktop Google SSE/result/usage was obtained.

`fixture-close` closes only the isolated CDP instance. `stop-local.ps1` validates
owned process names/command signatures and protects the owner's PID. Remote
`stop` removes only the named diagnostic containers/network, retains uniquely
named safe reports, and validates the relay process before stopping it. Private
files and all older sealed windows remain preserved. The launcher manifest
prevents silently reusing the same local preparation.

Evidence and exact hashes are in
[`desktop-interception-20261012.json`](../../../deploy/antigravity/desktop-interception-20261012.json).
A real two-client comparison still needs a new tested durable gate carrying all
seven generation reports and21 attempts forward, pinning the observed High
contract, bounding output8192 and blocking every auxiliary/tool/retry write.
The owner's existing diagnostic authorization is retained; this metadata
gateway must never be converted into an unbounded forwarding proxy.
