# Antigravity ACP integration decision

Date:2026-11-16. Issue:[200](https://github.com/alex123321-maker/butler/issues/200).
Status: operator-enabled on primary laba after isolated and composed checks.
The main agent uses ACP; native-qualified background memory stays on its
explicitly selected existing native backend. See acceptance receipts below.

Affected subsystems: model transport, execution-budget admission/read model,
operator configuration, runtime/container boundary, private CLI authentication
and provider status UI. Session ownership, PostgreSQL authority, memory layers,
Tool Broker policy, credential references and channel delivery remain separate.
No new Go dependency or infrastructure outside Docker Compose is introduced.

## Owner decision about opaque metering

The owner explicitly accepted request/time/data budgets for subscription ACP,
with unknown native token counts and no automatic credit purchase. The separate
`subscription_agent_no_overage` exact provider/model basis implements that choice.
It is not a fake `InferenceLimits` declaration or a bypass of the ledger.
Missing/ordinary native basis still denies ACP admission. Token-metered adapters
retain the old strict policy. See the [execution-budget contract](execution-budget-contract.md).

Every prompt has a fresh permanently spent reservation under current ownership,
source/authority versions and all applicable scopes. The unit is one ACP prompt,
not a proven single Google inference. Actual token measurements stay NULL.
Migration112 permits only the distinct ACP provider's not-applicable zero token
allowances; read models label opaque attempts explicitly. Time/data limits,
failed-operation spending, cancellation and no automatic agent retry remain.

## Execution and authentication boundary

Core's default-off adapter dials only a private Unix socket. It neither launches
the CLI nor mounts its profile. The dedicated runtime executes official Linux
CLI1.5.9, archive SHA-512:

`d01eb13ba7750eed18907f1ae8860864371e0ad9064415e8d033d22b4bd7b9b4e4c7f962058520b452d67db1c33163d9a889d55a7dffa159596ec6cd91a87b79`.

The executable SHA-256 is
`39816d49d5ab92229952987225c23ea3fed7580699dcb4a444e0388e0ac1ce43`.
The build verifies the archive, image verifies the executable, and the launcher
hashes/executes an opened ELF descriptor. Ubuntu/glibc and CA roots have pinned
base-image digests. The runtime is private non-root PID1, network none,
read-only root, no caps, no-new-privileges and finite CPU/memory/pids. The root
and all remaining private-namespace children are killed/reaped before output;
namespace destruction handles supervisor exit. This does not prove that remote
Google computation stopped or supply a remote cost/usage measurement.

The managed custom agent disables native tools, skills, MCP, plugins, commands
and subagents. The CLI's automatically enabled Swarm customization is explicitly
disabled. Benign JSON formatting changes are accepted canonically; unknown or
unsafe settings/customizations are not repaired silently. Protocol tool activity
and permission requests abort the turn. Only logical proposals go to Butler.

A separate egress service permits fixed Google auth/inference/account-image
hosts, not arbitrary web/Butler/private/metadata access. The runtime reaches it
through a Unix socket and loopback relay. A fixed TLS Google DoH bootstrap avoids
the observed198.18 fake-IP host answers; normal public-address guards remain.
There is no TLS interception or owner proxy reconfiguration.

On laba the fixed egress dial explicitly chains through the already configured
`antigravity-proxy:18080`. Only this Go egress sidecar joins its existing network;
the CLI remains network-none. CONNECT receives the previously checked public
IP:443, never a model-selected hostname/private target or proxy credential.
The same route carries fixed Google DoH bootstrap. Direct AICode HEAD timed out;
that failed preflight is retained and did not trigger another login window.

Official `agy acp --login` runs only in that profile, without inference. The owner
submits a one-use code or exact Google callback URL through a protected mode0600
file with atomic delivery; it is deleted after reading. Raw codes, tokens and
CLI logs never enter Core/model context. Login success and exact-model protocol
readiness have been observed. Login does not prove entitlement/quota or resolve
Google's consumer-account policy ambiguities; no identity disguise, account
rotation, token copying, API-key fallback or no-ban promise is implemented.

## Butler integration

The adapter replays current normalized messages/tool schemas into a fresh Google
session per turn. Core checks authority before connection, immediately before
prompt dispatch and before releasing an answer/proposal. The private two-phase
exchange preserves the pre-prompt dispatch checkpoint. Tool results stay DATA;
current tool replacement and strict response validation remain mandatory.

Approvals and recovery use existing durable Butler context. No provider handle
can resume an effect or grant permissions. Schedules, initiative and converter
preflight support the same explicit opaque budget. The active provider UI reports
operator-managed authentication without claiming cached login means connected.

## Evidence and remaining acceptance

Real PostgreSQL tests cover opaque reservations, unknown observations, restart
spending and migration rollback refusal; native reservation regressions remain.
A non-root private PID1 Docker test kills/reaps a detached descendant. Unit tests
cover protocol updates/denials, replay, validation, cancellation, authority,
private profile configuration and egress denial, including fake-IP/metadata IPs.

Live public TLS checks reach OAuth, AICode and Cloud Code without credentials or
inference. The separate acceptance stack runs real Core, migration112,
PostgreSQL, Redis, Broker and a doctor runtime. A completed text run returned
`ACP_BUTLER_OK: 42` through the ordinary Web ingress and execution kernel.
Tool/approval/restart/delivery and broader functionality have separate receipts;
only completed checked cases may be marked accepted in issue200.

The completed doctor case used input `input-f6beb47a21aa876ac85d0e696c9b56c5`
and run `run-1794773639976597817-793d6b0003b7792a`. Core restarted while waiting
for owner approval; the exact action then completed through the real Broker and
doctor runtime, with `effect_confirmed` and `dispatch_completed`. Two ACP turns
retained NULL token observations. The final honestly reported degraded checks.
An earlier missing-anchor case remains failed and is not counted as successful.

## Independent qualified memory backend

ACP foreground does not inherit native memory-consumer qualification. The initial
rollout preserved these consumers through explicit cold operator selection
`BUTLER_MEMORY_MODEL_PROVIDER=antigravity`, the existing measured Flash High
32768/300s profile and its separate native budget basis. This preserves existing
candidate/summary/consolidation/temporal capabilities and does not retry failed
ACP requests on native transport. The main ACP agent can use their ordinary
Broker/read APIs. Background generation through ACP needs the separate consumer
qualification below; transport/doctor tests alone do not supply it.
Without that explicit selection startup refuses a silent capability downgrade.

The subsequent independent ACP compatibility gate covers the fixed32-case
Russian extraction corpus (23/24 core semantics,8/8 holdout,32/32 citations,
zero unsafe promotions), exact-prefix summary publication and recovery of the
retained transcript after a real300s timeout, duplicate/UNKNOWN consolidation,
and live cancellation/revocation. Shared PostgreSQL lifecycle, source, authority,
publication and immutable-spend regressions also ran. Failed windows stay failed;
native token, cost or latency parity is not asserted. Code-owned ACP gates accept
only exact `gemini-3.8-flash`,300s,2MiB/64KiB declared transport bounds and a matching
explicit subscription quote. Experience qualification is unchanged. Explicit
`BUTLER_MEMORY_MODEL_PROVIDER=antigravity-acp` selects this checked profile;
existing token policies wait visibly rather than acquiring a different meaning.
Native remains an explicit independent operator choice. The extended issue200
acceptance report records actual deployment separately.

Earlier failed login/runtime windows and spent/unknown attempts are retained,
not relabeled successful. No code was merged. Primary rollout is a separate
operator-authorized action with retained immutable artifacts, private configuration
backup and tested rollback. Google account policy and real external effects
remain distinct from transport success.

On primary laba, input `input-9ccc85de1a6a10bf0bce9ecf71d77a4c`, run
`run-1794776414397795534-96c224d6eb8b4f28`, completed two actual ACP turns and
`communication.message`. `ACP_LABA_OK: 42` was acknowledged by Telegram and read
through the protected Web feed. Token observations remained NULL. The deployed
Web ACP settings bundle was fetched and its hash matched the staged artifact.
The earlier missing-catalog deployment test failed before model admission and
was retained. Both mounted artifact directories now preserve their original
`tools.json` bytes. A30-second broader-tool test then timed out before any tool;
the configured provider bound now supports the existing300-second primary policy
without automatic replay or a spending reset. Full browser/credential/schedule/
delegation and background-ACP acceptance are not implied by the message receipt.

## Primary references

- [Official ACP/custom clients](https://antigravity.google/docs/ide/extensions/acp.md)
- [CLI ACP](https://antigravity.google/docs/cli/acp.md)
- [Custom agents](https://antigravity.google/docs/subagents.md)
- [CLI reference/settings](https://antigravity.google/docs/cli/reference.md)
- [Permissions/sandbox](https://antigravity.google/docs/sandbox.md)
- [Account restrictions FAQ](https://antigravity.google/docs/faq.md)
- [Official CLI installer](https://antigravity.google/cli/install.sh)
- [ACP protocol](https://agentclientprotocol.com/protocol/prompt-turn.md)
- [T3 integration precedent](https://github.com/pingdotgg/t3code/blob/d8c7855a884c58715c2d66ca82a3334bd6bc398c/apps/server/src/provider/acp/AntigravityAcpSupport.ts)

The T3 precedent is not Butler acceptance and no upstream implementation was copied.
