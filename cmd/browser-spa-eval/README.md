# Independent SPA recipient

Operator-only fixture for #165. Initialize with `-init -config PRIVATE.json
-origin https://OWNED_HOST`; serve with `-config PRIVATE.json -journal NEW.jsonl
-listen 127.0.0.1:18198`. Files are exclusive creations, mode0600. Back up the
configuration privately before testing; it is the independent expected-value
oracle. Only give the model the URL and requested category, never title/marker.

Reverse-proxy only `/spa-VARIANT/` on the owned HTTPS site to this loopback
listener, preserving WebSocket upgrade and immediate streaming. No DNS/private
IP exemption, runtime-specific headers or credential bypass exists here. Never
replace existing RP/IdP/Google routes. No passwords or real accounts are used.

The page loads an independently checked GraphQL query, updates a category filter,
subscribes through `graphql-transport-ws`, consumes SSE/fetch streaming, registers
and updates a service worker and fetches a Cache API entry twice. The durable
recipient journal records counts/timestamps/category only, not request bodies.
Public HTML contains no private expected title, marker or stock. Fault scenarios
are configured privately and are test-only behavior of this recipient.

Use unit/race checks for `./cmd/browser-spa-eval ./internal/browserspa/eval` and
the mandatory Chromium/PG network suite. Mechanical tests do not establish native
model or channel delivery. Measure native accepted-ingress-to-delivery wall time
including deliberation and all waits; retain failed/unknown attempts separately.
