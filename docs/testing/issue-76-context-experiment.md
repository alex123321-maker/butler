# Issue76 prospective context comparison v1

Fixed hypothesis (2026-10-23): current-message lexical relevance remains the
primary profile rank. Candidate breaks ties with relevance to the current typed
task goal before existing scope/key/text ordering. Baseline uses
`memory-context-v3`; both arms include complete preference formatting from
`a9cea901`. Empty experimental `ProfileTaskGoal` preserves production behavior;
ordinary Orchestrator callers do not supply it. No automatic promotion is allowed.

Use four development and twelve independently authored control cases. Freeze
corpus, instructions/schema, code, executable and every projected input before
native entry. Public synthetic controls are independent authorship, not a secret
benchmark. Do not tune the candidate, prompt or rubric after observing controls
or outcomes. The separate reviewer owns semantic judgments and records ambiguity.

Both arms use the real memory bundle selector/formatter, identical limits
(profile20, bundle12, chunk4, optional8192 conservative JSON byte units,
complete65536). Input profiles are explicitly accepted/authorized fixtures;
document candidates are fixed, not a measured retrieval index. Neither arm tests
claim acceptance, source revocation, embedding search or PostgreSQL memory
recovery. Current goal/constraints/question remain fully visible in both arms.
Working state is a fixture of that same current goal, never old session state.

Native generation uses the existing Butler Antigravity transport and PostgreSQL
budget/lease/task/attempt collector in a fresh isolated `_test` database.
Model `gemini-3.8-flash-high`, observed version `gemini-3.8-flash-n`, 32768 maximum
generated tokens including thinking, five-minute turn timeout. Fixed serial
schedule: corpus order; baseline/candidate on even-index cases, candidate/baseline
on odd-index cases. Maximum32 calls, four-million reserved units, three-hour
window, no inference retries, repair, fallback, continuation or external tools.
The container gets2 CPUs,1536MiB,128PIDs; private reports are bounded128MiB.
Credential import is a read-only file reference resolved by existing auth code.
No credential bodies, reasoning traces or raw provider errors are exported.

Record selected source handles, omitted counts/reasons, exact rendered inputs,
nullable actual tokens, latency, exact output bytes and original failed attempts.
No expected answers, selection targets, split/group labels or rubrics reach the
answering model. Unavailable entries remain in scheduled denominators; uncertain
usage is unknown. Monetary cost is unmeasured, not inferred from token counts.

## Evaluation fixed before execution

- Context recall: required source handles present in selected records, separately
  from semantic sufficiency; compare paired wins/losses/equality per case/group.
- Independent answer review: correct/partial/incorrect/unresolved both given the
  actual input and given the complete authorized fixture; explain retrieval
  omissions separately from answer errors.
- Preference application: scope, current exception, uncertain/derived preferences,
  necessary and unnecessary clarification, action proposal and profile update.
- Exact structured answers are checked for syntax and source visibility. They
  are observations of proposed behavior, never proof of an executed effect.
- Any proposed violation of a current prohibition, authority, privacy or scope
  is a critical semantic regression. Preserve it and reject promotion. Otherwise
  describe improvements/regressions without claiming statistical superiority.

Runtime/integrity/authentication/retention failure stops the window. Availability
failure may proceed to the next predeclared slot only through the existing
collector's safe-availability classification; no failed slot is resampled.
All results, including a negative or incomplete comparison, are retained.
