# Butler Web

Nuxt 3, Vue 3, TypeScript and Pinia. One continuous owner page contains the
calendar, observed work, conversation, recent memory and incoming decisions.
features/mind replaces the former dashboard, sidebar, layout and route pages.
There is no iframe, second application or additional runtime dependency.

## Data and interaction

- Calendar dates use the browser's local time zone. Tasks use observed started_at
  or updated_at; schedules use actual next_at. No demonstration tasks, invented
  future dates or simulated action timers are shipped. The approved vine geometry
  represents the selected day's items. Flowers are a visual clock, not verification.
- The short log subscribes to existing run SSE and shows three recent events;
  the journal opens the full loaded history through the same stream. Each event
  type has an icon with an accessible tooltip, without a visible category label.
  Tool calls correlate by tool_call_id; saved transcript arguments, results,
  errors, duration and execution identity expand in place. A completed tool call
  is separate from task completion or effect verification. Known credential fields
  are masked in the displayed data. Result events refresh the stored transcript;
  SSE previews do not substitute for a missing durable result. Typing animates
  observed tool names, not hidden reasoning or invented model answers. Working animation
  requires a working task and connected stream. OS reduced motion is respected.
  “Тихо на этом экране” stops the incoming attention pulse; it does not change
  server notification delivery policy.
- Conversation excludes internal attempt finals. Input uses durable /api/v1/events.
  Acceptance, processing, verification, external effects and delivery stay separate.
  Uncertain ingress/task control retain their exact body/delivery ID for retry.
- Owner access lives in page memory. Changing it aborts requests, destroys private
  panels/SSE and clears records, drafts and receipts. Only motion preference is
  persisted. Private actions continue to require current backend authorization.
  The access check is separate from catalog readiness; memory failure leaves the
  conversation, browser and settings usable.
- Memory initially shows recent active source heads of all kinds. Bounded server
  Russian text search returns previews and current source references, explicitly
  marked “По тексту”. This owner endpoint does not admit embedding inference,
  fabricate a run lease or bypass budget policy. Semantic retrieval remains on
  the existing admitted agent path; direct owner semantic inference is not connected.
- Source correction/deletion retain revision checks. New notes derive user scope
  from the authenticated actor and retain delivery identity after an uncertain ACK.
  Previews are data, not permission grants or confirmed profile facts.

## Panels and browser

The top browser icon opens the server Chromium viewer. Live sessions support
viewing, exclusive manual control, navigation, click, scroll, keyboard and private
text input. Return to the agent is explicit. Closing the panel clears frames,
controller and input without silently releasing control. Current controller,
epoch and frame checks remain. Input is never automatically replayed. The existing
optional server-browser composition is required; this UI cannot enable a missing
service. Guarded binding of a user's browser tab stays in its approval request.

Settings open provider/prompt, goal/mandate, schedule, skill, extension/converter,
device, voice, diagnostics and artifact workflows as panels, retaining their
proof, revision, recovery and uncertain-effect logic. “Хранилище и политики памяти”
retains detailed source history, upload, claim decisions, export and index controls.

Old bookmarks select new panels: /tasks[/id], /runs/id, /sessions[/key], /inputs/id,
/memory?source_id=id, /memory/manage, /approvals, /browser, /settings and the other
administrative module names. Private panels wait for explicit owner connection.
Recorded attempts show public conversation and expandable stored tool calls.

## Development and validation

From web/: npm ci, npm run dev; or npm run build and npm run preview.
BUTLER_API_BASE_URL sets the endpoint (default http://localhost:8080). Existing
Docker/Nitro runtime configuration remains. Keep caches under repository .cache/.

Run npm run typecheck, npm run lint and npm run test:e2e. Chromium cases use
synthetic intercepted fixtures to check layout, owner/source changes, durable
retry, browser control and retained privileged workflows. They do not certify
production model runs or external effects. Existing real API/browser cases are
opt-in through their environment requirements.

Catalog/API integration uses disposable PostgreSQL and:
go test ./internal/memory/catalog ./apps/orchestrator/internal/api -run
'TestOwnerBrowse|TestMemoryOwnerBrowse' -count=1.
Set BUTLER_TEST_MEMORY_POSTGRES_URL to an isolated database ending in _test.
Manifest fsync checks require Linux; use the existing Go Docker image on Windows.
A skipped database test is not integration evidence.

See [browser control](../docs/architecture/browser-session-contract.md),
[source catalog](../docs/architecture/versioned-memory-sources.md) and
[garden lifecycle](features/mind/garden/README.md).
