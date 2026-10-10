# Deploy

Docker Compose files and environment templates live here.

- `docker-compose.yml` defines the internal Butler stack used as the base for all local environments.
- `docker-compose.dev.yml` adds host-exposed ports and persistent dev volumes required for browser and direct local access.
- Copy `../.env.example` to `../.env` before running `make up` or `make infra-up`.
- Compose no longer injects default values for Butler config fields; if an env var is absent, the service falls back to Butler's own config resolution (env -> DB override -> code default).

Current Compose stack includes:
- `postgres` using a pgvector-enabled image
- `redis` for transient state and queues
- `migrator` for schema setup
- `orchestrator` for ingress, run execution, memory context loading, and transport
- `restart-helper` for narrow helper-managed Docker Compose restarts of allowlisted Butler services
- `tool-broker` for tool validation, credential policy checks, secret resolution, and runtime routing
- `tool-http`, `tool-webfetch`, `tool-browser`, `tool-browser-local`, and `tool-doctor` as isolated runtime services
- `web` for the Nuxt operator UI

Current behavior notes:
- runtime isolation is network-based inside Compose through the internal `tool-runtime` network
- `browser-bridge` stays host-side for native messaging mode; `tool-browser-local` is now part of Compose for `orchestrator_relay` single-tab flows
- `restart-helper` is intentionally attached only to the internal backend network and uses an explicit allowlist plus self-exclusion before touching the Docker socket
- `tool-doctor` does not get the Docker socket and limits `doctor.check_container` to probing configured service health URLs
- for local browser usage, start Compose with both files or use `make up`; the base file alone does not publish the orchestrator REST API to the host
- the stack wires fixed internal Compose URLs for PostgreSQL, Redis, Tool Broker, and web-to-orchestrator connectivity; optional Butler settings are passed through only when they exist in the host environment
- optional variables such as `BUTLER_OPENAI_API_KEY` and Telegram credentials are passed through only when they exist in the host environment, so missing values do not appear as empty env overrides inside containers
- Antigravity selection uses `BUTLER_MODEL_PROVIDER=antigravity` and forwards `BUTLER_ANTIGRAVITY_MODEL` to the orchestrator. OAuth credentials stay in the encrypted provider-auth setting; they are not Compose model arguments.

## Separate SSH lab

`docker-compose.lab.yml` overlays the base stack for a dedicated machine. It
uses Compose project `butler-lab`, publishes the UI on port 3080 and API on
8088, keeps PostgreSQL/Redis/runtime ports private, and binds durable data to
`BUTLER_LAB_ROOT/data` on the lab's storage disk. Compose 2.24.4 or newer is
required. The local-browser relay addresses the container orchestrator.

From Windows, after creating a writable `/srv/storage/butler` on `ssh laba`:

```powershell
pwsh -NoProfile -File scripts/dev/lab.ps1 -Action Sync
pwsh -NoProfile -File scripts/dev/lab.ps1 -Action Test
pwsh -NoProfile -File scripts/dev/lab.ps1 -Action Up
pwsh -NoProfile -File scripts/dev/lab.ps1 -Action Integration -Packages ./...
pwsh -NoProfile -File scripts/dev/lab.ps1 -Action Race
pwsh -NoProfile -File scripts/dev/lab.ps1 -Action Web
pwsh -NoProfile -File scripts/dev/lab.ps1 -Action Status
```

Source snapshots and dependency caches are separate from durable data. Sync
uploads tracked and nonignored source files into a fresh snapshot, so removed
files cannot survive from an earlier build. It preserves previous snapshots.
Deployment generates settings encryption and extension access keys on the lab
in a mode-0600 `lab.env`, without printing them. Edit that file on the lab to
configure a model provider or channel credentials; source uploads do not touch
it. Memory extraction is disabled until a provider is configured. Ollama has a
6 GiB container limit; this is a deployment limit, not a model scheduling policy.
Go checks and deployment logs are retained in `BUTLER_LAB_ROOT/reports`.
Go builders and lab checks pin the patched Go 1.27.2 release. The module minimum
matches that version; validation must not fall back to an unsupported Go 1.24
compiler. Version changes keep dependency caches on the lab storage disk.
Compiler temporary files also use the storage disk. Lab deployment provisions a
dedicated Docker Buildx container driver with its state volume bound to
`BUTLER_LAB_ROOT/.cache/buildkit`, so BuildKit state stays on the storage disk.
Exported service images still use the Docker daemon's configured image storage.
The builder is selected only for that Compose command; other projects keep their
Docker builder. Existing cache volumes with a different path are refused.
Integration and race checks use `butler_test`, `butler_lab_actionauth_test` and
`butler_lab_completion_test`, plus Redis database 1. The completion environment
is explicitly supplied so its PostgreSQL fault fixtures cannot silently skip.
Integration, race, migration and browser approval suites take a common lab lock
before touching shared test fixtures, so concurrent invocations run in sequence.
Destructive migration fixtures use additional dedicated databases; deployed
application data is excluded.
Web checks run Vue/TypeScript checks, lint, functional Playwright fixtures, and a read-only browser
smoke against the deployed services. Screenshot baselines have no committed
references, so the visual-baseline test is excluded; the live smoke saves a
current UI screenshot in `.cache/lab-ui.png` on the lab.
Browser approval proofs verify credential isolation, one actual submission and
replay refusal. A DOM acknowledgement without a recipient receipt remains
`effect_unknown` in the durable journal and cannot prove task completion.
Race checks use the C toolchain already included in a disposable Go Debian
container and run tests as UID 1000, keeping build/module caches on the storage disk.
- browser, HTTP, webfetch, and doctor runtimes are runnable V1 services rather than skeleton placeholders
- for Windows local-browser testing, install the native messaging host with `pwsh -NoProfile -File scripts/dev/install-browser-bridge-host.ps1 -Browser Chrome -ExtensionId <extension_id>`

Network tool images use read-only filesystems, dropped capabilities, no-new-privileges and bounded PIDs/memory. HTTP/webfetch/Chromium join tool-runtime plus tool-egress, without the core backend. Doctor uses fixed read-only core diagnostics and no longer receives PG/Redis/model credentials. Operator restart-helper and separately approved user-tab relay are explicit exceptions. Apply the Compose changes before claiming runtime isolation; code tests do not alter a running deployment. See [security boundaries](../docs/architecture/agent-security-boundaries.md).
