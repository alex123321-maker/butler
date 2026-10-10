# Versioned memory and structured ingestion acceptance

This records the implementation and lab evidence for issues 22 and 23. PostgreSQL
owns sources, immutable revisions, candidates/decisions, original assets and job
claims. Existing transcript, working, episodic and profile stores remain separate;
pgvector and lexical indexes remain derived retrieval data. Deployment and issue
closure require integration of migrations 046–049 and the private deletion anchor.

## Issue 22 criteria

| Requirement | Implementation and acceptance evidence |
| --- | --- |
| Explicit scoped preference supersession; history retained | `catalog.Decide` locks the scope/key head and requires `expected_active_id`. `TestSourceDuplicateAndConflictingPreferencesIntegration` and `TestConcurrentExplicitSupersessionIntegration` prove one current owner decision, historical supersession and concurrent CAS. |
| Contradictions not resolved by score/upload time | Every extracted value is a candidate, including confidence 1. `TestCurrentProjectRuleIsScopedAndOldDocumentRemainsCandidateIntegration` proves a document dated 2020 and uploaded now does not replace the confirmed project preference. |
| Original message/fragment, author, event/extraction time, version and child ancestry | Transcript source backfill/future triggers, immutable catalog revisions, object links/locators and parent pins retain originals. `TestCorrectionExportAndOriginalParentPinsIntegration`, `TestAuxiliaryManifestPinsOriginalAncestorsIntegration` and `TestTransitiveDeletionAndBackupRestoreIntegration` cover a false child report, parent correction and deletion. |
| Candidates distinct from facts and authority | `memory_claims` acceptance is separate from extraction confidence; current constraints/goals/mandates stay in their typed owner-controlled stores. Existing provenance tests reject instruction promotion from memories/documents/child output. No index operation writes owner authority. |
| Scope and executor access, including local indexes | `AccessTx` checks current ownership or independent current job claim, task/assignment and exact owner source grants. The project-rule bundle test proves an ungranted foreign project is excluded and task amendment invalidates the grant. `TestProjectStructuredIndexRenameReusesVectorsAndRefreshesLocatorIntegration` proves real project-scope indexing and lexical isolation. |
| Owner inspect/correct/export/delete | Authenticated v4 APIs expose source history, candidates, originals, citations and NDJSON without export list truncation. API PG tests and two Playwright fixtures cover owner proof, exact revision correction, supersession history, export/download and deletion. The owner token stays in page memory. |
| Transitive removal from indexes, summaries, caches and final private content | Source deletion recursively removes class rows/vectors/original assets, invalidates transcript heads/covered summaries, bumps working revisions, redacts private transcript/results/artifacts/outbox/input payloads, and persists scoped Redis cleanup. `TestPrivacyCleanupRestartAndStaleSummaryWorkingPublicationIntegration`, `TestDeletionRedactsImmutableFinalWithoutInventingUndoIntegration` and late provider/Coordinator/ClaimGrant race tests prove retained IDs/hashes/verdicts/effects and denial of late publication. |
| Backup policy and no resurrection | Content-bearing PG backups expire within 30 days unless the owner chooses a stricter period; the newest independent content-free anchor survives every retained backup. Missing/truncated/wrong history/private-mode/symlink anchors fail closed. Actual `pg_dump -Fc` before deletion, `pg_restore --exit-on-error` into a fresh test DB and `TestActualPostgresDumpRestoreDeletionIntegration` prove access denied before replay, original/child/binary/chunk/claim text purged after replay, and reindex denied. |
| Legacy migration and concurrent changes | 046 quarantines legacy records instead of inventing verification; 048 attaches original transcripts and privacy fencing; 049 retains original assets. Actual PG tests use isolated schemas with MaxConns2 and race detection, including simultaneous supersession, source deletion versus grant/result, stale summaries, forged job snapshots and lease takeover. |

## Issue 23 criteria

