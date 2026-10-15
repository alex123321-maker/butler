# Memory bundle service

The service assembles current summary/working state and optional Profile,
Episodic and document evidence. PostgreSQL remains authoritative; source access
is enforced by the composed catalog adapters/reader and provider release guards.

`BundleRequest.QueryPolicy` separates local use of the query from external query
release. `{Local:true, External:false}` uses the policy-aware semantic reader,
including local SQL exact/full-text search, and never calls the legacy embedding
provider. Unproven/unavailable local inference is an explicit diagnostic; genuine
source/authority failures are errors. A nil policy preserves old explicit
`IncludeQuery` callers. Query permission does not grant generation/data release.

`memory-context-v2` ranks eligible confirmed profiles by lexical query overlap,
then scope order/key/text. Stable presentation/identity keys remain applicable
without literal query overlap. Eligibility and current access are decided before
ranking; rejected sources cannot fill a quota. Profile selection happens after
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
the optional allowance. Oversized optional records are omitted whole, duplicate text is
removed without changing case/punctuation, and unspent space remains available.
`BundleBudget` is a secondary count cap, never a substitute for size accounting.
Profile source handles and document revisions stay with selected evidence.

Safe diagnostics expose version/estimator/allowance/counts and omission reason
codes, never query or source text. The orchestrator additionally admits the
complete encoded context, reserves continuation headroom and trims optional
items individually before rejecting mandatory overflow. This is bounded lexical
selection; adaptive multi-step search and learned policies remain separate work.
