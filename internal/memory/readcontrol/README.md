# Broker memory reads

`memory.search`, `memory.open` and `memory.neighbors` are fixed owned revision1
contracts. They are registered programmatically by Broker only when the operator
enables `BUTLER_MEMORY_READ_ENABLED` in both services. The private Broker URL must
end in `/internal/memory-read` and use the existing action service token. The
optional `deploy/docker-compose.memory-read.yml` supplies the fixed private
`http://orchestrator:8080` route; both gates still default false. Other remote
hosts require HTTPS, while local development may use a loopback IP. Requests
never use an egress proxy or follow redirects. Database
settings cannot enable this topology. No credentials are supported in arguments.

Search uses the existing PostgreSQL source/fragment catalog, with same-owner
current session/user/task scopes and exact current source grants. Version1 uses
`simple` full-text lookup (`strategy=catalog_exact`), independent of vector index
availability. It has no automatic cloud query, embedding call or external fetch.
Original source scanning is limited to the first65,536 Unicode characters; current bounded fragments remain searchable. Large originals set truncated=true even with an empty result, so an incomplete scan cannot claim evidence absence. Semantic/adaptive ranking and scale qualification remain separate milestones.

Search accepts a query up to1024 bytes, limit1–10 and page0–9. Results contain
compact snippets, DATA trust, source kind/scope type/event time, exact source ID,
revision/access/hash, stable fragment key and safe locator. A same-key pending
candidate is an unresolved-conflict signal; it does not supersede accepted memory.
`current_revision_unverified_fact` describes freshness, not truth or authority.

Open requires all four returned source fields and optionally the fragment key;
it reads original source text or that exact current catalog fragment. Offset and
max_chars count Unicode characters. At most8192 characters are returned. Neighbors
requires a fragment key and reads at most the two adjacent ordinals within the
same source and revision; its combined text is at most8192 characters. A result
marks truncation and next offset. Recognized credential spans are masked on the
complete source/fragment before pagination, with `redacted=true`. One asterisk
replaces each original Unicode character, so page boundaries cannot reveal token
suffixes and offsets still refer to original text. Locators never become
paths, SQL or URL fetches. Whole responses are bounded to128KiB.

Fragment titles are limited to2048 bytes at publication. Credential-bearing
fragment keys are rejected rather than replaced with unusable masked handles.
Older unsafe keys are suppressed from search/neighbors with `truncated=true`;
open/neighbor requests carrying such keys are denied. The redaction flag covers
recognized spans in either title or text.

The owning boundary reconstructs the actual dispatched action, input claim,
Session lease/fence and typed task from PostgreSQL. Each call uses the ordinary
Broker grant, action journal and shared budget reservation. Catalog authorization
pins complete source ancestors and current deletion watermark. Dispatch/source
proof is repeated after lock waits and before committing a read. Source IDs are
references, never lasting read capabilities. Ordinary provider release guards
continue checking those pins, including local-only content and corrected/deleted
sources. Prepared contexts and recovered tool results do not create new access.

`ok`, `empty`, `not_found` (missing fragment), `denied`, `stale` and `backend_error`
are distinct data statuses. Unknown/inaccessible source IDs are denied, so an
unauthorized caller cannot enumerate them. Infrastructure/RPC failure is an
ordinary tool failure. No HTTP200 or completed tool result claims fact acceptance.

Run `bash scripts/dev/memory-read-lab.sh` on laba after copying the current source
snapshot to the task's isolated `current` directory. It uses existing images and
cached modules offline, disposable PostgreSQL, actual Broker executable and
mandatory no-skip gates. Historical native windows are not evidence for this code.