| Requirement | Implementation and acceptance evidence |
| --- | --- |
| Original asset/message and adjacent fragment context | Owner-proved original revision downloads plus `Citation` and ordinal neighbours; source, parser, line/byte/page/heading, author/time/thread and repository locators. API and Playwright upload fixtures open original/citation context. |
| Repeat-safe updates and rename | Stable logical source/entity keys; exact duplicates keep IDs, single-function changes update one entity. Parsed-stage restart repairs publication. Project rename updates the path in citations/lexical search and reuses the unchanged exact-space vector without another inference. |
| Delete/revoke everywhere | The same catalog access/tombstone graph applies to original assets, chunks, extracted candidates, semantic entries and scoped caches. Revoked originals cannot be downloaded or recreated. |
| Mixed Russian and code identifiers | UTF-8-safe fragments and simple/Russian lexical indexes retain identifiers, path and repository revision. Real PG project query `new.go` returns Russian function context. |
| Real OCR only for absent usable text layer | Fixed Poppler successfully extracts a synthetic Unicode Russian PDF; Tesseract rus+eng recognizes the real scanned Russian PNG embedded in PDF. OCR refuses a usable text layer, and corruption/blank text/17 pages/cancellation never report success. Method and uncertainty remain attached. |
| Meaningful tables | CSV/TSV, Markdown and DOCX fragments retain headers with each logical row. Russian table fixtures verify row relationships. PDF layout/page text remains layout data; ambiguous visual grids do not receive invented cell semantics. |
| Bounded files/time/fragments and isolated diagnostics | At most 8 MiB source, 2,000 fragments and 16 MiB normalized text; cooperative 10-second Go parsing acceptance deadline. Native parser adds fixed 15-second process deadline, 16 pages, 8 MiB stdout, 1600×1600 pixels/page, one admitted request, 512 MiB RAM, 32 PIDs and private 128 MiB tmpfs. Corrupt assets persist generic diagnostics with no fragments. |
| No execution of embedded code/macros/instructions | Go uses standard AST; DOCX uses bounded ZIP/XML and rejects macro assets. Parser accepts bytes only, with fixed executables/arguments and no paths/URLs/model-selected commands or credentials. Parser container has read-only rootfs, no caps/host ports and an internal-only network. Actual native-engine test ran with network none. |
| Dependency choice | No Go/Python library was added. Standard Go parsers cover the supported structured formats; separate Poppler/Tesseract packages are justified by actual PDF/OCR. Non-Go code uses explicit text/Markdown mode. |

## Owned auxiliary composition

The production opt-in path composes one source catalog and execution-budget
Controller with pinned `local-models:9090`, a single private mode-600 token file,
exact reviewed embedding space and explicit `local_no_billing` pricing. Price is
separate from locality proof. Live query requires current run ownership; post-run
indexing requires a current durable job claim, original assignment and canonical
source/ancestor hashes. A completed task is not silently detached from its policy.
Every IPC uses the admitted bounded context and is charged/rechecked independently.

Atomic `EnqueueWithPublication` saves a finite owner job, building generation and
index projection in one transaction. Kind-specific claims avoid consuming another
worker's queue. Actual PG tests prove rollback without orphan generation, failure
after a durable four-vector prefix, restarted completion without repeated prefix
inference and deletion during inference rejecting late publication. Independent
owner review with a measured quality artifact and expected active pointer is
required for activation. Extraction confidence and index completion confirm no
facts. Reranking and legacy cloud extraction/summary workers remain off.

## Lab evidence, 2026-10-11

All execution occurred on the isolated laba agent root; no shared Compose/current
pointer, production PG reset or credential mutation was performed.

- Actual PG/race: `internal/memory/...`, `internal/actionauth`, orchestrator
  API/app/completion/orchestrator passed; fresh recovery/project-index cases passed
  with pool size two. Other unrelated optional database tests may skip without
  their own environment variables; this is not an all-system PG claim.
- Actual owned `Runtime.ProcessOne`: two real embedding IPCs produced five vectors
  in 10.01 seconds; generation stayed building and owner deletion erased vectors.
- Fixed native Russian PDF/scan/blank/17-page/cancel test passed in 1.79 seconds; the final actual HTTP boundary test passed in 0.96 seconds.
  An earlier instrumented equivalent native run peaked at 163,844,096 bytes of
  cgroup memory (~156 MiB); separate actual HTTP adapter test peaked at
  136,269,824 bytes (~130 MiB). HTTP proxy/redirect bypass is disabled.
- Actual PG custom dump was private mode 600 (232,127 bytes). Restore/deletion
  verification passed under race against a fresh test DB and the newest anchor.
- Web typecheck, lint and two functional Playwright tests passed on laba.
- All `./apps/... ./internal/... ./cmd/... ./scripts/smoke` passed on laba. An initial
  `go test ./...` on the supplied peer baseline found duplicate standalone `main`
  declarations in `scripts/dev` and an unrelated browser-bridge two-second
  shutdown timeout. Browser-bridge then passed three isolated repeats; no local
  fix to another agent's helper files is included in this delta.

## Operational limits

Provision and back up the private independent anchor before rollout; never create
an empty replacement after loss/restore. Reconcile the newest anchor before memory
release. Unknown offline backups and already delivered/exported external copies
need the owner's separate erasure procedure; no external undo is claimed.

Unattributable global legacy memory stays quarantined until reviewed ownership
migration. DOCX pages reflect explicit page breaks, not rendered pagination. The
standard Go AST call is synchronous and cannot be forcibly preempted; its bounded
input and pre/post deadline checks prevent late acceptance. PDF/OCR additionally
has process/container confinement. Automatic extraction and quality thresholds are
separate work and do not change candidate-versus-authority semantics here.

Runtime enablement still requires the operator's exact local pricing/profile,
private token and optional Compose files. Parser/indexer default off until those
conditions are configured. Deployment acceptance belongs to the integrating root
agent; passing isolated tests alone does not certify its running main stack.
