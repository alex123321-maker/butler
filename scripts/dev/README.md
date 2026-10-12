# Butler Dev Scripts

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
- `delegation-acceptance-high-lab.sh /srv/storage/butler/agents/delegation-acceptance-high-20261012 prepare` - new zero-inference PG/Broker/independent-owner/race/vet preparation for full issue16 High acceptance, carrying27 historical attempts in a separate one-use native window. See [the declared scope](../../docs/ai/delegation-full-high-acceptance-20261012.md).
- `delegation-high-lab.sh /srv/storage/butler/agents/delegation-high-20261012 prepare` - integrated disposable PostgreSQL/race/vet and worker/Broker preparation with a committed `BUTLER_DELEGATION_NATIVE_RELEASE`, zero inference and zero allowed skips. The single execute window is permanently sealed after native attempts23-27; see [evidence](../../docs/ai/delegation-high-acceptance-20261012.md).
- `bootstrap.ps1` - create repo-local cache directories and optionally install dependencies into those caches
- `doctor.ps1` - verify that the Windows host has the required tools, env templates, and Docker daemon access
- `up-agent-stack.ps1` - start PostgreSQL, Redis, Ollama, Tool Broker, and tool runtimes in Docker
- `down-agent-stack.ps1` - stop the Docker-side agent stack; add `-RemoveVolumes` for a full reset
- `run-orchestrator.ps1` - run the orchestrator locally against the Docker-side dependencies
- `test-go.ps1` - run Go tests with repo-local cache isolation
- `verify-changed.ps1` - map changed paths to the relevant specs and targeted verification commands
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
