# Current mandatory faults for issue33

Base main895bdd3. This current gate is additive: the independently signed V3
criteria digest `76437b99d5eb79baadb39c5eb6d7958a2d6108296c7b6dc6c6118f42b21cd058`,
corpus, old V1 matrix and failed/native history are immutable. Old dependency
labels are historical; they are not rewritten to PASS.

`FullFaultGroups` owns exact required test names; `FullFaultScenarios` contains
the eight mandatory bounded product fault groups. Report mode requires an
exclusive pre-test pin of current source bytes, commit, migrations, new fault
criteria and hardware/images. Missing, skipped, failed, duplicate, foreign or
malformed named events fail; actual single-tab Chromium subtests are required.

| Mandatory fault | Current owning evidence |
| --- | --- |
| Same-session multistep browser; stale/lost page | Browser/Broker session E2E; actual extension/native-host Chrome `manual_input`, `dom_mutation`, `navigation`, `lost_ack_restart`, `two_executors`, `installed_native` |
| Approval restart, fast/double/stale reply | Actual worker SIGKILL/restart; decision-before-notification; durable duplicate/stale decisions and exact saved arguments |
| “Не отправляй”, all-descendant Stop, lost resource | Owner amendment/Stop vs real dispatch; actual grandchild grant; shared reader/writer and expired ownership; private workspace-runtime drain |
| Disconnect before/after effect, UNKNOWN, failed delivery | Permanent effect journal, atomic result/outbox/unknown-send recovery; real workspace rename/failed metadata and lost erase ACK; actual Chrome lost ACK |
| Fresh constraint, new task after old failure, deletion/reindex | Current summary/context tests plus `TestDurableTaskExecutionIntegration`; Catalog/source index; actual three-phase PG dump/delete/restore with newest private deletion anchor |
| Tab/file conflict, shared budget, loops/depth | Actual Chrome two executors and filesystem contenders; independently checked conflicting child reports; durable budget/normalized loop, delegation total/depth/dependency caps |
| External/child instruction, secret, forbidden resource | Selected-original child/source guards; instruction derivation, secret-before-IPC, workspace/egress/reflection and real credential/Broker boundary |
| Model OOM, incompatible/failed release, rollback | Separately sealed physical cached CPU model LOAD OOM; separate owned-process/pressure controls; current reviewed extension version/probe/restart/rollback, immutable receipts and workspace readiness |

All evidence is initially PENDING until the executable current-release report is
recorded. Prior reports explain invariants but cannot substitute for current runs.

Recorded first current full matrix on2a2be53 is FAIL and retained unchanged:
`fault-20261012T103522Z-1118687/report.json` SHA
`3de13d44a24b3a4c9351d67b0c4b3aabf1691a1a4322d69e9bd8792a288ac2b0`.
It found an obsolete deletion fixture that acquired a source body in finalizing
and a new single-tab runner with the wrong working directory. The fixture now
acquires/pins under actual current ownership before finalizing and asserts
finalizing/new-body and deleted-owner reads deny. Narrow actual PostgreSQL/race/
vet passes all8names (2tops/6orders), no skips, on54aec53: events SHA
`253bf70c5982862a7eb7f123e0a45e98b924ebd55f22a87fb68f518094170698`.
The Chrome lane now uses the actual broker package directory for migrations.
These fixes change fixture/runner setup, not production source guards.

Reproduction on SSH laba only, in a dedicated agent root:

```text
scripts/dev/local-model-load-oom-lab.sh <agent-root> <exact-commit>
scripts/dev/fault-lab.sh <agent-root> full-faults <exact-commit> <sealed-local-load-oom.json>
```

The first command is an explicitly authorized **one-use** physical model load,
not a prepare/retry loop. It only mounts the already cached Qwen3 0.6B GGUF read
only into a pinned cached llama.cpp runtime; runc, UID10001, no network/GPU/caps,
read-only root, bounded tmpfs/CPU/PIDs and 256 MiB RAM/swap are checked before
start. It never downloads, sends HTTP, supplies a prompt, performs inference,
touches the existing loaded-model observer, or mounts source/DB/auth/socket.
Actual weight/local-manifest/reviewed-registry-layer bytes are verified first.
The cached runner's own metadata help defines `--load-mode none` as “no special
loading mode”, compared with mmap/mlock. It loads the explicit full GGUF without
special mapping; it does not skip tensors. The global one-use marker is fixed at
`/srv/storage/butler/fault-acceptance-windows/issue33-cached-qwen3-load-20261012`,
so changing the agent root cannot open another attempt. A failed/no-OOM load remains
failed and its marker sealed. A PASS requires public GGUF metadata read, exit137
and Docker OOMKilled within90 seconds, not an allocator fixture or timeout kill.

