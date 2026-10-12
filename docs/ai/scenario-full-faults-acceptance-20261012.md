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
Actual weight/manifest bytes are verified first. A failed/no-OOM load remains
failed and its marker sealed. A PASS requires public GGUF metadata read, exit137
and Docker OOMKilled within90 seconds, not an allocator fixture or timeout kill.

The physical load is deliberately separate from Butler's owning kernel: it does
not prove that a real model OOM was received there. Fixed owned-worker tests
separately prove cancellation/resource pressure and durable no-replay controls.
Cloud 3.8 Flash High RAM/OOM are not observable. No actual native calls occur in
these runners. All actual local model-load attempts are explicitly counted.

`mandatory_faults_passed` approves only this bounded fault matrix. Full issue33
requires separate independently reviewed High quality/single-vs-multi complete
cost evidence and the complete mandate→task→delegation→adapter development→
independent checks→approved activation→original task→verified result→one final
notice, repeated with restart/Stop/delivery faults. No signed-quality gate or
old FAILED report is relaxed or reclassified by this current fault report.
