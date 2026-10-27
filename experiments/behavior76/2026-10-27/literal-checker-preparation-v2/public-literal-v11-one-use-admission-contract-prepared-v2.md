# Literal v11 admission contract v2 — PREPARED_ONLY, not a certificate

Additive preparation revision of public-literal-v11-one-use-admission-contract-prepared-v1.md. The prior document/schema and their published hashes remain unchanged. No formal freeze, hidden authorship, actual admission or execution exists here.

All v1 lifecycle,16/32/68/8/6 plan, original bounds, closed12/971/959/7700211/12 history, typed UTC/day-fit, exact unique prepared ELF/no rebuild/no overwrite, one-use CreateNew+durability/readback, no retry and unchanged stop-priority requirements remain.

The new schema literal-v11-one-use-admission-v2.schema.json additionally REQUIRES both artifacts.launcher_core and artifacts.admission_schema, each with exact fixed path/SHA256 (and optional recorded bytes). The launcher and host_wrapper artifacts remain separately mandatory. A clean source tree/source-inventory hash does not replace these direct byte checks at validation and immediately before the one allowed executable entry.

The launcher must constrain all four artifacts to the frozen code-owned paths before evaluating their bytes. Certificate data cannot select an alternative core/schema/host script. Reject duplicate/case-aliased/unknown fields before lossy parsing; compare decoded date strings with typed UTC DateTimeOffset. ValidateOnly remains nonconsuming and must not build/launch/read auth or alter an existing ELF/output/marker. Actual Native additionally needs explicit operator switches and stopped owned heavy/native lane. It consumes the irreversible exact marker before WSL/ELF and still depends on actual Go preentry guards.

There is no hash cycle: static launcher/core/schema/host bytes are pinned in the future certificate, while the future certificate hash is supplied/verified separately by the operator/issuer. None of those scripts rewrites itself to bake that final certificate hash. The prepared schema's own hash is not embedded in its content.

The future certificate representation still uses version issue76-literal-v11-one-use-admission-v1 because no v11 certificate has been issued or frozen; v2 here versions the PREPARED schema/contract revision. The schema id distinguishes this stricter preparation. If the producer changes the actual certificate version before freeze, all consumers must change prospectively and be repinned together.

This document does not claim review or success of the parent's new launcher implementation; actual public tests/source review and final source/scorer freeze remain separate. No hidden case, actual approval JSON, model, PG, Go or Docker operation is produced by this revision.
