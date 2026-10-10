# Structured source ingestion

Owner-proved `/api/v4/memory/ingest` selects one uploaded file. The request carries
a stable logical external key, expected source revision, scope, author/event time,
path/repository revision and original bytes. It cannot fetch a URL, scan a disk,
select executables or grant source/goal authority. Bytes are at most 8 MiB; parsing
uses a 10-second acceptance deadline, at most 2,000 fragments and 16 MiB normalized
text. Cancellation is checked in parser loops and before/after the standard Go
AST call; that synchronous library call is not forcibly preempted. Input remains
bounded at 8 MiB. Native PDF/OCR processes additionally have a hard timeout and
container resource bounds.

The Go standard library is sufficient for Go AST functions/methods/types/values,
Markdown headings/fenced code and table rows, CSV/TSV headers+rows, JSON messages
with author/time/thread, and DOCX ZIP/XML headings/paragraphs/table rows. Long
functions retain signature and bounded comment context in each fragment title.
UTF-8 boundaries are preserved. Other code languages use explicit text/Markdown
mode rather than claiming unsupported AST structure. DOCX page locators count
explicit page breaks; they are not renderer-derived pagination. No code/macros,
document instructions, embedded links or external XML entities are executed.

PDF uses the separate fixed Poppler/Tesseract adapter. OCR is attempted only
after successful empty text extraction, and remains uncertain evidence. PDF
pages/layout are retained; automatic table semantics for visually ambiguous PDF
grids are not invented. Structured DOCX/CSV/Markdown tables repeat their headers
with each logical row, keeping related cells together.

Migration 049 stores original assets/revisions/parser/method/status in PostgreSQL.
Successful parsing first stores `parsed`; publication moves to `ready`. A crash
can therefore leave a repairable `parsed` asset, never a false completed index.
An exact retry repairs stages without duplicate source/chunk IDs. Failed/corrupt
files retain a generic diagnostic and original for owner inspection, but publish
no fragments. Changed entities update only their stable fragment keys; a rename
refreshes citation metadata and leaves text hashes reusable for new generations.

Current owner source inspection exposes revisions, bounded fragment locators,
adjacent ordinal neighbours and privately downloadable original bytes. NDJSON
export includes all revisions and base64 original assets. Current source access
is rechecked before old asset versions can be downloaded. Delete/revoke physically
purges every binary/text version and derived indexes, with content-free hashes,
IDs and tombstones retained. This is a source store, not a vector truth store.
