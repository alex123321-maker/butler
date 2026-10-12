# Supplementary command expectations: independent bounded review

The root agent reviewed the separate corpus author's45-case candidate in
`testdata/acceptance/ru-command-proposals-v1.json`. Exact raw file SHA256:
`2facbb0d7eb5fc6829b748bd11fe62f6e81086c6ede25b6523b54a649703d379`.
This review covers expected proposals and original-input witnesses only. It
does not approve model quality, execution or a protected holdout release.

All36 public owner messages were read independently. Eighteen explicit stop or
prepare-only instructions propose the corresponding existing `task.control`
kind. The remaining public cases clarify rather than elevate external DATA,
guess a target among two current tasks, or use an unknown schema. Nine
secret-bearing cases are excluded before model-input export and scoring; they
are not successful model answers. The secret bodies were not exported for
this review.

An independent exact-field check found zero mismatches in proposed task ID,
current task revision, full immutable owner instruction, source ID, source
revision and full quoted original. Foreign/external originals, guessed or stale
revisions, invented fields and broadened execution policy must be rejected by
the programmable checker. This is an observation protocol; proposals are never
registered or submitted as authenticated owner decisions.

Independent implementation review found that the original exported case names
revealed their expected categories. Export now uses stable opaque case/group
identifiers; the checker maps observations back internally. The original issue
was corrected without changing the expected corpus bytes. Wrong control
arguments and kinds must remain distinct from missed controls and unexpected
actions in reported errors.

The nine topic groups still reuse similar control constructions across declared
development/tuning/holdout splits. The lexical normalization check detects named,
numbered and near text copies, but cannot establish semantic independence. This
supplement is a programmable contract fixture and retains
`CANDIDATE_REVIEW_REQUIRED`; it does not satisfy issue33's independent model
holdout criterion. The previously signed240-example v3 corpus, its instructions,
criteria and external review remain unchanged.

No model ran during this review. Fixture-validation success and unreviewed
observations never set `model_quality_evidence` or `execution_authorized` true.
Protected criteria/signing/runtime deployment and actual quality/cost
measurements remain separate release gates.
