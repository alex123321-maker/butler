# Butler Web UI

Nuxt.js frontend for the Butler self-hosted AI agent platform.

## Overview

Provides a browser-based interface for:
- **Dashboard** — system health overview
- **Sessions** — session list and run history (T-0604)
- **Memory** — profile and episodic memory browser (future sprint)
- **Doctor** — system diagnostic reports (T-0605)
- **Settings** — configuration management (future sprint)

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
docker compose -f deploy/docker-compose.yml up web
```

The web service is exposed on port 3000 by default (configurable via `BUTLER_WEB_PORT`).

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
    ├── sessions.vue           # Sessions (placeholder)
    ├── memory.vue             # Memory (placeholder)
    ├── doctor.vue             # Doctor (placeholder)
    └── settings.vue           # Settings (placeholder)
```
