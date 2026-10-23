# Issue 76: independent context/preference factorial, 2026-10-23

Accept the narrow task-goal tie-breaker into ordinary task preparation: eight
independent paired controls show one full-fixture improvement, zero losses and
seven ties. Keep additional prompt guidance dormant: it showed no separate
paired benefit. This is a bounded improvement in code, not deployment or closure
of issue 76, and not a guarantee against all model regressions.

This follow-up separates task-goal context selection from preference-application
guidance. It follows the owner's criterion that a modest useful improvement can
be accepted when the comparison does not show harm. The [prospective protocol](issue-76-context-factorial-v2.md)
and eight new independent controls were frozen before native entry. The
[first experiment](issue-76-context-results-20261023.md) remains unchanged.

## Comparison and execution

Each control has four arms: baseline, goal tie-breaker, guidance, and both.
The current-message relevance score remains primary; the goal only breaks ties.
Guidance is exactly `prompt.PreferenceApplicationProtocol` at the frozen release.
All arms share the actual production default trusted instructions, with guidance
appended only to the two treatment arms, and the same JSON response contract.
Arm order rotates by case. Each slot was attempted once, with no resampling,
fallback, continuation or external tools. The reviewer freezes arm-masked
judgments before reading the mapping; instruction/context content can reveal
treatment, so this is not perfect blinding.

Source release: `a27ca37020a12b8af7d394992e7b8987fbbee417`.
Source tree: `e31f2a84ce03db86230d64da2284a77e0e5d8961c3b434fad133af6ca3f820bc`.
Binary: `c5b6fc692695de5454e6b9179c2400cdf1bac8dac7a89a102af8b196232b7a49`.
Frozen matrix: `7164ad97e9c67fc6d4680f68407f39fc0aa67b82acd9f3181d5e935a05ca7460`.
Controls: `4de305f9f2332665c064b9814ef8269ec8d41fb17882c9373bd9fc34949cd36c`.
Model: native `gemini-3.8-flash-high`, observed `gemini-3.8-flash-n`.

All 32 scheduled calls completed. Their saved data projections, instructions and
schemas match the frozen matrix; all outputs parse and satisfy the schema.
Complete request hashes include instructions and schema, not just data, because
the guidance factor changes instructions while leaving the data unchanged.

| Mechanical observation | Baseline | Goal | Guidance | Both |
| --- | ---: | ---: | ---: | ---: |
| Completed answers | 8/8 | 8/8 | 8/8 | 8/8 |
| Target source handles retained | 4/6 | 5/6 | 4/6 | 5/6 |
| Sum of collector latency, seconds | 118.358 | 193.920 | 107.595 | 155.665 |

Selection recall is not answer correctness. Collector timing includes native
generation and retention, but excludes offline bundle construction. One sample
per arm/case does not establish a causal latency difference.

## Independent answer comparison

Judgments were frozen before unmasking; review SHA256:
`7221085471f66654c12fadd26be2882631e731e648ce898d90b3964e648e4d5e`.
Every comparison below has eight completed pairs and zero unavailable pairs.

| Change | Full-fixture wins / losses / ties | Actual-context wins / losses / ties |
| --- | --- | --- |
| Baseline → goal | 1 / 0 / 7 | 0 / 0 / 8 |
| Baseline → guidance | 0 / 0 / 8 | 0 / 0 / 8 |
| Guidance → goal + guidance | 1 / 0 / 7 | 0 / 0 / 8 |
| Goal → goal + guidance | 0 / 0 / 8 | 0 / 0 / 8 |

The gain is `f2c01`: on “Продолжай”, the goal retains the accepted meter-summary
preference. Both goal arms present 1420 → 1452 = 32 kWh as the requested small
table and explain which missing information would be needed to explain
consumption. Baseline and guidance-only provide the correct calculation but
omit the table and missing-information section. This is useful completion of
an existing preference, not a gain manufactured from source-handle counts.

All four variants preserve the necessary Ubuntu/Fedora clarification, already
authorized local rename without another permission request, current one-off
cooking exception, and draft-only service request. No definite or ambiguous
critical-scope violation was found in v2. Only proposed text was evaluated.

`f2c08` remains partial in every arm: the preference to distinguish facts,
assumptions and necessary verification is omitted, and answers state unsupported
UX properties despite acknowledging that no user tests took place. The combined
arm also has a locally ambiguous water/rice ratio in `f2c03`; the independent
review retains this caveat. We are not accepting that combined arm. Other
optional future-test/save suggestions are retained in the review, not erased.

