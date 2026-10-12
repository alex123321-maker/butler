# Independent prospective V2 dataset review

Status: `INDEPENDENT_DATASET_REVIEW_ACCEPTED_FOR_PROSPECTIVE_V2`.
This accepts the meanings and grouping of a new candidate for a separately
frozen experiment. It is not a signed release, model-quality result, native
execution approval, or revision of the failed V1 experiment.

The corpus author is the resources agent; the separate evaluation agent reviewed
the whole file, including all 240 questions/answers and the original documents
in all 30 groups. The final reviewed LF bytes of
`testdata/acceptance/ru-cloud-fresh-v2.json` have SHA256
`467ae4978d2d19eed0cad753c0e6ee61081a4b71ee8a0060be5e67b37a26e67c`.
Author commit: `242cc7190c4b7e914e8b0a9bf58e4b868c5929f8`.
No V2 native calls preceded this review.

| Check | Review result |
| --- | --- |
| Inventory | 240 explicit cases, 30 original source/semantic groups, two cases of each of four kinds per group. No runtime expansion. |
| Parts | 12 development groups/96 cases, eight tuning groups/64 cases, ten holdout groups/80 cases. Groups remain intact. |
| Cloud eligibility | 236 public inputs: 96/64/76. Four document-bearing synthetic SECRET cases are blocked before export. |
| Retrieval | 34 nonempty and 26 empty answers. Supported negative statements remain selectable originals; absent-property requests have no asserted value in any supplied original. |
| Extraction | 52 nonempty and eight absent-field answers. Every selected source is original; each nonempty value equals its exact quote and complete requested field value. Units, punctuation, Cyrillic and signs remain literal. |
| Importance | 30 current blocking, scope, evidence or privacy problems versus 30 repeats, historical or intermediate observations. No answer mismatch was found. |
| Commands | 30 positive classifications and 30 clarification/no-action classifications. The schema is a single-intent classifier; independent compound requests cannot be silently collapsed into one action. `owner_required` is not an owner grant. |
| Frozen comparisons | Development cases q201a, q202c, q203e, q204g: one original of each kind. Selection precedes observations. |

The content review rejected earlier ambiguous empty retrieval answers and
cross-part command scaffolds before any call. The author replaced those queries
with genuinely missing properties, replaced the dictionary-proof episode with
a woven-textile measurement card, and rewrote all 60 commands with concrete
context. The final corpus retains deliberately supported negative contrasts
q207a, q210a, q221b, q222b, q224b and q226b. No answer was changed in response to a
V2 model measurement.

Positive command constructions are confined by part: development has stop,
pause, new prepare-only work, preference correction, budget restriction and
amend-to-prepare-only; tuning has extension revocation, source correction,
privacy restriction, private export, source deletion and scope restriction;
holdout has mandate revocation, initiative suspension, resource restriction,
independent evidence and reconciliation without replay. These are 6/6/5 positive
families, covering the same 17 labels. All parts use the complete fixed schema;
there is no per-part response schema or concealed removal of hard labels.

Clarification mechanisms are also confined: development has unselected targets
or parameters, tuning asks about reported external text, and holdout combines
two independent requests for different objects. Holdout originals introduce
guest-key/calendar, current versus archived river data, glass-negative/XML,
amended boat times, chilled-cargo MIME, embargo recurrence, prepared versus
applied exhibition changes, GS1 weights, radio custody and SECRET ZIP cases.
The review judged actual source/intent constructions, rather than treating
different object names or group IDs as independence proof.

The independent Go parser/export accepted the exact candidate on the isolated
lab, with 236 opaque inputs and four blocked IDs. Exported inputs contain no
golden/group/family metadata, SECRET document IDs or synthetic sentinel.
Mechanical proof:
`/srv/storage/butler/agents/scenario-cloud-observe/reports/candidate-review-20261012T131108Z-1669371`.
The author-side strict/schema/leakage tests separately passed with race/vet,
zero skips/failures and no network or native calls; their proof is recorded in
the authorship document and parent handoff. Lexical normalization is screening,
not the independent content decision above.

The proposed observation protocol pins are:

- semantic corpus: `62768fbcc7f389825946bb1632884c1de5ef835644ae473a68bb1ffc3caca172`;
- kind-generic JSON schema and fixed instructions: `8f4bcb0eb38a854c7307b3a54d36c4deac7779fe30024d216b6c37f14d8bbe4a`;
- unchanged numerical thresholds under the new V2 policy: `d68d1a104c0cf108c9b78c5293ccfe792a92525b88920954ec4b095faa3b4632`;
- new criteria: `280479d286975a09f60e0cee582200d3a36ab11099a86339cdfe0ea6c0556767`;
- fixed client resources: `df078aa7d2a0a0410be62f7259fe49a93ad31f572dc2a002f5639950abc1ab2c`.

An independent external approval must sign these new corpus/criteria identities
before release. The native review helper still denies while its new artifact
constants are empty. Exact source, runtime, hardware, owner window and all 228
previous charges must also be pinned. The old corpus, signature, thresholds,
183-call failed window, partial old holdout and NO_GO verdict remain immutable.

The data are synthetic, public and reviewed on a shared administrator-controlled
host. They are not a blind benchmark. Eight examples in a group are correlated;
the 30 groups do not establish statistical sufficiency or independence for every
abstract language skill. Shared task schemas and general Russian language are
expected. This review supports the bounded prospective corpus gate and preserves
those limits; it does not promise that High will meet its unchanged 90/85/safety0
quality, latency and complete-cost thresholds.
