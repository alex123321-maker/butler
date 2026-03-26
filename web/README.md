# Butler Web UI

Nuxt.js frontend for the Butler self-hosted AI agent platform.

## Overview

Provides a browser-based interface for:
- **Dashboard** — system health overview
- **Sessions** — session list and run history (T-0604)
- **Run detail** — transcript timeline for a selected run
- **Memory** — scope-based browser for durable working, profile, episodic, and chunk memory with provenance links
- **Doctor** — system diagnostic reports (T-0605)
- **Settings** — grouped runtime configuration management with source tracing plus provider auth flows for GitHub Copilot and OpenAI Codex
- **Browser extension onboarding** — rollout and remote-extension readiness checklist in Settings + System pages
  - System page also shows per-`browser_instance_id` remote instance state (`online`, `stale`, `disconnected`, `unknown`)
  - System page now polls lightweight orchestrator endpoint `/api/v2/single-tab/extension-instances` for near real-time extension liveness updates and supports state/limit filters
- **Prompt management** — operator base prompt editing, placeholder insertion, and effective prompt preview inside Settings

## Development

```bash
cd web
npm install
npm run dev
```

The dev server starts on `http://localhost:3000`.

### Environment variables

| Variable | Default | Description |
|---|---|---|
| `BUTLER_API_BASE_URL` | `http://localhost:8080` | Orchestrator REST API base URL |

## Production build

```bash
npm run build
node .output/server/index.mjs
```

## Docker

Build and run via Docker Compose from the repo root:

```bash
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.dev.yml up -d --build
```

The web service is exposed on port 3000 in the Compose dev stack.
The dev overlay also publishes the orchestrator API on `http://localhost:8080`, which the browser UI uses by default.

## Architecture

- **Framework:** Nuxt 3 with Vue 3 and TypeScript
- **Styling:** Tailwind utilities plus project design tokens from `app/styles/tokens.css` and `shared/design/tokens`
- **API client:** shared `$fetch` wrapper in `shared/api/client.ts`
- **State:** Pinia stores in `shared/model/stores`
- **Layout:** Nuxt layouts plus shared UI primitives and sidebar widget composition
- **Testing:** Playwright end-to-end coverage in `tests/e2e`

## File structure

```
web/
├── app/                       # App-level tokens and styles
├── assets/                    # Global CSS assets
├── components/                # Legacy shared components still used by route pages
├── composables/               # Nuxt composables used across route pages
├── entities/                  # API-facing entity modules such as tasks, approvals, memory, system
├── features/                  # Reserved for task-level UI workflows and composed behaviors
├── layouts/                   # Nuxt layouts
├── pages/                     # Route entrypoints (`tasks`, `approvals`, `artifacts`, `system`, etc.)
├── extensions/                # Standalone browser-extension assets for Chromium-based browser integration
├── shared/                    # API client, Pinia stores, design tokens, and reusable UI primitives
├── tests/e2e/                 # Playwright specs and snapshots
└── widgets/                   # Route-level widgets such as the sidebar
```

Key routes:
- `/` - task-centric overview dashboard
- `/tasks` and `/tasks/[id]` - normalized task list and detail views
- `/approvals`, `/artifacts`, `/activity`, `/system` - operator visibility pages
- `/sessions`, `/runs/[id]`, `/memory`, `/doctor`, `/settings` - legacy and operator workflows

## Chromium Extension Scaffold

`web/extensions/chromium-butler` contains a standalone Manifest V3 scaffold for the single-tab browser bridge flow.

Current scope:
- lists HTTP(S) tabs from the local Chromium browser
- keeps a long-lived remote relay connection to Butler API by default, with optional native-host fallback
- answers orchestrator bind-discovery requests and returns current tab candidates for `BROWSER_TAB_SELECTION`
- checks the active `single_tab_session` for a Butler session key
- executes host-routed single-tab actions in the bound tab through a persistent native messaging port
- supports rollout-aware connection strategy (`native_only`, `dual`, `remote_preferred`) for localhost and remote-hosted Butler deployments

Current limitations:
- native host installation is still manual and browser-specific when `native` mode is used

Manual load for development:

1. Open Chromium/Chrome/Edge extension settings and enable developer mode.
2. Choose "Load unpacked" and point it at `web/extensions/chromium-butler`.
3. Configure orchestrator `BUTLER_EXTENSION_API_TOKENS` and keep extension popup in `remote_preferred` mode.
4. Fill `Remote Butler URL` + `Remote API token` in popup.
5. Click `Connect relay` in popup.
6. Trigger `single_tab.bind` from Butler agent and approve tab selection in Web UI/Telegram.
7. Optional native fallback: install `com.butler.browser_bridge` manifest and run `apps/browser-bridge`.

Operator shortcut:
- in Butler Web UI open `/settings`, use **Download extension (.zip)** in the Browser Extension Remote panel, unpack it, then use "Load unpacked" in Chromium/Edge.
