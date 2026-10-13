# Current mandatory fault evidence

The complete fixed mandatory matrix passed on clean source
`29cd58c33da760324345e919855025b41b7e9351`:321 test names,166 top-level tests,
35 groups and8 required scenarios, with no failures or skips. Source tree SHA256
is `4f2137e1960696614393aac02ad248dc8e5c83c181728924b9fb166b4c2454c0`.
The current source includes the complete failed native-packet usage retention
and request-local private transport stage observer. No provider inference,
credential import, deployment or additional physical model load was performed.

Proof directory:
`/srv/storage/butler/agents/scenario-combined-v3-final/reports/fault-20261012T151615Z-2085619`.
Exact source-manifest SHA256:
`db1f61c8b87173de8d3d0dfa088732c819ad0759511f2bc00fddb167bb0317da`.
The unchanged report copied to
[the safe artifact](../../deploy/acceptance/full-mandatory-faults-v3-final-20261012.json)
is41958 bytes, SHA256
`a921d03e765676a28032d65a165054099c792a100f8efecc271a2abeb6c57a73`.

The run exercised actual disposable PostgreSQL, Redis, Chromium and backup/restore
with synthetic external effects. It reused the previously sealed cached-Qwen
OOM evidence and separately checked vocabulary witness; the original failed
metadata-line detector remains unchanged. Neither that witness nor this fault
report proves remote High model memory or model quality.

Reproduction from this source, on the lab host only:

```powershell
pwsh -NoProfile -File scripts/dev/fault-lab.ps1 -Profile FullFaults `
  -RemoteRoot /srv/storage/butler/agents/scenario-combined-v3-final `
  -LocalModelLoadOOM /srv/storage/butler/agents/scenario-full-faults/reports/local-model-load-oom-20261012T104346Z-1157147/local-load-oom.json
```

The report explicitly retains `issue_33_ready=false`. All three closed High
shadow NO_GO results remain immutable, including the subsequently signed and
executed [actual V3 window](scenario-cloud-v3-independent-audit-20261012.md).
Full native quality/comparison and the actual High release chain remain required.
The [current complete scripted chain](scenario-current-scripted-chain-20261012.md)
has separately passed; its earlier full-process RED and narrow PASS remain
recorded in [their offline report](scenario-release-chain-high-offline-20261012.md).
Neither scripted execution nor these fault checks establish native acceptance.
