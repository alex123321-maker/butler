# Independent renewed-five prepared/auth-only gate, 2026-10-15

Decision: **PASS_AUTH_PREFLIGHT_ONLY_0_INFER; NO_NATIVE_ENTRY_GRANTED_HERE**.
This follows source-readiness document SHA
`61ce41c9624a84962877f3018c39e5925984413f68a754b82296e4c3c569ed7b`.
Reviewer `/root/memory_independent_review` only read safe synthetic originals,
public approval material and metadata. No auth, Infer, private key/import,
encrypted setting or PG mutation was performed by the reviewer.

## Exact prepared root

Root: `/srv/storage/butler/agents/memory-eval-v2-renewed-five-20261015`.
Release `a3d66736dcc5e7ebf94a8b1d614b0e538f387eb8`; source-path
`20261015-142056-119`. Independently recomputed source TreeHash:
`d9824a33875f194085673f46aede24eea593f46ef990b8b9ea0a0855091d22f7`,
2590 files. Both auth-entered and entered were absent and private had no files
when checked.

The independently read five prepared files all match prepared.sha256
(`8f4bf6529c6ee46941f1cf4816b085f0eac32fd477253f360c4f7a1f6e4b8dc9`):

| File | SHA256 |
| --- | --- |
| build/collector.test | 6bf79d67bf279dee49230538676cbbdd53595bd4f651bd973b94df9936f75980 |
| build/memory-eval | 8b0d31d1751a5909b02dad64892131d6f81c091e62ebd9649780b39198d39362 |
| metadata/pins.json | e84b4f6eaa356505dcb1be4bf90692cd8920f05ca5c2b6bde68ad8434181a7d0 |
| review/approval.json | 8e505403891120577b38e2fcab987bf0a8b10513ed9d4c5e3edf5567467e13cc |
| review/public-key.hex | 3307a5cd20a691cfea766ac20933e649a148f5636f3789428ee40d2eabe90e78 |

Hardware file hash independently matches pins:
`7bd913027f306eda7adf21c009ec6709ed87078da72693c0924c6b0e4e573b3f`.
It names immutable Go/PG image IDs previously reviewed. The public Ed25519
approval was independently verified in memory against the prepared public key,
suite7c74abe1 and criteria9be3b4b5. Its reviewer and candidate-author are distinct.
This verifies the operator seal's bytes, not external ownership of a signing
identity. No private signing key was read.

## Exact immutable parent and five-input mask

All five prior ledger/frozen/measurement triples independently match their pins,
closed SQL exports, source/commit identities and original expiry. Each retained
comparison equals the accumulation from its original ancestors. The safe
filtered snapshot has662 files, only the reviewed filename whitelist and at
most five levels; no prohibited key/import/setting filename was present.

The retained state remains54 completed, five failed and eight skipped,
59 paid native plus53 auxiliary admissions, 937 expenses and391 logical stage
calls. Latest carry112/3212748; original expiry
2026-10-14T20:16:58.345536Z is unchanged. Every old observed slot stays excluded
from execution only; the original scored72 freeze stays intact.

Independent compact-JSON recomputation:

- parent digest:
  `f1c9e5d8f7f0f346750c8d2e7de2d93d424603781b3006abe48c63e25f5026d7`;
- ordered remaining-input digest:
  `62f20cca0b7e1a3f0dc9c6164073a8983115e99a40db01b6c8379ce0270ee140`.

The only five future model inputs are h11/full_context, h11/retrieval,
h12/no_ltm, h12/full_context and h12/retrieval. Independent reconstruction of
the exact unchanged model projection/native schema byte reservation gives
46061+45881+45315+46314+46108=229679 units. No skip or new auxiliary input exists.
Maximum new native admissions is5; if all are admitted, aggregate117/3442427.
The generic hardware ceiling66 is not the binding new five-input allowance.

The prospective grant is issued2026-10-15T07:25:18.766787Z, expires
2026-10-15T07:55:18.766787Z, exactly30 minutes, both microsecond-aligned.
It binds this root/source/binary, immutable parent and fixed input digest.
It does not alter any old deadline or permit a paid retry.

## Capacity and concrete auth-only scope

metadata/space-pre-auth.txt hash
`e7ce5e121d86f9bcaffe0cbeb7edff7fedad9c20e30becad182fdba4f8895eb2`
records843915264B available on root and727228416B on storage. Fresh independent
df reads843943936B root/727220224B storage, above256MiB/512MiB thresholds.

One auth-preflight action is admitted under the new grant, at most three safe
control-plane probes and zero Infer. The compiled gate must verify actual
auth-execution proof before private key/import: exact image/anchored auth test
command, UID10001,1CPU/512MiB/64PIDs, read-only rootfs and exact inputs, only
private writable, exact new-root+backend networks, required proxy/local bypass.
These are enforced prospective conditions; no auth container existed at this
prepared review, so its actual execution is not falsely attested here.

The preflight must use its fresh owned memory_auth_test encrypted PG store and
preserve refresh rotation before discovery. Failed/uncertain refresh or
retention stops further work. Every admitted probe needs its immutable safe SQL
admission/observation, exclusive original and actual expense, including failures.
Native execution has no authorization from this document.

After auth, independently check safe receipt/ledger/probes/expenses, actual
container proof, durable shared-store alignment and fresh disk headroom before
a separate concrete five-input native gate. Keep all old usage/cause unknowns
and every failed original. Full harness acceptance, numeric quality/cost NO_GO,
promotion and broader task effects remain separate.
