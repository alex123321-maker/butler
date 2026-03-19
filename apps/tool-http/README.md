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
