# Source candidate consumer (#85)

`source-candidates-v1` is a code-owned handler for the existing durable generative
memory worker. It uses its configured subscription transport, finite owner policy,
admission, reservation, current source/access/task checks and atomic publication.
It never calls the legacy extraction callers or local research models.

The first schema supports explicit preferences, scoped facts and outcome episodes.
Assertion and negation stay distinct. Quotations, hypotheses, conditions, one-time
requests, noise and ambiguous material receive an explicit skip. A model decides
semantic usefulness; a valid exact quote alone does not prove that decision.
Candidates have confidence0 and never create mandates or task state. Preferences
and facts project to Profile Memory only after the existing owner decision API;
outcome episodes project to Episodic Memory, with no fabricated vector or dates.

The complete quote is the value and summary. The parser rejects invented values,
extra/duplicate JSON keys, absent source accounting, ambiguous quote occurrences,
mid-sentence/truncated citations and recognized credentials. Unicode spans use
zero-based rune offsets with an exclusive end. Trusted code supplies original
revision/hash, actual source scope, reported event time and ingestion time.
`memory_claims.created_at`, exposed as `extracted_at`, is the extraction time.
Unknown event/validity fields stay null. Quoted scope and validity remain text;
relative dates are never silently converted into absolute timestamps.

This initial consumer creates claims only for a single original source with no
additional source in the policy manifest. Composite/multiple-source input receives
only explicit skips, avoiding a claim that loses cross-source dependencies.
Model-derived and legacy sources cannot generate recursive claims. Each derivative
still retains the complete generic worker parent graph. A source correction,
revocation or deletion invalidates accepted retrieval/context through the existing
catalog guards; deletion redacts the original and its candidate derivatives.

The claim dedup fingerprint uses version, immutable original revision and complete
canonical evidence, independent of job/policy IDs and retry timestamps. Repeated
events, policies and process restarts retain the original candidate/owner decision.
The optional publication hook writes candidates and job/result pointer in one
transaction; failure rolls all of them back. Paid observations remain durable.

Production registry needs `BUTLER_MEMORY_MODEL_CANDIDATES_ENABLED=true` plus the
code-owned qualified release gate. That gate currently remains **false**. The
native quality report and combined #83 lifecycle gate must pass before a reviewed
release changes it; an environment flag cannot promote this unqualified version.
Local-only policies remain visibly `local_backend_unavailable`, with no cloud
attempt/reservation or fallback. Existing `memory.propose_candidate` is separate.

Apply migration073 with the normal migrator. It adds typed claim projection and
allows a confirmed exact episode without an embedding. Its down migration refuses
typed episode history or null-vector episodes; preserve history on rollback.

Controlled tests are `TestCandidateHTTPWorkerOwnerDecisionCorrectionIntegration`
and `TestCandidateLateSourceChangeAndLocalOnlyIntegration` in the Orchestrator API
package. They cross real HTTP owner controls, durable policies/claims, worker,
controlled transport, reservations, catalog decisions and real PostgreSQL with
MaxConns2. The native opt-in `TestCandidateNativeRussianQuality` uses the same
consumer and HTTP path with actual High transport and the pinned server proxy.
It retains all attempts/reservations in a separate persistent test database.
The fixed24-case Russian corpus and95% semantic/100% citation/zero unsafe, authority
or unknown-usage criteria are frozen before inference. Failures do not lower them.
This finite screen does not establish broad memory quality or final task success.
