# Current combined deterministic acceptance, 2026-10-12

The current production `verified_task` composition and all eight mandatory fault
scenario groups passed on the same isolated release:
`f48c63a5cc5fb9b53ae3f49bd559a647b335e489`, snapshot
`20261012-192005-097`. This establishes the deterministic gates, not full issue33
acceptance or native High model quality.

The three actual SSH laba invocations used
`/srv/storage/butler/agents/scenario-combined-final`, without changing the shared
deployment. Each captured the same source-manifest SHA256:
`3ce72ea8b49dab4672c14b55cb6345a539df2f29c9cf198dd260e534bdd491a4`.
No new native inference, model import, physical model LOAD or human channel
message was performed.

| Current check | Result | Exact proof under the agent root |
| --- | --- | --- |
| `fault-lab.sh full-faults <commit> <sealed-oom-json>` | 317 pass names, 166 top-level names, 0 fail, 0 skip; all eight scenarios, 35 groups and 176 required names passed | `reports/fault-20261012T122048Z-1502530/report.json` |
| `scenario-release-chain-lab.sh <root> kernel` | 20 pass names, two top-level names, 0 fail, 0 skip; race and vet | `reports/release-chain-20261012T122800Z-1535679/tests.jsonl` |
| `scenario-release-chain-process-lab.sh <root> chain <commit>` | Nine pass names, three top-level names, 0 fail, 0 skip; race and vet | `reports/release-chain-process-20261012T122919Z-1540561/tests.jsonl` |

The [safe machine-readable summary](../../deploy/acceptance/scenario-combined-final-20261012.json)
records exact event/report, source, schema, binary and hardware hashes. The full
fault report SHA256 is
`adeb500d5ae72134119a4bb58a33a8eea40699579df35869287a7af5f32c2b6c`;
its current source-tree SHA256 is
`18e2ca0480ac04da0186a0c28d1baf48d336bfd8e17c8cab842f8adad39affa0`.

The fault matrix includes actual Chromium, Broker, PostgreSQL contention and
dump/restore checks. On this combined release, actual descendant grant revocation
and both owner-control/dispatch orderings passed. The kernel run independently
checks current owner verification, partial/unverified/foreign results, deleted
or corrected source evidence, correction before sending, atomic notice failure,
and ten denied provider entries. The process run checks the ordinary promoted
Task, bounded child developer, actual converter/Broker runtime, independently
checked activation, exact original CSV conversion, immutable readiness patch,
current owner completion and one Task-owned notice using the actual Telegram
adapter with a loopback HTTP recipient. Planning is scripted; it supplies no
owner verification, physical tool result or final notice.

The process faults cover a real Broker SIGKILL/restart at the same endpoint and
configuration, explicit recipient absence, acceptance followed by ACK commit
failure and natural outbox-claim expiry, and Stop with a live child Run/lease
before its first tool. Stop denies old child source access and Broker preparation;
worker drain confirms zero child actions and success notices, no live old lease,
and retained confirmed parent effects. Lost ACK preserves the frozen semantic
identity and becomes `delivery_unknown`; restarting the dispatcher does not send
again or rerun work. The generic factory observes 17 entries in a successful
workflow. A separate actual failed provider Start leaves one spent model admission
with NULL usage and zero tool effects/notices.

The additional `technical_failure_projection` case injects an outward failed-tool
channel event after actual successful Core execution. It verifies quiet delivery
projection and is explicitly not proof of runtime or Broker recovery. The result
verified by the owner is `prepared_verified`: exact original CSV conversion and
checked stored `ready=true` patch; the original project remains unapplied.

The prior physical OOM JSON SHA256 `23fc8f4cab1d105333d3057f136008d7ce0964feba7fbea0bec222c00c90b234`
and public loader log SHA256 `2303783bdb57a79266815f68908a10b053107acaebd3485badce8bbb7cf0d5ab`
were copied only for reading. The original strict report still says FAIL and
`model_metadata_read=false`. The independently reviewed additive vocabulary
witness reaches GGUF metadata processing before OOM; it does not establish
complete tensor loading, inference, High server RAM/OOM, Butler kernel receipt
of that physical OOM or deployment authority. The original one-use global marker
and every old failed/native report remain unchanged.

`issue_33_ready` remains false. Current independently reviewed native quality and
the separately frozen bounded native High release chain must pass before an
overall acceptance verdict. These deterministic proofs are not transferred to a
different production source without relevant validation.
