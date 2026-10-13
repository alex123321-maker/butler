# V13 final audit reproduction

The actual V13 five-case process is separate from its future final checker. These commands are prepared for the frozen09cf source and run only after the parent confirms native completion; this document makes no current five-case PASS or final expense claim.

The measured checkout is /srv/storage/butler/agents/scenario-release-chain-v13-high-20261013/snapshots/20261014-015432-120 at commit09cf34c05ab1fc1840dc73f1c9e1ecb2766b0faf and TreeHashe203276bb12e30c16c975952f7e70c92295adba80e76c85dc08d9ca43ade6e70. Actual binaries are app.test c290b25cacdeaf35fb7b89787fe18630689a29c13acbf37ed24bd6b50e005d26, Tool Broker2ed16aca088b7cb90b85b643a7e9533d336ca2d43ea0af5746e44847496d9e8e and converter5f2dee5acef95c2f279b849d7af76003688a3b46a274e0d44456a37c40536f75. The completed proof path for this once-only execution is /srv/storage/butler/agents/scenario-release-chain-v13-high-20261013/reports/release-chain-process-20261013T190121Z-3028894.

The single proposed script is the independently prepared reproduce-v13-chain-audit-09cf34c.sh SHA2566f8054a36b7e461d60679d930ee826c3020056a204e4c797a7d1566af9027f46. Root integrates it as scripts/dev/release-chain-v13-final-audit-reproduce.sh; no competing script is introduced. Reviewer bash-n and author read-only static review passed. The script has not been run against a live process by either review action.

Before invocation, an independent reviewer freezes the combined checker source/executable after49 exact Linux race names and vet (32 V12,11 V13,6 earlier positive compatibility). Set AUDITOR_SOURCE, AUDITOR_BINARY, AUDITOR_COMMIT, AUDITOR_TREE and AUDITOR_SHA from that separately retained proof. Never substitute the pre-native legacy checker or invent future binary/source pins.

```bash
PROOF=/srv/storage/butler/agents/scenario-release-chain-v13-high-20261013/reports/release-chain-process-20261013T190121Z-3028894
bash scripts/dev/release-chain-v13-final-audit-reproduce.sh \
 "$PROOF" "$AUDITOR_SOURCE" "$AUDITOR_BINARY" \
 "$AUDITOR_COMMIT" "$AUDITOR_TREE" "$AUDITOR_SHA" \
 e203276bb12e30c16c975952f7e70c92295adba80e76c85dc08d9ca43ade6e70 \
 c290b25cacdeaf35fb7b89787fe18630689a29c13acbf37ed24bd6b50e005d26
```

The script validates frozen path shapes, hashes and binary modes; requires14 individually named0600 safe leaves; reads only public proposal metadata to enumerate at most120 exact own proposal leaves with ordered576–695 ordinals and bounded262144-byte files. Its copy helper sees only named mounts, produces a new exclusive0700 review directory, and passes read-only named inputs and original own proposals to the separate UID10001 network-none checker. The complete original private directory, provider credentials/imports and SSE bodies are never mounted. No model, Broker dispatch, native restart or PostgreSQL mutation occurs.

The expected success is exit0 with chain_audit.status=CHAIN_V13_WITNESS_AUDIT_PASS and all five names normal_sent,restart_sent,explicit_absence,lost_ack,owner_stop. Check factual all_new_measured_cost against every actual PG entry, including any failed retry. Every completed request has known input/output/total and the selected version. Missing cost stays NULL; only the already-frozen safely recovered timeout exception can produce accepted_unknown_* subsets. When a result is NO_GO those subsets are zero, and actual cost/UNKNOWN remain retained. all_new_expense_measured may befalse for a legitimately accepted recovered unknown; never call that expense fully measured.

The checker binds prerequisitec1aa03290cc85616c4f2156acfde3d116037e7cafd0a4f89b5ea2b9d545c2f4e, humane62000001cd3685560587b1e74c6e0583118e8d0b143cc386f860ac4965d6934, historical575 foldd24720adf3522dc19ada8b53a7d54d4485fe7774c0cd029d3c717c9f1468db05, historical49 unknowns and separate historical537/557 exceptions. V12 zero-native bootstrap dca503946b4a1bfc5a09215b128b5d4d1ea5f3d71333d4a09edcff5271ef7326 is read from the measured public source tree. Its originalNULL field and NO_GO remain sealed, zero is an independent source-order/failure derivation, and no paid ordinal/refund is introduced.

All five original-result, Source/authority, Stop-descendant, original proposal/Core argument hash, eight-budget, serial-entry, duplicate-write/send, delivery absence and lost-ACK UNKNOWN invariants remain mandatory. Repeated fixed-project GET checks/inspections have actual qualified run/call receipt inventories and canonical body agreement within owning budgets; writes and every other mandatory logical effect occur once. Recipients are reviewed loopback fixtures; the prepared patch remains unapplied. Cloud weights/server effort/physical isolation/invoice are unavailable. A final chain witness PASS remains separate from independent issue-wide acceptance and does not itself set issue33_ready or authorize merge.
