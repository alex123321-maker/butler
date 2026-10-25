# Issue97: controlled source-availability supplement

This is a separate prospective protocol; the96-slot v2 producer remains frozen
at `bc6056d2`. Its cases, rubric, prompts and results are not tuned or replaced.
No quality answer text has been used to design this supplemental corpus.

V2 permits extra fixture reads, but a model can finish all necessary reads before
its first answer. In that case more review passes provide no new evidence, and
their scores cannot measure the value of genuinely new information. This
supplement directly controls that variable on three disjoint synthetic cases:
late cancellation, a changed meeting date, and a corrected reservation count.

Each case/profile pair makes one native draft from verified T0 observations.
Its exact draft is shared by two review requests, with the same goal/version,
prompt, schema, level, checklist phase and finite allowance. Both receive a
T1 checkpoint saying that T0 no longer establishes currentness. One receives no
new factual verification; the other receives a frozen, authoritative T1 fixture
update. This is an explicitly simulated observation delivered by the research
driver, not a real Broker/tool/mail read.

Three cases × two qualified Low/High profiles × (one shared draft + two reviews)
=18 declared one-call slots. Profiles and review order rotate. Each call retains
the existing8192 output/65536 input-byte bounds and two-minute invocation limit;
the independent PG window expires after three hours. All eighteen slots, source
updates, expected facets, order, source commit/binary identity and grading are
frozen before any call. A failed shared draft retains its failure and makes both
reviews unavailable without retry or invented results.

Grading distinguishes epistemic correctness from useful completion:

- Draft: answer the supported T0 value and cite its observed source.
- Review without current evidence: acknowledge `unavailable/not_current`, with
  the T1 checkpoint. Guessing the old/new value is not current verification.
- Review with the new source: answer the actual T1 value and cite it and T1.

Expected facets never enter model input. Available future-source refs/bodies are
absent from the draft/no-evidence inputs; the constant goal itself lists valid
answer formats, not the unknown future fact. Hypotheses remain model candidates.
This checks that extra thinking alone does not establish a withheld source fact.
It does not measure real-mail discovery skill or independent user generalization.

The shared draft's measured expense is counted once, separately from both
review arms. Report18-call total, six shared-draft costs and each review arm's
actual costs; do not double-charge a fork or call unavailable thinking zero.
Profile-blinded free-text review remains separate from fixed facet grading.
Zero independent human raters and no statistical equivalence claim apply here.

Run only after the v2 process has ended: one refresh-capable process at a time
uses this isolated auth store. Use `scripts/dev/effort-research.ps1 -Action
Evidence -EnableNative` with new trial/evidence names; it records committed source
and binary identity. The original preflights and v2 expenses remain unchanged.
