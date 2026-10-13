# Closed V11: extra read, all measured expense, no release acceptance

The actual native run on `323da2db21b37e2efd39639eaff98f838a33f0c6` is
**NO_GO**. The first case stopped at the fixture's exact13-action assertion:
there were14 confirmed actions, including two GET reads of the same immutable
patch checks. No provider request timed out. The original CSV conversion passed
its exact-byte check before that assertion. Owner verification and final notice
were not reached; the four remaining scenarios were not attempted.

The [canonical audit](../../deploy/acceptance/release-chain-v11-final-audit-20261013.json)
retains this failed result. Its generic `runtime_or_global_deadline` label covers
the actual exit1; this was not a model, case or global timeout. The independently
verified [correlation](../../deploy/acceptance/release-chain-v11-failure-correlation-20261013.json)
records the concrete assertion, completed runs, action authorizations, costs and
zero notices. All18 native requests558–575 completed with measured expense:
213479 input,3631 output and246218 total tokens, including provider-reported
thinking. Maximum request duration was60374ms. No safe retry or new unknown
expense was accepted.

The reviewer read only14 individually named own proposal leaves. Every original
argument hash matched its actual Core action. Both check proposals used the same
fixed GET resource and artifact; prepare-patch and every mutation occurred once.
The [proposal diagnostics](../../deploy/acceptance/release-chain-v11-proposal-diagnostics-20261013.json)
do not contain original arguments. The old projection deliberately clears check
response fields when there are two receipts, so actual per-read artifact and
boolean response fields remain NULL. They cannot be reconstructed from requests
or a confirmed dispatch. Two source-observer records are task-watchdog cleanup
cancellations after completed runs, not source timeout or provider failure.

The [history575](../../deploy/acceptance/release-chain-v11-history575-20261013.json)
and [independent carry](../../deploy/acceptance/release-chain-v11-carry575-independent-20261013.json)
append all18 attempts without changing the first557. Cumulative known subtotal:
689116 input,18790 output,1039921 total;546 known visible usage pairs,29 unknown
pairs and49 unknown full totals. The old628 ceiling has53 unused allowances and
stays sealed. Old failures and missing causes remain unchanged.

Independent auditor source `738ef1b629ea0d709a496ed20203c0d973d04369`, snapshot
`20261014-010049-476`, TreeHash
`fef314e960e2e7860918a839085c379fbdf3f447252ebbe94f0e050832132dc0`,
passed29 exact Linux race checks and vet with zero FAIL/SKIP. Its separate
executable SHA256 is
`d228b1edf9cbc9596e342212ff6c97aa596db2deca7f904beb1193c0fca87b03`;
[provenance](../../deploy/acceptance/release-chain-v11-auditor-provenance-20261013.json)
binds all2316 source files, runtime and events. Canonical audit SHA256:
`85a32e433345e53db47b0f314feb55c5d23baadff0e18be0756084e77f9f54ae`;
carry fold:
`d24720adf3522dc19ada8b53a7d54d4485fe7774c0cd029d3c717c9f1468db05`.

The [reproducer](../../scripts/dev/release-chain-v11-final-audit-reproduce.sh)
requires the retained isolated proofs. It copies14 named public-safe inputs and
only the explicitly declared own proposal leaves into a new exclusive review
directory; it performs no inference and mounts no original private directory,
provider import, credential material or SSE bodies. It never resumes the native
window:

```bash
bash scripts/dev/release-chain-v11-final-audit-reproduce.sh \
 /srv/storage/butler/agents/scenario-release-chain-v11-high-20261013/reports/release-chain-process-20261013T173957Z-2779736 \
 /srv/storage/butler/agents/scenario-release-chain-v11-auditor-20261013/snapshots/20261014-010049-476 \
 /srv/storage/butler/agents/scenario-release-chain-v11-auditor-20261013/reports/independent-checker-738ef1b-20261013T180145Z-2847478/bin/chain-final-audit \
 738ef1b629ea0d709a496ed20203c0d973d04369 \
 fef314e960e2e7860918a839085c379fbdf3f447252ebbe94f0e050832132dc0 \
 d228b1edf9cbc9596e342212ff6c97aa596db2deca7f904beb1193c0fca87b03
```

The user's later call-count steering supports prospective repeated owned GET
reads within current Core quotas, with every actual receipt verified. It does
not turn this closed report into PASS. All five actual scenarios and final
independent acceptance remain required before PR readiness, merge and closure.
