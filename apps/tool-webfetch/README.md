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
