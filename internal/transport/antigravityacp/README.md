# Antigravity ACP transport

`antigravity-acp` is a separate, default-off `transport.ModelProvider`. Core uses
its normal execution kernel, context, memory, approval, Tool Broker and delivery
paths. Google CLI runs in a separate isolated service; it never receives Core's
credentials, database, workspace, Docker socket or tool-execution authority.
The existing native `antigravity` transport/authentication is unchanged.

## Admission and accounting

On2026-11-16 the owner approved a distinct opaque subscription budget. Exact
operator policy must declare:

```json
{"models":{"antigravity-acp:gemini-3.8-flash":{"basis":"subscription_agent_no_overage"}}}
```

Core and Broker must use the same policy. Each actual ACP prompt reserves a new
permanent PostgreSQL operation, one prompt, active time, Butler input bytes and
`agent_output_bytes`. Task, conversation, owner/day, mandate and delegation
scope/authority checks remain in force. Native token budgets are not enforced
for this explicitly selected mode: native usage/internal inference counts are
unknown, not free or zero. Attempts have `accounting_mode:opaque_subscription`,
zero not-applicable token allowances and NULL measured tokens. The old native
providers retain token-metered admission. No paid fallback or automatic agent
retry is added; failures/cancellation/restart do not refund reservations.

Migration112 is additive to the existing ledger. Disabling ACP is the normal
application rollback; down refuses opaque attempt history.

## Model and tool loop

Each prompt starts a fresh official CLI process/session with the exact offered
Gemini model and acknowledgement. Ordered, provenance-normalized Butler messages
and current tool schemas are replayed as JSON. Only code-owned typed system
instructions retain authority; observations remain DATA. The managed agent
returns one strict JSON answer or one logical tool proposal. Schema/argument
validation runs before `ToolCallRequested`; only Core/Broker executes it.

Authority is checked before the runtime request, before dispatch after model
negotiation, and before releasing the result. A two-phase private exchange
prevents prompt dispatch if Core closes the connection or refuses authority.
Permission/client RPCs and native tool activity abort the turn. User-message
notifications are ignored, never surfaced as assistant output. Configuration
notifications cannot change the selected model or escalate execution mode.

Live tool results require a matching pending call and current tool snapshot.
An explicit empty snapshot removes capabilities. Durable approval recovery
starts a new context from saved Butler inputs and known results; it does not
replay effects or trust a provider session as authority. At most16 local contexts
are retained, with a15-minute idle expiry. There is no streaming, batch execution,
vision-content transport or provider-owned durable recovery.

## Runtime and operator setup

Core requires only three cold environment-only fields:

- `BUTLER_ANTIGRAVITY_ACP_ENABLED=true` (default false).
- `BUTLER_ANTIGRAVITY_ACP_MODEL=gemini-3.8-flash` (exact identifier).
- `BUTLER_ANTIGRAVITY_ACP_SOCKET=/run/acp/agent.sock` (private Unix socket).

`BUTLER_MODEL_PROVIDER=antigravity-acp` selects the route. Managed database
settings cannot select runtime paths or enable it. Earlier experimental
executable/SHA/profile fields do not grant local CLI execution in Core.

See [runtime service](../../../cmd/acp-runtime/README.md) and the optional
`deploy/docker-compose.acp.yml`. The service is non-root private Docker PID1,
network none, read-only root, no capabilities/privilege escalation and finite
CPU/memory/pids. It owns one private Google profile. Before releasing a result
it kills/reaps the root/group and remaining PID-namespace descendants, including
detached children. A supervisor crash destroys that namespace. Remote Google
work/usage after cancellation is still unknown.

A second private Unix socket reaches fixed Google auth/inference/account-image
hosts through the egress service, with public-address checks and pinned dialing.
A fixed Google DoH bootstrap avoids host fake-IP DNS without allowing private or
metadata addresses. TLS verification remains enabled. No host proxy/profile is
modified. The CLI's empty tools/skills/plugins/MCP/subagent definition, disabled
Swarm customization and no-credit-overage settings are checked before launch.

The logical input limit is2MiB and returned text64KiB. Turn time uses the common
`BUTLER_OPENAI_TIMEOUT_SECONDS` setting, between1 and300 seconds; its declared
budget bound matches that configuration. Earlier scope/authority/caller deadlines
still win. The isolated protocol/process cap is300 seconds, with305-second
finite relay lifetimes. Protocol
caps also cover frames/messages/wire data; these are not native token ceilings.
Login is an explicit official control-plane flow in the dedicated profile,
using the protected one-use code/callback file, not token transplantation.
The profile can retain auth/logs/transcripts and needs private storage/retention.

## Verification

Offline protocol, process, replay, validation, cancellation, policy and isolation
checks are distinct from real provider acceptance. The separate smoke stack in
`deploy/acp/compose.smoke.yml` uses real Core, PostgreSQL, Redis and Tool Broker.
`scripts/dev/start-acp-smoke.ps1` builds its current Go artifacts without changing
laba. Fresh anchor initialization is explicit and never repairs missing history.

Latest live receipts and untested scope belong in issue200. Neither one completed
run nor a stored login proves all tools, account eligibility, production rollout
or protection from Google account restrictions.
