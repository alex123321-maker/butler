# Butler Web

The credential vault bookmark is fixed at the right-center edge after owner
connection. It opens a six-cell PIN setup/unlock and a sidebar for account CRUD,
access/create requests, protected manual OTP and per-account SMS routing.
Unlocks/forms remain in page memory; locking clears private state. Protected
requests use a separate endpoint rather than ordinary chat/widget responses.
Credential-entry Playwright tests explicitly disable screenshots/traces/video.

Nuxt 3, Vue 3, TypeScript and Pinia. One continuous owner page contains the
calendar, observed work, conversation, recent memory and incoming decisions.
features/mind replaces the former dashboard, sidebar, layout and route pages.
There is no iframe, second application or additional runtime dependency.

## Data and interaction

- Communication history follows opaque server cursors until the requested batch
  of 50 publications is assembled. The server can release each publication's
  complete access proof in a separate HTTP transaction; small server pages do
  not truncate visible history. Invalid or non-progressing cursors fail closed.
  Explicit history expansion is retained during an in-flight background refresh;
  the queued read uses the enlarged window so a current question is reachable.
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
- An explicitly composed communication profile reads durable canonical
  publications, owner inputs and independent Web/Telegram per-part outcomes.
  Photo captions and complete typed reports use escaped text, scrollable tables,
  authenticated media reads and SHA-256-checked downloads. Reply pins a selected
  publication/binding/version; new conversations do not infer a Task from the
  selected panel. Lost ACK repeats the exact body/delivery identity.
  Authenticated fetch SSE carries cursor invalidation only; reconnect replaces
  loaded REST pages and removes revoked content/media URLs. An unknown profile
  blocks send; only an explicit legacy 404 retains /api/v1/events. Ordinary
  startup has no communication profile enabled.
- Owner access lives in page memory. Changing it aborts requests, destroys private
  panels/SSE and clears records, drafts and receipts. Only motion preference is
  persisted. Private actions continue to require current backend authorization.
  The access check is separate from catalog readiness; memory failure leaves the
  conversation, browser and settings usable.
- Memory initially shows recent active source heads of all kinds. Bounded server
  Russian text search returns previews and current source references, explicitly
  marked “По тексту”. Typing runs only lexical search. Explicit Enter/search sends
  `strategy: hybrid`: the server may add up to four semantic source previews using
  the already configured pinned local model and active indexes. Mode/diagnostics
  distinguish hybrid results, missing indexes, unavailable inference and exhausted
  allowance. Search never invents an agent run or indexes data. Nonindexed messages
  remain searchable by text. See [owner search](../docs/architecture/versioned-memory-sources.md#owner-web-hybrid-search-2026-10-23).
  New input aborts the previous request and fences late responses. Continuation
  uses the server's lexical `next_offset`, deduplicates sources and never repeats
  inference. Semantic labels/diagnostics survive continuation.
- Source correction/deletion retain revision checks. New notes derive user scope
  from the authenticated actor and retain delivery identity after an uncertain ACK.
  Previews are data, not permission grants or confirmed profile facts.
- The home memory block shows at most three records, including search results.
  “Обзор памяти” opens a viewport-sized explorer with all loaded records and
  pagination in the same interface; loading more there keeps home capped at three.
  Text search, paginated records and a draggable provenance graph share the
  selection. Desktop gives reading more width; on small screens a single tap
  opens the reader in the same dialog. Back/Escape restores the list, its scroll
  and selected-row focus. Source information, fragments and revisions expand
  inline rather than opening nested dialogs. Ctrl/Cmd+K focuses search;
  Down enters results, arrows/Home/End move focus, Enter opens a record.
  Counts describe loaded results,
  not a fabricated corpus total. The graph's “Весь список” includes loaded search
  results; additional pages remain explicit and the API offset ceiling is honored.
- Graph edges use exact `origin.parent_refs` matches between loaded sources.
  Other referenced sources can be opened from the detail card with a fresh owner
  check. No semantic relationships are inferred from titles or shared scope.
  The last five successful search terms stay in page memory until cleared or the
  owner changes. A bounded back/forward trail contains only IDs and re-inspects
  each source. Viewer-only pins were removed. Claims for the inspected revision use the
  existing versioned catalog decision endpoint, including the observed active
  claim ID for both confirmation and rejection. Failure/409 offers a fresh read;
  it never marks a decision locally or automatically retries a mutation.
  Claim catalog limits and unavailability are visible separately from source reads.
  A reversible source-use switch is not offered: the existing revoke endpoint
  purges content. “Управлять этой записью” opens the existing management panel
  with the exact source ID. Copy explicitly writes inspected text to the clipboard.
  See [workflow alternatives and rationale](../docs/design/memory-explorer-usability.md).

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
BUTLER_WEB_TEST_PORT selects an isolated Playwright preview port (default4173).

Run npm run typecheck, npm run lint and npm run test:e2e. Chromium cases use
synthetic intercepted fixtures to check layout, owner/source changes, durable
retry, browser control and retained privileged workflows. They do not certify
production model runs or external effects. Existing real API/browser cases are
opt-in through their environment requirements.

Targeted explorer validation: `npx playwright test memory-explorer.spec.ts mind.spec.ts`.
These tests cover inline provenance/history, claim confirmation/rejection and CAS
conflicts, delayed reads, recent searches, reading navigation, clipboard and exact
management links, graph dragging, keyboard access, and 320–1440px layouts.

Catalog/API integration uses disposable PostgreSQL and:
go test ./internal/memory/catalog ./apps/orchestrator/internal/api -run
'TestOwnerBrowse|TestMemoryOwnerBrowse' -count=1.
Set BUTLER_TEST_MEMORY_POSTGRES_URL to an isolated database ending in _test.
Manifest fsync checks require Linux; use the existing Go Docker image on Windows.
A skipped database test is not integration evidence.

See [browser control](../docs/architecture/browser-session-contract.md),
[source catalog](../docs/architecture/versioned-memory-sources.md) and
[garden lifecycle](features/mind/garden/README.md).

### Communication widgets

`MindWidget` renders single/multiple choice and text inside the existing feed.
The shared multi-choice draft is read from PostgreSQL-backed REST and submitted
with its own CAS version. Draft changes never submit an answer. An uncertain
request retains its exact body/delivery ID for explicit retry; stale responses
reload the winner. Terminal questions disable controls, with server enforcement
independent of disabled HTML. Unicode text limits count characters. Existing
private verified media rendering supplies optional option images. Targeted UI
checks: `npm run test:e2e -- communication-widget.spec.ts`.
