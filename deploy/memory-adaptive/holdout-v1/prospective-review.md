# Independent prospective review

Reviewer: `/root/candidates_85`, independent of corpus author `/root` and adaptive
policy author `/root/lifecycle_83`. Review completed before native inference.
This is a fallible agent review of synthetic fixtures, not a human label or an
actual runtime acceptance result.

Reviewed final committed LF bytes (`00b55508`):

| File | SHA-256 |
| --- | --- |
| corpus.json | `9b591b2e566c713773b721c7cfbc6761118740b0babe00c47a0569550f71a5fb` |
| controls.json | `f7e9d513e5e987da1c64bd43387ed477f276577c6ca683a06358111580172ac2` |

Initial review identified two rubric defects: e01 incorrectly forbade a true
clarification about the other document; u06 required the answering model to know
the exact permission-denial cause even if the actual read only returned no data.
Both were corrected before collection. e01 now forbids assigning the wrong
cooling mode to ТЕХ-641; u06 permits calibrated missing-data/access uncertainty
without guessing the reason or revealing the inaccessible number.

Final review PASS: no remaining corpus/protocol blocker found. The reviewer
checked all32 questions and rubrics, explicit UTC import/event dates, real project
grant controls, content-free tombstones, and the prospective three-arm96-slot
matrix. The fixed24 answerable/32 scheduled denominator plus all8 self-contained
must-answer cases prevent a blanket-refusal adoption result. There is no paid
oracle arm; uncertainty review uses the permitted complete corpus. Scope controls
must still be enforced by the actual runtime, not prompt filtering.

Offline Linux race checks on `ssh laba`, source `00b55508`, passed38 test/subtest
events,0 failed,0 skipped, plus vet. They cover old semantic review/CLI invariants,
exact current-question witnesses, explicit complete adaptive arm matrices and
corpus/control consistency. Retained test stream SHA-256:
`400f1ae16cf16c014af62cf52108eb4f91a9b9a70a82e91fe4d0a05a529358e2`
at issue87-eval `reports/corpus/tests.jsonl`.

These checks do not authorize a native window or prove source permissions,
measured benefit, production readiness, or the finite allowance. Freeze actual
policy/binary/runtime/arm order and admit the explicit allowance separately.
