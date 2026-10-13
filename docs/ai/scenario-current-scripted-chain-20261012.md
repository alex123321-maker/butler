# Current full scripted release chain

The complete owning process chain passed on source
`bb43e65b1adb69f866537e11f5080d4a82793217`, using deterministic scripted planning
and actual disposable Core/PostgreSQL, Broker and isolated converter runtime.
Twenty kernel names/two top tests and nine whole-process names/three top tests
passed, with no failures or skips and race/vet. No provider inference,
authentication import or human channel delivery occurred.

Both runs used the exact source-manifest SHA256
`a13aa771ffae018ca9a03ce4c3d44cc7c171522eee3426d966b9907544c7fafb`:

- Kernel: `/srv/storage/butler/agents/scenario-combined-v3-chain/reports/release-chain-20261012T152942Z-2140066`.
- Full process: `/srv/storage/butler/agents/scenario-combined-v3-chain/reports/release-chain-process-20261012T154042Z-2174032`.
- Whole-process test events SHA256: `a3fc10504ca1eda3511e36a6109a4edfe3fcf10b8e7d42e5e14d2f406f3b3543`.
- Whole-process binaries manifest SHA256: `6f27a7c32d917a2d48cbe4827f7cbee696d5dbdfa1ec93daf3410e6a8d6ba0b3`.
- Hardware declaration SHA256: `00e48da54c568edf9090fc0838c99aaf8c56c98b49a141b88a6137fe176386b7`.

Whole-process cases were generic current-owned provider, charged start failure,
baseline, actual Broker restart after admission, explicit recipient absence,
accepted request with acknowledgement-commit failure, child Stop after admission
and quiet technical failure projection. The owning task still proves its original
prepare-only goal: the checked patch is stored as `prepared_verified`; applying it
to the original source is not claimed. One semantic notice is sent to the
loopback recipient in the successful case. Lost acknowledgement remains UNKNOWN
without replay; explicitly proved absence permits only the frozen bounded retry.

The [earlier full-process RED](scenario-release-chain-high-offline-20261012.md)
and later narrow diagnostic PASS remain unchanged. This is a new complete run
after real transport/diagnostic changes, with the same automatic25/30/20-second
fixture fences. It does not establish the cause of the older unreproduced timeout.

Reproduction, with this clean source synced on the lab host:

```sh
bash scripts/dev/scenario-release-chain-lab.sh /srv/storage/butler/agents/scenario-combined-v3-chain kernel bb43e65b1adb69f866537e11f5080d4a82793217
bash scripts/dev/scenario-release-chain-process-lab.sh /srv/storage/butler/agents/scenario-combined-v3-chain chain bb43e65b1adb69f866537e11f5080d4a82793217
```

These measurements verify product execution and delivery contracts with scripted
planning. The new cloud shadow quality/cost gate and the separately admitted
actual High chain remain pending; this report cannot authorize them or a merge.
