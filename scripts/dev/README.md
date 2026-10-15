# Butler Dev Scripts

Issue33 V4 is now sealed NO_GO after11 new entries, for279 cumulative attempts.
Never Sync or execute its root again. `cloud-v4-final-audit-reproduce.sh` runs
the frozen independent auditor read-only/network-none against named retained
leaves and saves exact content-free output; it invokes no model. The new
`scenario-schema-dialect-high-lab.sh` has its own279→283 marker, exact reviewed
two-input/two-dialect protocol and prepare/execute separation. Preparation has
zero authentication/inference. Its actual native gate remains closed until review.

Historical issue33 V4 used new isolated roots and once-only windows. Its
`scenario-shadow-v4-high-lab.sh ROOT offline|prepare|execute COMMIT` for the fresh
240-case signed quality corpus and four paired High tasks; `offline` has no auth
or inference. The full collector preserves private raw finals after SQL spend and
exports separate safe PostgreSQL diagnostics. `cloud-v4-review` seals exact bytes
only after independent review. `scenario-release-chain-v4-high-lab.sh ROOT prepare|execute COMMIT`
opens native chain execution only from actual independently accepted V4 quality
and comparison evidence. Historical V1–V3 and diagnostic roots remain sealed.

Windows-first helper scripts for Codex Desktop and other host-side agent workflows.

Goals:
- keep Go, npm, Playwright, and Docker client caches inside repo-local `.cache/`
- prefer narrow, repeatable commands over ad-hoc shell setup
- support a hybrid workflow where infra and tool runtimes stay in Docker while the actively edited Go service runs on the host

Recommended flow:

```powershell
pwsh -NoProfile -File scripts/dev/doctor.ps1
pwsh -NoProfile -File scripts/dev/bootstrap.ps1 -ShowEnv -DownloadGoModules
pwsh -NoProfile -File scripts/dev/up-agent-stack.ps1
pwsh -NoProfile -File scripts/dev/run-orchestrator.ps1
```

