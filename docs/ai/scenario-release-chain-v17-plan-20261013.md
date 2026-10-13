# Pending fault cases with retained failed-case evidence

The human requested proceeding directly to untested cases and running High
immediately after a fix. This fresh declaration runs `owner_stop`,
`explicit_absence`, `lost_ack` and `restart_sent`, in that fixed order. It
does not rerun the separate normal case, the completed quality benchmark or
the four paired comparison tasks. Each fault case owns a fresh isolated
fixture. A failed case remains failed while subsequent independent cases
continue under the same finite global window; any failure keeps overall NO_GO.

V16 is closed at actual636. All20 of its new model requests completed with
measured usage, but the restart inventory failed and the final product export
was the literal `[]\n`. Two prepare-patch proposals are retained. A read-only
copy of the named physical PostgreSQL cluster confirmed that fixture cleanup
had dropped the action schema; no actual receipt or artifact count can be
recovered from that copy. The [recovery report](../../deploy/acceptance/release-chain-v16-readonly-pg-recovery-20261013.json)
has SHA256 `41624a4af3cfd4c09b9465f9d489c515510df7a75c233aa9d59ef81cbb0fc168`.
Original physical bytes, empty export, history and closed windows are unchanged.
The effects of the duplicate proposals remain UNKNOWN.

The future-only evidence collector now persists the actual owning SQL snapshot
even if original-result artifact capture fails. It records an invalid or
incomplete capture, NULL original result and failed case before returning the
typed error. It does not turn missing or partial evidence into PASS. Old
collector behavior and native entries retain their original semantics; the
closed V16 entry denies execution in the current source.

The new window is636+120=756, starting from the independently checked fold
`314bf4a5a3b649f6dc6354a03ea4c536a85f6a8ea92c756ef50bcc288aeda951`.
Known cumulative subtotals are1372607 input,30515 output and1844192 total
tokens. Historical30 unknown input/output pairs and50 unknown full costs remain
unknown. All new attempts, including failures, remain individually recorded.
Request300s, case40m, global120m, serialized input4MiB and all eight existing
resource dimensions are unchanged. There is no new write, source, Stop,
authority, original-result or delivery waiver.

Acceptance remains a composite of retained V14 original CSV/checked immutable
patch facts and the four actual fault cases. `restart_sent` must prove the full
current Core/Broker/independent result verification/one-sent-notice chain. The
old V14 normal case remains NO_GO. Independent final source/PG/raw proposal/cost
acceptance is required before ready/merge/close.

```powershell
pwsh -NoProfile -File scripts/dev/lab.ps1 -Action Sync -RemoteRoot /srv/storage/butler/agents/scenario-release-chain-v17-high-20261013
```

```bash
ROOT=/srv/storage/butler/agents/scenario-release-chain-v17-high-20261013
bash "$ROOT/current/scripts/dev/scenario-release-chain-v17-high-lab.sh" "$ROOT" EXACT_CLEAN_COMMIT
```

The wrapper builds and immediately executes the frozen native entry. Meaningful
collector PostgreSQL and changed-source checks run in parallel, not as a broad
pre-model gate. Every actual fault result and any newly found problem remains
separate from the preserved quality benchmark and final release acceptance.

The parallel [source/runtime review](../../deploy/acceptance/v17-parallel-build-source-1a22b7a-20261013.json)
pins measured commit `1a22b7af2f87f56c82a45b53ca7abaa64eaa2980`, snapshot
`20261014-033559-144`, tree
`bf54ecd28bdd8753469ce2bc406a34db87726c4a305356d4e81bd8dc89f65a16`
and app binary
`3afd047fe67663138fdef479cc79f6d2a68fc202ae1003de674374f338b13cb5`.
All657 production files equal the measured9c full-fault source; seven runtime
binaries and fixed image/resource pins match. This is source/runtime correlation,
not final release acceptance.

The actual PostgreSQL [collector review](../../deploy/acceptance/v17-collector-pg-independent-1a22b7a-20261013.json)
independently verifies five named passes, zero failures/skips, race detection and
empty vet diagnostics on those same source bytes. It covers retained failed
capture, unchanged legacy denial, exact pending-case order and continuing other
cases after a failure. Raw tests SHA256 is
`29987e4ef4df8913311ff0afa2fd83feb902f410cde64c29601f36b6a4a5a082`.
The [narrow reproducer](../../scripts/dev/scenario-release-chain-v17-evidence-check.ps1)
runs only those five checks against the retained read-only snapshot and a new
isolated PostgreSQL proof; it performs no model/authentication calls:

```powershell
pwsh -NoProfile -File scripts/dev/scenario-release-chain-v17-evidence-check.ps1
```

V17 was stopped after a verified terminal fixture stall and is now closed
**NO_GO**. The actual `owner_stop` subcase passed in71.14s; its owning product
record is retained for separate independent acceptance. The `explicit_absence`
case reached a blocked activated-parent input and did not verify delivery.
The later `lost_ack` and `restart_sent` cases did not run. The final scope file
is absent/zero bytes after the isolated native process was stopped; no scope
or completed-case record is reconstructed as a native PASS.

Requests637–658 contain21 completed measured responses and one cancelled
response-stream attempt658 with diagnostic `transport_connection_error`,
elapsed30175ms and NULL usage/version. This was not a300s nominal timeout.
The native failure predates the manual stop by several minutes. Known new
usage is229303 input,3796 output and255324 total tokens; the cancelled attempt
remains UNKNOWN. The final exact carry is independently derived from the
owning exported ledger, not from an absent scope file.

The pre-stop [terminal metadata](../../deploy/acceptance/release-chain-v17-live-terminal-stall-20261013.json)
records zero unfinished native requests, one completed converter-return input
and one blocked input with `execution_stopped` and no retry. The fixture's
second completion wait only counted two completed inputs, ignoring that
terminal failure; it could therefore wait the full40m while the model was idle.
The future repair must detect that actual failed/blocked status immediately.

The pre-stop [action and budget snapshot](../../deploy/acceptance/release-chain-v17-live-action-budget-20261013.json)
records two distinct confirmed prepare-patch effects with completed dispatches,
identical authorization argument hashes and identical fingerprints. A shared
fingerprint does not prove deduplication because the effect IDs differ. Exact
runtime receipts were independently recovered from a read-only copy of the
named fixture PostgreSQL cluster in the [receipt recovery report](../../deploy/acceptance/release-chain-v17-readonly-pg-receipts-20261013.json),
SHA256 `c596334f44fceb748581c2f113afc62353a55874f4c342a93b753e95d5806013`.
The two completed prepare receipts have distinct effect IDs, distinct
idempotency-key hashes and distinct actual artifact IDs. Each corresponding
check passed on its own unapplied artifact. This proves two actual commits
across the candidate and activated parent runs. Original physical bytes before
and after the copy agree; no original database/window/source was reopened.
The cancellation cause of658 remains UNKNOWN separately. No duplicate-write
or original-result criterion is waived. A later repaired scope must skip the
successful Stop case and run only the remaining three cases.
