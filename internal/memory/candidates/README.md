# Source candidate consumer (#85)

`source-candidates-v2` is a code-owned handler for the existing durable generative
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

This initial consumer creates claims for one original leaf event/document. Its
trusted selector uses the PostgreSQL dependency graph, verifies that the leaf's
canonical ancestry equals the complete durable job manifest, and releases only
that leaf body to the model. Earlier chat events remain privacy dependencies.
Multiple unrelated roots receive explicit skips. Model-derived/legacy sources
and assistant messages cannot generate recursive claims. Every accepted claim's
original source and every result derivative retain the full ancestor graph. A source correction,
revocation or deletion invalidates accepted retrieval/context through the existing
catalog guards; deletion redacts the original and its candidate derivatives.

The claim dedup fingerprint uses version, original revision and exact citation
anchor, independent of model topic/type, job/policy IDs and retry timestamps. A
changed interpretation on the same anchor fails the existing request-hash CAS.
Repeated
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
`TestCandidateAcceptedPreferenceScopedOrdinaryContextIntegration` additionally
crosses owner HTTP extraction/acceptance and the ordinary Orchestrator `Execute`
path with real leases, preparation and the production Profile adapter. The exact
quote is absent before acceptance, appears only in its source session after
acceptance, and disappears from fresh contexts after correction/revoke. Its
controlled provider records actual prepared inputs without echoing the preference
into transcript. This proves scoped context composition, not native answer quality.
It retains all attempts/reservations in a separate persistent test database.
The fixed24-case Russian corpus and95% semantic/100% citation/zero unsafe, authority
or unknown-usage criteria are frozen before inference. Failures do not lower them.
Version2 clarifies quoted applicability and validity metadata using general rules.
Its fresh window retains the original24 cases and adds an independently frozen
eight-case temporal/project/ambiguity holdout requiring100% semantic matches.
All older attempts, unknown usage and conservative reservations remain durable.
After a passing screen, a separate zero-model-call native owner stage accepts one
actual extracted candidate through HTTP, verifies catalog eligibility, corrects
the source and verifies invalidation, then explicitly revokes that synthetic
source. These intentional owner writes are separate from the extraction screen.
This finite screen does not establish broad memory quality or final task success.
`TestCandidateTranscriptAndParsedSourcePreserveAncestorPrivacyIntegration` proves
actual transcript registration, historical privacy parents and a parsed original
document select the correct leaf, preserve scope and invalidate accepted context
after an ancestor correction. `TestCandidateLifecyclePoisonRemainsDataAndDeletionSelectiveIntegration`
uses all eight shared #83 attacks; `TestCandidateAtomicConsumerRollbackIntegration`
proves claim/result checkpoint atomicity, retaining the paid failed observation.
