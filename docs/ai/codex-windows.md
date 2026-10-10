# Butler with Codex Desktop on Windows

Practical host-side workflow for iterative development with Codex Desktop.

## Why this exists

The repository already has strong agent-facing rules and subsystem docs, but Codex Desktop on Windows benefits from a few extra conventions:
- avoid user-profile cache directories that are often blocked or noisy in sandboxed runs
- keep infra and tool runtimes in Docker, while running the actively edited Go service on the host
- prefer narrow verification commands that match the changed subsystem

## Recommended workflow

1. Create `.env` from `.env.codex-windows.example` or `.env.example`.
2. Prepare repo-local caches and prefetch host-side dependencies:

```powershell
pwsh -NoProfile -File scripts/dev/bootstrap.ps1 -ShowEnv -DownloadGoModules
```

3. Start Docker-side dependencies and runtimes:

```powershell
pwsh -NoProfile -File scripts/dev/up-agent-stack.ps1
```

4. Run the orchestrator locally:

```powershell
pwsh -NoProfile -File scripts/dev/run-orchestrator.ps1
```

This hybrid flow keeps:
- Docker: `postgres`, `redis`, `ollama`, `migrator`, `tool-http`, `tool-browser`, `tool-doctor`, `tool-broker`
- Host: the Go service currently under active development

## Repo-local cache policy

The dev scripts pin the following caches under `.cache/`:
- `GOCACHE`
- `GOMODCACHE`
- `NPM_CONFIG_CACHE`
- `PLAYWRIGHT_BROWSERS_PATH`
- `DOCKER_CONFIG`

This avoids common Windows/Codex friction around `%LOCALAPPDATA%`, `%USERPROFILE%\.docker`, and Playwright browser downloads.

## Fast verification loop

Use the narrowest matching checks first:

```powershell
pwsh -NoProfile -File scripts/dev/verify-changed.ps1
pwsh -NoProfile -File scripts/dev/test-go.ps1 -Packages ./apps/orchestrator/...
pwsh -NoProfile -File scripts/dev/test-go.ps1 -Packages ./internal/memory/...
pwsh -NoProfile -File scripts/dev/test-go.ps1 -Packages ./internal/transport/...
```

For frontend work:

```powershell
cd web
npm run lint
npm run test:e2e
```

If host-side Go modules are still missing, rerun:

```powershell
pwsh -NoProfile -File scripts/dev/bootstrap.ps1 -DownloadGoModules
```

## Butler-specific playbooks for Codex

OpenCode already has focused Butler prompts in `prompts/`. They are also useful as role playbooks for Codex:
- `prompts/butler-build.md` - implementation-first mode
- `prompts/butler-plan.md` - planning and impact analysis
- `prompts/butler-architecture.md` - service boundaries and spec conflicts
- `prompts/butler-transport.md` - provider and transport work
- `prompts/butler-tools.md` - Tool Broker and runtime concerns
- `prompts/butler-memory.md` - memory model and retrieval
- `prompts/butler-security.md` - credentials and approval logic
- `prompts/butler-doctor.md` - observability and diagnostics

If a task grows beyond a single subsystem, start with `prompts/butler-plan.md`, then switch to the narrowest relevant prompt.

## Reading order by task type

Start with:
- `AGENTS.MD`
- `docs/ai/engineering-rules.md`
- `docs/ai/repo-map.md`

Then add subsystem specs as needed:
- run execution: `docs/architecture/run-lifecycle-spec.md`
- memory: `docs/architecture/memory-model.md`
- transport: `docs/architecture/model-transport-contract.md`
- tools and runtimes: `docs/architecture/tool-runtime-adr.md`
- credentials: `docs/architecture/credential-management.md`
- config: `docs/architecture/config-layering.md`
## Remote lab verification

Use `scripts/dev/lab.ps1` with the configured SSH alias `laba` to synchronize a
source snapshot, run Go/Chromium checks and deploy Compose on `/srv/storage/butler`.
`Integration` uses a separate test database; `Migrations` verifies task and summary
upgrade/rollback on exclusive disposable databases. `Race` uses the Debian Go
image with its installed C compiler and runs as the workspace user; it does not
install compiler packages on each invocation. Go caches remain on the lab storage
disk. `Race` runs the race detector;
`Web` runs lint, functional fixtures and a live UI smoke. Keep provider sign-in
interactive and private; see `deploy/antigravity/README.md` for the opt-in adapter.
