# Fixed document parser

This optional Go HTTP service owns deterministic PDF text/OCR ingestion. It is
not an agent tool runtime, accepts no paths, URLs, commands or credentials, and
does not make orchestration, source-access or model-authority decisions.
Orchestrator authenticates source selection and retains original bytes in PG.

`POST /text` and `/ocr` accept only `application/pdf`, at most 8 MiB. One request
is admitted before reading bytes; extra requests receive 429. Poppler extracts
UTF-8 text with layout. OCR independently checks the text layer and refuses to
replace any usable layer. Only an empty successful extraction invokes fixed
Tesseract `rus+eng`. OCR observations retain method/page and uncertainty.
Corrupt PDFs receive 422; limit/timeout failures receive 413. Errors contain no
document text or native stderr. No result is partial-success.

The optional `deploy/docker-compose.memory-parser.yml` profile installs Poppler
and Tesseract language assets in a dedicated image. These native packages are
justified by actual PDF/OCR requirements that Go's standard library cannot meet;
no new Go/Python dependency was added. Go AST, CSV, JSON, ZIP/XML and plain-text
parsers handle their simpler formats in the core without this service.

The image runs UID 10001, read-only rootfs, no capabilities/credentials/host ports,
an internal-only dedicated network, 512 MiB RAM/no extra swap, 1 CPU and 32 PIDs.
Only private temporary files use 128 MiB noexec tmpfs. Files are removed after each
attempt. Execution is fixed at 15 seconds, at most 16 pages, 8 MiB stdout, and
1600×1600/2.56 million pixels per rendered page. Container bounds also contain
malicious native-parser allocation. This is process/container confinement;
Poppler/Tesseract still parse untrusted binary bytes and require image updates.

Build/run with the base Compose file plus this override and profile
`--profile memory-parser`. Orchestrator connects only to
`http://memory-parser:9091`; redirects and environment HTTP proxies are disabled.
There is no outgoing network from the parser's private network.

Actual acceptance uses `BUTLER_TEST_DOCUMENT_PARSER=true` to run the real native
engine in this sandbox. The Russian text fixture has a Unicode PDF text layer;
the scanned fixture embeds `testdata/russian.png` and checks actual recognition.
The PNG was rendered from synthetic Russian text using DejaVu Sans and librsvg
for fixture preparation only. `BUTLER_TEST_DOCUMENT_PARSER_HTTP=true` separately
tests the fixed real HTTP adapter on an isolated Docker network. Neither a mock
PageAdapter nor a successful build proves OCR acceptance.
