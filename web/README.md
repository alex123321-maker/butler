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
├── shared/                    # API client, Pinia stores, design tokens, and reusable UI primitives
├── tests/e2e/                 # Playwright specs and snapshots
└── widgets/                   # Route-level widgets such as the sidebar
```

Key routes:
- `/` - task-centric overview dashboard
- `/tasks` and `/tasks/[id]` - normalized task list and detail views
- `/approvals`, `/artifacts`, `/activity`, `/system` - operator visibility pages
- `/sessions`, `/runs/[id]`, `/memory`, `/doctor`, `/settings` - legacy and operator workflows
