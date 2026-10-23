# Custom emoji research importer

Operator-only public catalog/file import for issue #140. Not installed by
Bootstrap, not a model tool, no getUpdates, message sending, owner conversations,
delivery or provider authentication. Uses the existing private Telegram channel
client and its explicit CONNECT proxy. This command receives private file
references, never raw token/proxy arguments. It cannot register a new capability
or bypass Broker on behalf of a Butler run.

Required flags: `-token-file`, `-proxy-file`, `-database-file`, `-out` (new absolute
private directory), `-source-commit` (40-character Git base).
Database URL must select exactly `butler_emoji140_test`; provision an independent
PostgreSQL container, no product schema migrations. PostgreSQL owns runs and
per-item attempts. JSON is a derivative export. An interrupted run remains
running with its last per-item state; it must not be reported complete. No
automatic retries, overwrite of artifact directories, or crash resume.

The default seven packs come from the issue. All public metadata is inventoried.
At most 450 unique IDs are selected in round-robin pack order, with
SHA256(custom_emoji_id) ordering inside each pack. This is a discovery pool,
**not** the stratified 60/90/300 study split. Exact hashes group identical media;
near-duplicate groups, semantic quotas and final splits still require review.
Unavailable packs and failed downloads are explicit. File ID aliases remain
metadata; credential-bearing download URLs and paths never leave the client.

Each metadata reply is capped at 1 MiB; each media file at 2 MiB. IDs, set type,
dimensions, file size and magic bytes must agree. This is signature validation,
not proof of successful decoding. TGS/WEBM decoding is separate, with bounded
offline processes. Duration remains null until actually decoded.

See [experiment protocol](../../../../docs/architecture/telegram-emoji-research.md)
and [isolated runner](../../../../experiments/emoji140/README.md).
