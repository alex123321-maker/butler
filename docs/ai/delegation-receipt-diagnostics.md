# Exact parent receipt diagnostics

Fixture-only `nativeReceiptProjection(raw, children)` returns
`nativeReceiptProjectionEvidence`: a fixed `Reason` (`passed` only for an exact
match), `Bytes`, `SHA256`, JSON/object/field/array/string/distinctness validation
flags, nullable ID count, actual-children validity and unordered exact ID match.
No raw output, key, task ID or decoder error enters the evidence. GetByRun
availability, unverified verdict and privacy state remain separate caller gates.
Matching admissions grants no child-result, task-completion or owner authority.

The classifier accepts normal JSON whitespace, Unicode escapes representing
the canonical ASCII field/IDs and either ordering of the two exact actual IDs.
It requires one JSON object with exactly `admitted_task_ids`, a two-string array,
distinct nonempty IDs and two distinct nonempty independently supplied children.
Unknown fields, wrappers, duplicate fields (including escaped/case/Unicode-fold
aliases), malformed/trailing JSON, narrative and Markdown fences are denied.
It never strips fences, rewrites a body, trims/case-normalizes IDs or substitutes
actual IDs for model output. Input is bounded64 KiB, and object field examination
is bounded32 fields. Later false flags represent unperformed checks after an
earlier failure; a null ID count remains unknown.

## Historical evidence boundary

The sealed High release `7431af70a7f9f8699be86a2a73c053c2be56a3c5` report is at
`/srv/storage/butler/agents/delegation-high-20261012/reports/delegation-high-execute-20261011T195753Z-2543435`.
Read-only inspection used only safe scenario/native-events contents, runtime
filenames/permissions and ledger schema/table metadata. Native-events is229 bytes
and contains only the combined parent final/receipt gate failure. Runtime
directory metadata contains only0600 `scenario.json`; no raw final/receipt file
is present there. Ledger is1002 bytes and has no CREATE SCHEMA/TABLE or COPY data
table header in its metadata projection. No raw dump rows,
auth/key/import/profile/env contents or WAL were inspected or exported.

The safe before-cleanup audit records the parent's final/unverified metadata
and content hash `3e7d6af84cac0f47bc38a2fa6386db0706a7894acb2f04ab30fda05c5c1c0db7`,
matched through parent run-ref SHA256
`47e226be29c6064771fdc1683092b9873a3b60f6fd8cb6ec49246ff4d47fe861`.
It retains no final body, GetByRun error, privacy flag or receipt-parse reason.
The old exact cause remains unknown. GetByRun reads the persisted final row and
decodes its effects/limitations JSON; it has no actor-dependent read redaction.
The original checker already compares child IDs unordered. Its standard struct
JSON decoder can accept case aliases, which motivates this stricter diagnostic
parser without assigning that hardening gap as the old failure's cause.

The original checker, fixture, windows and reports remain unchanged by this
helper. The new owning fixture must actually use this classifier and export
its result at the failure boundary before worker/state cleanup. The caller must
separately record final availability/verdict/privacy rather than converting a
successful projection into completion or owner truth. No model call is required
for the diagnostic tests.

## Isolated validation

The pure unit matrix covers whitespace and ordering, canonical escaped JSON,
fences/narrative, malformed/trailing documents, aliases and Unicode duplicates,
unknown/wrapped fields, null/wrong-shaped arrays and IDs, duplicates, wrong IDs,
untrimmed/case-changed IDs, actual-child failures and content-free export with
private-looking markers.

Source snapshot `20261012-110534-637` on isolated laba root
`/srv/storage/butler/agents/delegation-receipt-diagnostics` passed33 test/subtest
names across2 top-level tests, with0 failures/skips. Go1.27.2 ran
`go test -json -race -run '^TestNativeReceiptProjection' -count=1 -timeout=60s`
and targeted `go vet ./apps/orchestrator/internal/app`, both with networking
disabled and repo-local caches. No PostgreSQL, auth or provider entry was needed.

Report: `reports/receipt-20261012T040624Z/tests.jsonl`, SHA256
`289ac813adcb72a2a84cca859c9146fe4c0e44424cd1df5774c9d233480e6889`.
Helper source SHA256:
`987ce2f4d7bdfa04ef95f22b3c9d84ccf9bfb77ef1315d6d3500e590507917f1`.
All historical windows/reports, current main and deployment were unchanged;
new model calls:0. This unit proof validates classification and safe export;
the new owning scenario's actual helper use remains the root caller's work.
