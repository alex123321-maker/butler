# Closed V7 resource lineage

`scripts/dev/memory-closed-temporal-v7-history.py prepare BASE LABEL` creates
exclusive, fsynced read-only resource evidence under `BASE/reports/LABEL/history`.
The fixed source is closed `native-v7-preseal-v3`; the frozen native source and
all its original results remain unchanged. `check-current PATH SHA256` repeats
the seven-store inventory, original-file and future90 namespace checks. These
commands read no credentials and make no provider calls.

`loadClosedTemporalV7History(path, sha256)` in the app integration-test package
validates format `butler.memory.closed-owner-leaf.v1`. Its typed result exposes
`Hash`, `Day`, `Deadline`, `Carry`, inclusive `Usage`, independent `NewUsage`,
`SeparateAdaptive`, recursively validated `Previous`/`Historical`, and
`SourceHashes`. A later lab cohort may use this receipt as a predecessor; this
helper does not create production cross-database admission or a budget import.

The loader re-proves the prior109 lineage and the new V7 ledger against all12
original outcomes, every actual immutable native entry and the original exact
handback. It checks one labelled prior carry, current owner-day records across
all seven stores, and unchanged other owners and schemas. Future90 uses exactly
`issue86-20261016-pg/memory_read_test`, schema `experience_native_v1`, domain
receipt `public.issue90_native_windows`, human window `experience-explicit-v1`.
The consuming collector must still repeat fresh checks, atomically claim its
namespace/receipt and import the validated finite carry once before admission.

Product admissions and native entries have independent counts. A failed run can
retain charged product rows with NULL observations and no native entry; zero
native calls do not refund those quotes. Native UNKNOWN usage remains unknown.
All twelve outcomes are required even when failed or unexecuted. Existing older
V5/V6 historical recipes retain their positive-native restriction. Native
entries require a preceding unique Start per run and at most12 per original.

Original historical161, earlier failures and NULLs, distinct adaptive226, the
zero-auth V7 setup failure and the separate operator control-plane receipt remain
in the recursive evidence. The inclusive scopes are never summed as separate
new charges. `paid_discovery` in handback is credential project discovery, not a
model generation; unobserved invoice cost is not fabricated as zero model usage.

This accounting receipt does not confer semantic qualification. Independent
grading of the original V7 answers is retained separately.