Available scripts:
- `tool-turn-usage-lab.sh AGENT_ROOT` — future complete-turn usage in Tiered/High synthetic SSE and real disposable PostgreSQL budget wrapper, NULL/malformed/duplicate/cancel/source-denied cases, race/vet; no actual model calls, private auth or old history edits.
- `owner-state-lab.sh AGENT_ROOT` — disposable PostgreSQL, actual owner HTTP/authority/replay and safe approval response checks; all Telegram uses fake local HTTP. Race/vet, zero inference and zero human messages.
- `owner-state-web-lab.sh AGENT_ROOT` — targeted actual Chromium presentation scenarios, TypeScript/lint/build. HTTP observations are intercepted synthetic fixtures; this is not external-effect evidence. Excludes the unrelated stale all-page screenshot baseline whose merged Skills/Converters sidebar changed before this task.
- `owner-state-browser-api-lab.sh AGENT_ROOT` — actual Chromium → Go production owner handlers → disposable PostgreSQL; paused reload, branch controls and committed lost-ACK replay with independently checked durable event count. Seeded effect/result observations are labelled fixture data; no provider/executor/main deployment is present.
- `delegation-final-high-lab.sh /srv/storage/butler/agents/delegation-final-high-20261012 prepare` - separately bounded final two-call High window39→41, thirteen immutable history pins, exact tested source/release and binary, zero-inference preparation, and a mandatory authless two-lane daily DNS/CONNECT/TLS prerequisite before any auth import. See [probe contract](delegation-final-tls/README.md) and [declared scope](../../docs/ai/delegation-final-high-acceptance-20261012.md).
- `delegation-remainder-high-lab.sh /srv/storage/butler/agents/delegation-remainder-high-v2-20261012 prepare` - twelve immutable history pins and labelled scripted parents/width1, with only two real High width2 child slots at the original39 ceiling. Both execute markers are sealed; the latest report retains two connection failures and39 total attempts. See [evidence](../../docs/ai/delegation-high-remainder-acceptance-20261012.md).
- `delegation-acceptance-high-lab.sh /srv/storage/butler/agents/delegation-acceptance-high-20261012 prepare` - new zero-inference PG/Broker/independent-owner/race/vet preparation for full issue16 High acceptance, carrying27 historical attempts in a separate one-use native window. See [the declared scope](../../docs/ai/delegation-full-high-acceptance-20261012.md).
- `delegation-high-lab.sh /srv/storage/butler/agents/delegation-high-20261012 prepare` - integrated disposable PostgreSQL/race/vet and worker/Broker preparation with a committed `BUTLER_DELEGATION_NATIVE_RELEASE`, zero inference and zero allowed skips. The single execute window is permanently sealed after native attempts23-27; see [evidence](../../docs/ai/delegation-high-acceptance-20261012.md).
- `bootstrap.ps1` - create repo-local cache directories and optionally install dependencies into those caches
- `doctor.ps1` - verify that the Windows host has the required tools, env templates, and Docker daemon access
- `up-agent-stack.ps1` - start PostgreSQL, Redis, Ollama, Tool Broker, and tool runtimes in Docker
- `down-agent-stack.ps1` - stop the Docker-side agent stack; add `-RemoveVolumes` for a full reset
- `run-orchestrator.ps1` - run the orchestrator locally against the Docker-side dependencies
- `test-go.ps1` - run Go tests with repo-local cache isolation
- `verify-changed.ps1` - map changed paths to the relevant specs and targeted verification commands
- `memory-lifecycle-lab.sh <isolated-issue83-root> <fresh-label>` - shared synthetic poisoning corpus, actual PG/race/Broker and old-backup deletion-anchor restore; exact mandatory tests cannot skip. See [the consumer lifecycle gate](../../docs/ai/memory-lifecycle-acceptance.md).
- `task-observations-lab.sh <isolated-agent-root>` - disposable PostgreSQL and Go race/vet proof for authenticated owner observations, shared root usage and truthful effect/result distinctions; no inference or shared deployment changes.
- `task-view-lab.sh <isolated-agent-root>` - Vue type/lint/build and actual Chromium task/control/observation fixtures with repo-local lab caches.
- `optional-models-integration-lab.sh <isolated-agent-root>` - actual disposable PG/race/vet checks for the combined voice/function proposal, input-source and task/API code; actual model quality probes remain explicit opt-ins and skips are audited separately.
- `lab.ps1` / `lab.sh` - upload source snapshots, run Go checks, deploy, and inspect the isolated SSH lab; see `deploy/README.md`
- `fault-lab.ps1` / `fault-lab.sh` - disposable lab-only acceptance matrix, actual Chromium and backup/restore; see `internal/acceptance/README.md`. Pending product scenarios remain pending even when selected core checks pass.
- `workspace-lab.sh <isolated-agent-root>` - disposable PG/filesystem race proof for task-bound code copies and exact version/hash patch CAS; no shared Compose or production tool wiring.
- `delegation-lab.sh` - disposable PostgreSQL manager proof on a synced isolated agent root: real bounded receipts, shared budget reservations, dependency decisions and stop races with two pool connections; no production delegation enablement or model comparison claim.
- `delegation-attempts-lab.sh <isolated-agent-root> numeric` - narrow actual PostgreSQL/race/vet proof for selected numeric witness acceptance, retained contradictory results and agreeing unsupported finals; `verification` also checks existing owner decision and legacy promotion controls. All modes use synthetic model outcomes and no credentials or inference.
- `delegation-receipts-lab.sh <isolated-agent-root> narrow|full` - actual private Broker/core/PG receipt preservation and final-turn failure checks, synthetic High serialization and recovery DATA checks; no account credentials or inference. Full mode also runs core/transport and existing Broker admission regressions.
- `lab-converter-container.sh <isolated-agent-root>` - actual production Broker and network-none converter proof, including ordinary parent/child workers with explicit and automatic055 continuations. Run `lab.ps1 -Action Sync` first; tests use that fixed snapshot, no native inference or shared deployment changes.

Optional dependency install:

```powershell
pwsh -NoProfile -File scripts/dev/bootstrap.ps1 -InstallAllDependencies
```

Targeted Go verification examples:

```powershell
pwsh -NoProfile -File scripts/dev/test-go.ps1 -Packages ./apps/orchestrator/...
pwsh -NoProfile -File scripts/dev/test-go.ps1 -Packages ./internal/memory/...
pwsh -NoProfile -File scripts/dev/test-go.ps1 -Packages ./internal/transport/...
```
- `delegation-remainder-high-lab.sh`: explicit two-call High child remainder at37→39, with scripted parent/width1 component baselines clearly separated; carries all11 pinned artifacts and never reopens earlier windows. See `docs/ai/delegation-high-remainder-acceptance-20261012.md`.
