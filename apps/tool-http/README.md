# Tool HTTP

HTTP runtime service for Butler web and API tools.

Current state:
- runtime exposes the internal `ToolRuntimeService` gRPC contract
- supports `http.request`, `http.download`, and `http.parse_html`
- `http.request` executes with a domain allowlist enforced from the tool contract
- `http.request` can inject resolved credential refs into outbound auth headers without exposing secrets in model-visible args
- `http.download` enforces configurable size limits and `http.parse_html` provides selector-based extraction with bounded input size
- Docker Compose deployment must attach the runtime to an egress-capable network in addition to the internal tool network so outbound requests can resolve public DNS
- HTTP health endpoint and unit tests are in place

Local run:
- `go run ./apps/tool-http`
- service exposes HTTP health on `BUTLER_HTTP_ADDR` and runtime gRPC on `BUTLER_GRPC_ADDR`

Intended responsibilities:
- execute HTTP-class tools only
- stay behind Tool Broker routing and policy checks
- support credential-aware execution through broker-mediated resolution only

Related docs:
- `docs/architecture/tool-runtime-adr.md`
- `docs/architecture/credential-management.md`
- `docs/planning/butler-implementation-roadmap.md`

An explicitly scoped `external_receipt_v1` contract can receive a Broker-owned
idempotency key and fingerprint through private execution context. The runtime
injects fixed headers, rejects caller overrides and does not follow redirects
for these mutations. It performs no implicit retry. Generic HTTP success does
not verify an external write; Broker owns receipt validation and reconciliation.
See [the effect contract](../../docs/architecture/external-effect-journal.md).

Public network execution pins checked DNS, rejects internal/ambiguous destinations, denies redirects and caller routing/auth headers, and bounds the serialized request including mediated headers to 4 MiB before sending. Broker requires exact release approval or current scoped owner purpose even for arbitrary GET. Reflected resolved secrets and cookie values are masked; response headers and network errors are bounded safe data. See [security boundaries](../../docs/architecture/agent-security-boundaries.md).
