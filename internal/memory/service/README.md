# Memory bundle service

Automatic document context requires lexical term/prefix overlap, diagnosing
unrelated omissions. This precision choice can omit paraphrases; explicit
semantic search retains recall. Stored documents, profile preferences and typed
constraints are unchanged.


The production composition passes the existing metrics registry. Additive
operation/duration/item/limitation metrics distinguish search from selection and
finish each started stage once, including errors and cancellation. Reasons use
fixed vocabularies; queries/content/scope identities never become labels. The
older selected-class hit/miss counters retain their meaning. Zero document hits
do not establish an empty corpus. See the
[operator runbook](../../../docs/operations/memory-observability.md).

The service assembles current summary/working state and optional Profile,
Episodic and document evidence. PostgreSQL remains authoritative; source access
is enforced by the composed catalog adapters/reader and provider release guards.

`BundleRequest.QueryPolicy` separates local use of the query from external query
release. `{Local:true, External:false}` uses the policy-aware semantic reader,
including local SQL exact/full-text search, and never calls the legacy embedding
provider. Unproven/unavailable local inference is an explicit diagnostic; genuine
source/authority failures are errors. A nil policy preserves old explicit
`IncludeQuery` callers. Query permission does not grant generation/data release.

`memory-context-v3` ranks eligible confirmed profiles by lexical query overlap,
then scope order/key/text. Stable presentation/identity keys remain applicable
without literal query overlap. Eligibility and current access are decided before
ranking; zero-overlap authorized entries remain lower-priority candidates inside
the same profile count/size quota. Rejected sources cannot fill a quota. Profile selection happens after
potentially slow document retrieval. The product default is four document hits,
within the existing bounded hybrid reader, rather than two.

Selection keeps complete summary/working state first. Optional evidence has an
8192 conservative UTF-8 JSON byte allowance, including item/section overhead,
inside the complete context allowance (65536 by default, supplied by Orchestrator).
The smaller optional quota never rejects mandatory history that fits the complete
allowance. These units are not a native tokenizer or billed tokens. Oversized
mandatory state fails explicitly.
Optional documents/profile/episodes are admitted in deterministic round-robin
order; when documents exist, each other class can consume at most one third of
the optional allowance. Oversized optional records are omitted whole. Deduplication
uses profile scope/key/value/provenance or document source/revision/title/excerpt/
provenance; equal values across different keys, sources or classes are retained.
Unknown document identity is not deduplicated by text. Unspent space remains available.
`BundleBudget` is a secondary count cap, never a substitute for size accounting.
Profile source handles and document revisions stay with selected evidence.

Profile text formatting retains `scope_type`, `scope_id` and source handles for
each selected record, including after a JSON round trip. Missing scope stays
explicitly unknown. Scope labels are data: they neither resolve contradictory
claims nor grant authority. The shared formatter is also used by the Orchestrator.
Its model-facing profile section preserves complete records, including trailing
exceptions; complete-context admission omits optional records whole when needed.

`BundleRequest.ProfileTaskGoal` is a caller-supplied current typed task goal used
only to break ties in current-message profile relevance. Empty keeps
`memory-context-v3`. The Orchestrator supplies it only for an explicit active task
whose ID, owner, session and live authority were checked, and rechecks the task
revision, constraints, goal and release policy after retrieval. Conversation,
preview and callers without the authority reader keep the empty hint. It does
not add scopes, change eligibility or send the goal to embedding/search backends.
The nonempty diagnostic variant is `memory-context-v3-task-goal-tiebreak-experiment-v1`.
This diagnostic name retains the experimental suffix for continuity with frozen
observations. The [factorial comparison](../../../docs/testing/issue-76-context-factorial-results-20261023.md)
supports a limited selection improvement; it is not universal model safety or
deployment evidence. Preference-application prompt guidance remains dormant.

Safe diagnostics expose version/estimator/allowance/counts and omission reason
codes, never query or source text. The orchestrator additionally admits the
complete encoded context, reserves continuation headroom and trims optional
items individually before rejecting mandatory overflow. This is bounded lexical
selection; adaptive multi-step search and learned policies remain separate work.
