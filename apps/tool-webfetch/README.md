# Tool WebFetch

WebFetch runtime service for Butler retrieval-style web tools.

Current state:
- runtime exposes the internal `ToolRuntimeService` gRPC contract
- supports `web.fetch`, `web.fetch_batch`, and `web.extract`
- executes through a provider chain:
  - self-hosted primary
  - Jina reader fallback
  - plain HTTP fallback
- keeps provider selection inside the runtime so Tool Broker can route one stable tool family
- does not expose credential refs in public tool contracts

Local run:
- `go run ./apps/tool-webfetch`

Intended responsibilities:
- fetch and normalize web page content for downstream reasoning
- prefer self-hosted retrieval when configured
- fall back safely when the preferred provider is unavailable

Related docs:
- `docs/architecture/tool-runtime-adr.md`
- `docs/planning/butler_single_tab_browser_webfetch_backlog.md`

Requested resources pass public-IP/DNS checks before delegation; checked clients deny redirects and share a private 4 MiB counter across batch/fallback requests. The fixed operator self-hosted service is a network exception and must enforce equivalent DNS policy itself. Private query/path disclosure needs Broker release authority; public DNS alone grants none. Reflected Jina tokens and unsafe error bodies are excluded. See [security boundaries](../../docs/architecture/agent-security-boundaries.md).
