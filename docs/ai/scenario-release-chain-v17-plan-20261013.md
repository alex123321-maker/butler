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
