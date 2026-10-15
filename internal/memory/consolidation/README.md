# Owner-reviewed memory consolidation

`memory-consolidation-v1` prepares immutable DATA views of a bounded exact
evidence set. It preserves all unique complete evidence, associates repeats and
keeps differences unresolved. Model output can only group existing unit IDs;
trusted code retains provenance and current typed acceptance/temporal semantics.
It never rewrites accepted facts, owner preferences or task/action authority.

The [architecture contract](../../../docs/architecture/memory-consolidation.md)
defines finite explicit cross-session context pins, shared budget/transport,
version/diff review, owner activation CAS/rollback, retirement and deletion fences.
Implementation and consumer qualification are in progress. `ReleaseQualified`
is false; no production activation or native quality result is claimed.