The initial preflight on ba62c02 failed before `docker start`: `len` of nil
Docker `DeviceRequests` threw during inspect. Its original report and global
marker are retained. Actual model-load attempts were0; no original created-state
proof survived, so none is added retrospectively. The explicitly reviewed
`resume-zero-start` mode admits only this exact recorded failure: old committed
runner bytes and source ID, absent start/attempt markers, and an exclusive
continuation marker are required. A new owned container must have a saved actual
`created/0001-01-01T00:00:00Z` state and exact isolation proof. Global atomic
`load-started` precedes its single start and can never be reset/reused. This is
the same one-load experiment, not another OOM attempt or another root's window.

`oom_killed` describes actual OS/Docker physical OOM;
`butler_kernel_received_oom=false` records that the physical load is deliberately
separate from Butler's owning kernel: it does
not prove that a real model OOM was received there. Fixed owned-worker tests
separately prove cancellation/resource pressure and durable no-replay controls.
Cloud 3.8 Flash High RAM/OOM are not observable. No actual native calls occur in
these runners. All actual local model-load attempts are explicitly counted.

The one physical load artifact SHA
`23fc8f4cab1d105333d3057f136008d7ce0964feba7fbea0bec222c00c90b234`
has `oom_killed=true`, exit137, elapsed1second, actual model-load attempts1,
inference/native calls0, verified256MiB limits/isolation and
`model_metadata_read=false`. Its original strict gate remains FAIL: the older
complete-metadata log string is absent. Global load-started remains sealed; no
second load or inference occurred. Public no-input loader log SHA
`2303783bdb57a79266815f68908a10b053107acaebd3485badce8bbb7cf0d5ab`
is retained unchanged. Content-free original JSON and this public software log
are copied into testdata only to verify exact hashes and adversarial rejections.

Independent reviewer `codex-independent-cloud-critic-20261012` and the root
approved a **separate additive vocabulary witness-v2**. Current mandatory-fault
contractv3 accepts the old strict successful marker, or exactly this old strict
FAIL plus the approved original/log/global-start hash pins and both ordered
patterns: loading `/model.gguf`, then the specific control-looking token128247
`</s>` warning. The new scope states vocabulary metadata processing reached;
complete weights loaded, inference, Butler kernel receipt and deployment
authority remain false. Original FALSE/FAIL is never rewritten. This is fixed
independent source review and root approval, not an Ed25519 signature or proven
administrative separation; the generated adapter author does not own this gate.

Primary reasoning uses the cached binary's declared llama.cpp revision
`631109b34da437a3c4a5ebd75091d677671392e3` (version0.5.0-dev/build1):
[vocabulary loader](https://github.com/ggml-org/llama.cpp/blob/631109b34da437a3c4a5ebd75091d677671392e3/src/llama-vocab.cpp#L1799)
reads the GGUF metadata context, builds its token mapping and emits the exact
[control-token warning](https://github.com/ggml-org/llama.cpp/blob/631109b34da437a3c4a5ebd75091d677671392e3/src/llama-vocab.cpp#L2696)
while checking vocabulary entries; the preceding
[server loader message](https://github.com/ggml-org/llama.cpp/blob/631109b34da437a3c4a5ebd75091d677671392e3/tools/server/server-context.cpp#L944)
identifies the explicitly mounted model. This supports that narrow stage from
actual preserved bytes before OOM. The image is pinned and its declared revision
matches these sources; the binary was not rebuilt/attested from that source, and
the log does not prove tensor loading completion, inference or kernel propagation.

`mandatory_faults_passed` approves only this bounded fault matrix. Full issue33
requires separate independently reviewed High quality/single-vs-multi complete
cost evidence and the complete mandate→task→delegation→adapter development→
independent checks→approved activation→original task→verified result→one final
notice, repeated with restart/Stop/delivery faults. No signed-quality gate or
old FAILED report is relaxed or reclassified by this current fault report.
