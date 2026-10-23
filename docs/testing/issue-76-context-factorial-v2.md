# Issue76 prospective factorial comparison v2

The owner requested continued improvement after v1, accepting a modest useful
change if it does not worsen existing behavior. V1 remains a closed diagnostic;
its controls and answers are exposed development evidence, not fresh holdouts.

Fixed before reading the new independently authored controls: compare the
existing selector and task-goal tie-breaker, each with and without the exact
experimental `prompt.PreferenceApplicationProtocol`. The new guidance preserves
scope and one-off exceptions, distinguishes a requested proposal from later
execution, avoids unnecessary confirmation without bypassing approval, and
forbids inferring facts/authority from preference compliance. It is dormant in
production `TrustedInstructions` pending comparison. The goal selector is
unchanged from v1 and also remains opt-in.

Eight new independent controls, four arms per case, 32 prospective calls total.
Arm order rotates by case index through baseline, goal, guidance, goal_guidance.
Each is observed once; failures remain failures without resampling. Same actual
production `DefaultBasePrompt`/`TrustedInstructions` and structured-answer
contract in every arm, plus the candidate guidance only in its two arms. Thus
v2 is not a direct numerical continuation of v1's custom answering instructions.
No expected labels, arms or rubrics enter the answering projection.

Both factors are evaluated separately. Compare baseline/goal, baseline/guidance,
guidance/goal_guidance, and goal/goal_guidance on completed pairs; report interactions and every
unavailable pair. Complete-fixture and actual-context answer verdicts remain
separate from selection recall and source attribution. Answer text is a proposal,
never executed-effect evidence. A reviewer freezes judgments with arm labels
masked before unmasking. Context/instruction content can reveal treatment.

A small independently judged paired improvement can justify accepting a narrow
change; statistical superiority is not required. Do not accept an arm with an
observed paired semantic loss, newly ambiguous/violating critical scope, or
regression on necessary clarification or authorized initiative. Missing critical
controls remain unverified, not passes. If only one factor meets these criteria,
retain only that factor as a candidate for normal code review; no automatic
deployment or issue closure. No candidate, prompt or rubric tuning within v2.

Freeze exact source, executable, protocol, corpus, schema and all inputs before
native entry. Use a new isolated trial/database and an unused explicit subnet.
Reuse the existing real budget/task/lease collector, native Antigravity
`gemini-3.8-flash-high` (observed `gemini-3.8-flash-n`), 32768 output-token cap,
five-minute per-call timeout, 32-call/four-million-unit/three-hour window,
2CPU/1536MiB/128PID container and 128MiB report bound. No external tools,
fallback, continuation or inference retries. Credential imports remain read-only
file references. All v1 roots/windows stay unchanged. Actual usage is nullable;
monetary charge remains unmeasured. Fixed profile/document fixtures do not test
memory lifecycle or embedding retrieval. Collector latency excludes offline
bundle selection. Authentication/integrity/retention failures seal the window;
only the existing safe-availability classification permits the next fixed slot.
