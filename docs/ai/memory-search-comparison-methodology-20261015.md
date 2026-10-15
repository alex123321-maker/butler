# Prospective search/context comparison for issues 79 and 80

This supplements the fixed semantic methodology. Eight new synthetic Russian
cases are frozen before model execution in `ru-memory-search-v1.json`, raw SHA256
`9c78b9469bb6e61c143020324fe7af8b75e9344e6dd0a2154775f370ddeb964f`.
The targeted sample diagnoses top-two loss, profile starvation, a relevant
profile beyond the first 25 keys, long multilingual profiles, current Stop,
corrections, near-identical entities and genuinely absent information. It is
neither a random user sample nor a production accuracy estimate.

The baseline uses product code from b4a2fd3 with only the same additive test
collector/suite/launcher instrumentation. Its product files remain unchanged.
The candidate uses the revised locality/context policy. Each compiled source tree
and binary has its own exact pins; result commits are not executed-code claims.
The common instrumentation selects each product's actual default ChunkLimit,
not a test-specific improvement. Baseline executes eight retrieval inputs;
candidate executes eight each of no-LTM, sufficient-source oracle and retrieval.
Total fixed schedule: 32 native inputs, one attempt each, no replacement retries.

All arms use the same Gemini model, native transport, free-answer instructions,
schema, five-minute timeout and provider limits. Expected facts, profile fixture
bindings and review labels never enter model input. Profile bodies are ordinary
source text. Before native auth, every encoded projection is checked against a
24KiB data bound, leaving framing within the common 32KiB request allowance.

Both variants independently ingest the identical logical source history into
their own Catalog and PostgreSQL. Declared profile fixtures use real owner
Propose/Decide, are never also indexed as document chunks, and are retained in
Profile/claims/head exports. This is accepted-profile retrieval, not automatic
extraction or permission creation by a model. Writer exits, PostgreSQL restarts,
and a new reader captures current evidence before native inference. Profile
acceptance operations and retained bytes are recorded separately from embedding,
indexing, read and generation. Kernel/physical overhead remains unknown.

Every source is synthetic/public. The live model comparison tests cloud-allowed
answer quality. Assigned local-only access, pinned/unproven backend behavior,
external-release denial and revocation races are separate actual PostgreSQL
integration proofs; no local-only snippet is sent to Gemini to test its answer.

Independent semantic review judges grounded completeness, current facts,
uncertainty and task compliance using exact originals and actual visible context.
No exact sentence, source count or confidence enum is required. A scoped answer
need not repeat the scope name; "only green" already excludes blue. Correct
partial information remains partial when the full permitted corpus suffices.
Failure availability and costs/latency stay separate from semantic correctness.
All inputs, original answers, source/usage/version events and failures survive.

Report paired baseline/candidate results per case, the oracle/no-LTM controls,
context presence and conservative size, latency and actual measured calls/usage.
Useful context is evidence needed for the question, not merely shorter text.
No tokenizer billing or statistical improvement is inferred from byte estimates.
Prompts/corpus/selection are not tuned after observing these new answers; a
further tuning iteration receives another fresh sample. Adaptive multi-step
search, consolidation and automatic memory write remain separate roadmap work.
