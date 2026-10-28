# Static review: proposed research-only Linux spool

Decision: feasible as a new prospective storage variable, conditional on the
publication and failed-path rules below. This is not a source freeze, repair
verification, experiment preparation, native admission or claim that the original
802 ms failure has a uniquely proven root cause. No Go, PG, auth, model calls,
fault tests or parent-source edits were performed for this review.

The unchanged reader can consume copied segments: it validates ordinary relative
filenames, compressed and uncompressed hashes/lengths, contiguous sequence/span
pairs, identity, limits, zero open/dropped/observer-failed state and final seal.
It does not require callbacks to have originally written to the publication
filesystem. Keep the same gzip bytes; copy them without re-encoding events.

1. **Separate writer closure from successful evidence publication.** Under the
   observer mutex, freeze the final segment descriptors/counts and reject every
   later callback as GLOBAL. After actual owning workers/processes/claims/leases
   are quiescent, finish the partial chunk in the spool, copy each exact segment
   with CreateNew, close/Sync/read-back and verify its hash/length. Publish the
   final header only after all referenced files are verified, then create the
   fixture inventory. Recheck shared GLOBAL/observer state after publication and
   before returning or admitting another arm. A successfully closed spool is not
   itself a successfully published arm. Current sealStream sets its in-memory
   sealed flag before sealFixtureEvidence writes the header; simply changing
   directory plus copying after that existing header write would be wrong.

2. **Preserve failed-path evidence without inventing quiescence.** Current r2
   retained an unsealed 8,214-event trace despite failed quiescence. A rule that
   permits all copying only after *successful* quiescence must explicitly retain
   the original failed spool, its exact immutable segment/header/tail inventory
   and identity for private forensics. Never emit a qualified sealed header that
   references absent publication files. Missing/failed publication stays GLOBAL
   and unavailable. Do not discard the partial tail, clear faults, reconstruct
   callbacks or rerun the failed slot. If the process cannot establish writer
   closure, retain the original failure and mark the forensic snapshot unsettled.

3. **Own and bind the spool.** Select its base in trusted test code, create a
   unique 0700 directory, pin it to the exact window/slot/arm/schema/producer/
   protocol identity, and use 0600 exclusive segment files. Do not use
   testing.T.TempDir or an automatic cleanup that deletes failed evidence.
   Refuse reuse, symlinks/reparse substitutes and fallback to the Windows output
   directory. Verify the actual filesystem/device used by /tmp in this declared
   chroot; a Linux-looking pathname alone does not establish ext4 storage.

4. **Copy is a separate durable operation.** /tmp and /src20 are different
   filesystems: rename is not an atomic publication mechanism. For each bounded
   existing descriptor, stream-copy with exact byte count, Sync/Close and a fresh
   destination read-back hash. Handle short writes, read/copy/Sync/Close/hash
   errors and destination collisions as GLOBAL; retain the original spool and
   partial output, never overwrite or silently resume publication. Use the
   existing segment descriptors as an allowlist, not a recursive arbitrary tree
   copy. Keep marker/header handling explicit and reader-relative paths unchanged.

5. **Keep failure priority and retention bounded.** Original context/deadline,
   750 ms authority checks, 9+1 pools, 4096/16384/512 MiB bounds, operation errors,
   late-callback faults and final arm/window closeout remain unchanged. Existing
   bounded evidence-cleanup contexts may perform publication; they grant no new
   execution time or authority. Retained spools share the finite 4 GiB research
   disk: 48 times a 512 MiB per-arm maximum does not fit. Declare retention/free
   space in the resource profile and fail GLOBAL on shortage, with no recursive
   deletion or quota/buffer/cap increase. Any later explicit cleanup of verified
   successful spool files would need a separately stated ownership/retention rule.

6. **This does not eliminate all observer delay.** gzip/JSON and Sync remain
   synchronous in the pgx callback under observer.mu. pgx baseRows.Close invokes
   TraceQueryEnd before pgxpool poolRows.Close releases the connection, so even
   ext4 spool work can hold the sole authority connection. The actual r2 gap at
   event 8192 and concurrent deadline support this storage hypothesis but do not
   isolate compression, mutex contention, filesystem latency, scheduling or a
   separate database cause. Do not advertise an established repair from pathname
   replacement alone or mask another timeout if one recurs.

Existing required verification remains sufficient in scope: adapt the public
stream mechanism/collision/late-callback/last-arm failure checks to this storage
seam; establish actual spool filesystem and byte-identical reader-visible output
after normal Core/Broker quiescence; run the existing Public, setup/composition,
resource and vet gates on the final frozen source. These are representation and
lifecycle checks, not quality passes or a demand for another paid reproduction.
Any new actual study still needs separate source/scorer freeze, substantive fresh
controls and its original one-use admission. Closed r2 stays immutable NO_GO;
history remains 11 closed / 774 admissions / 762 measured / 5,948,714 known tokens /
12 NULL. No new corpus or experiment has been prepared in this review.
