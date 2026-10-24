# Tool WebFetch

`web.search` uses a fixed public DuckDuckGo Lite GET endpoint, no key or new
container. It returns up to eight bounded titles/URLs/snippets and UTC observation
time. Fetch primary sources to verify claims. Challenges/non-200/unknown markup
fail explicitly, making provider layout changes visible. Broker validates
bounds/provider destination/release before dispatch; runtime repeats decoding
and public network guards. Plain HTTP page reads send an identifying User-Agent.


WebFetch runtime service for Butler retrieval-style web tools.

Current state:
- runtime exposes the internal `ToolRuntimeService` gRPC contract
- supports `web.fetch`, `web.fetch_batch`, and `web.extract`
- executes through a provider chain:
  - self-hosted primary
  - Jina reader fallback
  - plain HTTP fallback
- keeps provider selection inside the runtime so Tool Broker can route one stable tool family
- omits unconfigured providers, including typed nil constructor results; a
  disabled primary cannot panic the process before reaching a healthy fallback
- does not expose credential refs in public tool contracts
- accepts UTF-8 text, JSON and XML MIME types (sniffs a missing MIME); PDF,
  binary/invalid UTF-8 and NUL-containing input fail with a bounded validation
  diagnostic instead of becoming readable text
- excludes script/style subtrees from HTML text extraction
- direct/Jina text bodies have a 2 MiB limit and explicit `metadata.truncated`;
  a split UTF-8 rune is removed at the limit. The fixed self-hosted JSON envelope
  exceeding that limit is refused. Serialized output above 3 MiB is refused
- `web.extract` revision 2 URL mode requires HTTP 2xx, preserves status/provider/
  final URL/MIME/truncation metadata, and distinguishes a successful empty page
  from missing input. HTTP failure diagnostics contain status, never the body

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

`web.fetch` revision 2 and fetch-batch items include `observed_at` and bounded
`links` (URL/text). Plain HTTP extracts links even without returning HTML. They
help reach primary references, but do not establish officiality or event state.
`web.search` uses a fixed public DuckDuckGo Lite reader; a challenge or unfamiliar
result page is an explicit failure, not a successful empty search. No API key is
required. Every subsequent page read keeps the existing Broker and egress checks.
