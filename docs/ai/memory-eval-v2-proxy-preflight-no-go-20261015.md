# Independent metadata preflight retention blocker

Decision: **WITHHELD_NATIVE_ENTRY_PENDING_REFRESH_RETENTION** for candidate
`9e7d7e0218973953efb7d19862af5ba4a7fe026d`, exact source snapshot
20261015-021749-570, independently reproduced TreeHash
`9872a2c1f52a4522d5ab897cb0bef5c806f2cb7d295247115d002299ef679f43`.
This is a specific preflight blocker, not a rejection of the retained original
data, proxy correction or three-prefix accounting. No new Infer is admitted.

Reviewer: separately appointed `codex-independent-memory-review-20261014`.
No credential/import/key contents or authentication/Infer call was accessed.
All preceding spent roots remain sealed and all three paid failures remain
eligible. No actual token rotation, credential loss or account failure is
asserted by this finding.

## Concrete code path

scripts/dev/memory-eval-model-metadata/main.go SHA
`e510f04019abc35d225037e00304d18861cc0e08af3e64b121facbe774077d7f`
imports the private reference into a map-backed privateStore and calls
Manager.ResolveAntigravityCloudCode. That calls ResolveAntigravity, which can
refresh an expiring OAuth access token. Its refresh response can contain a new
refresh_token; Manager saves that credential state before returning. The helper's
Set persists only to its process map, then the helper exits. A subsequent native
process imports the original file again. There is no persistent transfer of a
possible rotation to that next process.

The existing Manager code explicitly requires rotation persistence before
another request, because reusing the old refresh token can strand the operator.
credential-management.md:601-603 requires strict encrypted provider-auth storage
and persistence of rotations before inference. The new model-transport text
describes transient metadata state without resolving this refresh path. A lack
of generation in the helper does not remove the OAuth state transition.

The reviewer does not infer that the actual metadata probe rotated a token.
Its safe report has no rotation observation, and no private state was inspected.
This is an executable path and missing retention guarantee, not a diagnosis of
any old HTTP400 or unknown failure.

## Required finite correction

Before entry, either prevent OAuth refresh in this disposable metadata probe
(use already sufficiently valid imported/cached access only, fail closed before
refresh), or retain and supply any updated state through a newly owned strict
encrypted durable store. Existing sealed credential/state stores may not be
mutated or old credentials printed. A diagnostic transient-store exception must
be explicitly aligned with the credential spec and must not discard rotations.
The corrected helper/source must be pinned, checked and reflected in fresh
prepared metadata before an entry gate can pass.

## Scope of already checked positive evidence

The reviewer independently read the 19-pass narrow race proof SHA
`6a40041b0693d9df6c4a64b0361fbbba368d81216049ff8df8001ce33e03e8db`
and three-pass CLI race proof SHA
`c5450e0dacde801c3f92e4d54766651a9e6d2e94213ea0217b07dda6b745e124`.
The source-alignment manifest SHA
`3b56a3bcc06e12dc2e5e345ea06a21533319c38f7f233691c0190a294f84e51e`
honestly distinguishes the earlier narrow snapshot and final shell/review changes.
The final exact shell passes bash-n. Original 72-input scoring, 61 unpaid
schedule, three failed prefixes/236 expenses, original56/660396 carry and
absolute20:16:58.345536Z expiry are structurally consistent in the reviewed code.
The new proxy/bypass requirements and fourth-parent rejection are appropriate.

Metadata report SHA
`3b223d6d8ecb2d94724ea50ac548ab82e75fc18fb5038c21e773650c694a68c9`
exports only the seven declared safe fields: model_calls0, proxy configured,
paid discovery, valid project, access-valid-three-minutes, HTTP200 catalog and
frozen High profile present. It proves that metadata observation, not generation
or absence of refresh/rotation. The helper bytes match the checked snapshots.
Original model inputs/schema/control/criteria remain unchanged. Full comparison,
costs, task outcomes and promotion are still unapproved.
