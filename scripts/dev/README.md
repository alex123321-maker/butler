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
- `bootstrap.ps1` - create repo-local cache directories and optionally install dependencies into those caches
- `doctor.ps1` - verify that the Windows host has the required tools, env templates, and Docker daemon access
- `up-agent-stack.ps1` - start PostgreSQL, Redis, Ollama, Tool Broker, and tool runtimes in Docker
- `down-agent-stack.ps1` - stop the Docker-side agent stack; add `-RemoveVolumes` for a full reset
- `run-orchestrator.ps1` - run the orchestrator locally against the Docker-side dependencies
- `test-go.ps1` - run Go tests with repo-local cache isolation
- `verify-changed.ps1` - map changed paths to the relevant specs and targeted verification commands
- `lab.ps1` / `lab.sh` - upload source snapshots, run Go checks, deploy, and inspect the isolated SSH lab; see `deploy/README.md`
- `fault-lab.ps1` / `fault-lab.sh` - disposable lab-only acceptance matrix, actual Chromium and backup/restore; see `internal/acceptance/README.md`. Pending product scenarios remain pending even when selected core checks pass.
- `workspace-lab.sh <isolated-agent-root>` - disposable PG/filesystem race proof for task-bound code copies and exact version/hash patch CAS; no shared Compose or production tool wiring.
- `delegation-lab.sh` - disposable PostgreSQL manager proof on a synced isolated agent root: real bounded receipts, shared budget reservations, dependency decisions and stop races with two pool connections; no production delegation enablement or model comparison claim.

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