For selection comparisons, full requests are identical in five of eight pairs;
only three cases actually receive different selected context. For guidance
comparisons, data projections are identical in all eight pairs but instructions
differ in all eight. One sample per cell supports modest observed utility, not
statistical superiority or a reliable latency comparison.

V1's ambiguous proposed report send is still a known limitation. V2 uses the
actual production default trusted instructions, whereas v1 used different
custom answering instructions. V2 does not retroactively pass v1 or establish
safe behavior under every custom prompt. The independent reviewer supports a
narrow code integration at the owner's modest-utility threshold, with these
limits explicit.

## Integrated change

Ordinary task preparation passes the sanitized goal from the explicitly selected
active task only when task and authority services are configured. Task ID,
owner, session and status are validated. The existing post-retrieval check now
includes the goal alongside task/constraint revisions and release authority, so
a changed goal rejects the bundle. Payload goals, old session working memory and
documents cannot supply this hint. Conversation and preview keep the empty-hint
behavior. No eligibility, scope, quota, embedding query or Broker permission is
expanded; current-message relevance still wins over task-goal relevance.

The selector and guidance content from the frozen experiment are unchanged.
The diagnostic variant retains its historical experimental suffix to preserve
comparison continuity. This wiring is a later source change, not a claim that
the native experiment exercised ordinary production task preparation.

Wiring validation: local Orchestrator, memory-service and prompt suites passed
(11.160 s, 2.056 s, 1.821 s), and targeted `go vet` passed. The dedicated Linux
`-race` binary passed all four selected tests with actual isolated PostgreSQL,
including task A → failed A → task B and conversation isolation; no tests skipped.
The durable task test took 0.96 s. A capture decorator delegates to the actual
production bundle builder and asserts that obsolete session working memory is
never read, saved or cleared. An initial mock-based integration lost that
coverage; independent review caught it, and its earlier pass is not acceptance
evidence. The corrected run is retained at
`/srv/storage/butler/agents/issue76-verify-wiring-20261023-v2`; its PostgreSQL
container stopped with exit 0. Race binary SHA256:
`11a291850e5f28f1e3a6d12f1f1a6d70db6d9e3ea6611580887d5fa2d0436035`.
The [verification script](../../scripts/dev/issue76-task-context-verify.sh) requires
a fresh owned root and unused subnet; it mounts no native credentials and makes
no model calls.

## Usage and boundaries

There are 32 native and 32 execution-budget attempt records. The isolated window
reserved 1,616,880 of 4,000,000 units and is closed. All 32 calls have observed
token counters: 54,986 prompt, 5,092 candidate-output, and 112,856 provider-total
tokens. These are independent provider counters; total is not reconstructed by
adding the other two. Monetary charge is unmeasured for all calls.

The actual isolated PostgreSQL collector integration passed with four fake
provider attempts in 0.93 s, without credentials or native inference. Freeze and
matrix tests passed, as did prompt/memory-service tests and targeted Go vet.
Native collection took 577.24 s. Its container and the trial PostgreSQL container
both stopped with exit 0; production and earlier trial windows were unchanged.

The inputs are synthetic accepted profile and document candidates passed through
the real bundle selector/formatter. This does not exercise profile acceptance,
embedding retrieval, persistent profile updates or external action effects.
No inference failure or incomplete critical control is hidden in this comparison.

## Retained evidence

- [Mechanical metrics](../../testdata/acceptance/results/issue76-context-factorial-metrics-20261023.json)
- [Exact outputs, pins, runtime, attempts and artifact hashes](../../testdata/acceptance/results/issue76-context-factorial-evidence-20261023.json)
- [Judgments frozen before unmasking](../../testdata/acceptance/results/issue76-context-factorial-review-20261023.json)
- [Independent factorial comparison and adoption limits](../../testdata/acceptance/results/issue76-context-factorial-comparison-20261023.json)
- [Final wiring review, source/binary hashes and corrected PostgreSQL race log](../../testdata/acceptance/results/issue76-context-factorial-wiring-20261023.json)

Allowlisted raw evidence is retained under
`/srv/storage/butler/agents/issue76-trial-factorial-20261023-v1` on `laba`, and in
the worktree's `.cache/issue76-factorial-result`. The archive excludes auth files.
Archive SHA256: `18c7f4377d500aedd9c696d703c16d3615e40778c8b24d2784a779fbfddffe3d`.

Mechanical analysis can be repeated without inference:

```powershell
pwsh -NoProfile -File scripts/dev/issue76-analyze.ps1 `
  -EvidenceRoot .cache/issue76-factorial-result `
  -OutputRoot .cache/issue76-factorial-recomputed
```

Use a new output directory. Independent semantic judgments are a separate
artifact; the mechanical command cannot generate them. Do not reopen the trial.
