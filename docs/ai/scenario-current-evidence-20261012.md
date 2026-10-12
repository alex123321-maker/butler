# Scenario acceptance after the owner and delegation merges

## Current baseline

PR71 merged as `90ad32a62d2e6ea9731f583d7e037f2918497393` and closed #16.
PR72 merged as `3d2b44bb0a0626db58c60b26f388fdd53826c8d1` and closed #15 and #32.
The owner's instruction is to merge accepted changes and continue remaining
issues with subagents. #17 remains open: workspace discard does not reconcile
an unknown browser effect or unblock the quarantined tab.

Read the actual acceptance scopes in `delegation-current-evidence.md`,
`initiative-high-acceptance-20261012.md`, `task-owner-state-20261012.md` and
`resource-full-acceptance-20261012.md`. Historical failed reports remain failed;
later acceptance did not reconstruct the missing parent receipt from attempt27.

## Fifteen-report native history preparation

`scenario_native_history_test.go` reads all fourteen existing pinned reports and
the separately accepted initiative report
`deploy/initiatives/native-high-20261012.json` (SHA256
`0ded1f418c3ecf9fad177fe5141350e8c5ca8002452fb462e3114219c4ab4ca9`).
It requires that report's exact release, previous history, task/run, four attempt
ordinals42–45, run-bound reservation identities, result statuses and usage.
Every missing or changed pin is rejected, including a copied report with just
one extra newline.

The baseline is45 attempted requests,20 known usage pairs and25 NULL pairs.
The known subtotal is74698 input and2210 output tokens. This is a partial
observation of these acceptance attempts, not an account-wide or invoice total.
The first three initiative tool turns remain NULL in their original journal;
that absence alone does not prove the upstream provider omitted usage.

Reproduce after an isolated Sync on laba:

```text
bash scripts/dev/scenario-native-history-lab.sh /srv/storage/butler/agents/scenario-native-history
```

Snapshot `20261012-161341-638` passed the fifteen-pin tests with `-race` and
`go vet`, using a read-only source mount and `--network none`. Private proof:
`/srv/storage/butler/agents/scenario-native-history/reports/scenario-native-history-20261012T091357Z-841691`.
Test events SHA256:
`7e09f6aa8835f218da2229efcbe9eaa2ae28ef6d914ed12310f5b68dcfca2873`.
Source manifest SHA256:
`45ece4f3e265ad31d3c62e2b6dc3e8533436666d2ffaac2be10ba8ec3590ca5e`.
The earlier invalid shell invocation produced no valid test evidence and is
retained separately; it is not a passed preparation.

This helper grants no inference allowance and touches no import, provider,
counter, marker, database or deployment. All previous native windows remain
sealed. Any later native experiment needs its own finite scope and window and
must carry this entire45-attempt baseline.

## Active #33 work

Independent subagents are preparing a supplementary typed command/witness
corpus, future tool-turn usage accounting, and actual converter/result/outbox
fault checks. The existing signed240-example Russian corpus remains unchanged.
Fixture validation does not establish model quality. A saved transport attempt
notice does not establish verified-result delivery or a single final notice.

The full mandate-to-extension release chain, its restart/Stop/delivery variants,
model quality measurements and fair single/multiple-agent comparison remain
acceptance gates for #33. Component proofs must retain their precise scopes;
they cannot close the issue by themselves.
