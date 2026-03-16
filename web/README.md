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
- **Styling:** Plain CSS with CSS custom properties (no framework)
- **API client:** `useApiClient()` composable wrapping `$fetch` with runtime config
- **Health check:** `useHealthCheck()` composable polls `GET /health` on the orchestrator
- **Layout:** Fixed sidebar with navigation, main content area
- **State:** Nuxt built-in composables (`useAsyncData`, `useState`) — no external store

## File structure

```
web/
├── app.vue                    # Root component
├── nuxt.config.ts             # Nuxt configuration
├── assets/css/main.css        # Global styles
├── components/
│   └── SystemStatus.vue       # Health indicator component
├── composables/
│   └── useApi.ts              # API client and health check
├── layouts/
│   └── default.vue            # Sidebar + main layout
└── pages/
    ├── index.vue              # Dashboard
    ├── sessions.vue           # Sessions list
    ├── sessions/[key].vue     # Session detail with runs
    ├── runs/[id].vue          # Run detail with transcript
    ├── memory.vue             # Memory browser
    ├── doctor.vue             # Doctor reports and system checks
    └── settings.vue           # Settings management
```
