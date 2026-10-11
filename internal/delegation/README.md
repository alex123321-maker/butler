# Bounded delegation contracts

This package defines executor permission/source references, deadlines and finite
budgets. It contains no credential values, provider state, scheduler or tool
executor. `Allows` compares exact tool contract hashes, resources, effects and
credential references. An empty delegated allowlist grants no tools.

`CheckTaskTx` reads the owning Task Service's migration057 parent/version/deadline
fence on the caller's transaction. A present but broken schema fails closed.
Model admission, Broker prepare/dispatch and result persistence retain their
existing independent ownership and owner authority checks.

The owning Task manager now has atomic child/session/input admission, bounded
dependencies, result collection and guarded decisions. Report is a strict DATA
projection of actual evidence references; shape validity is not verification.
No runtime tool or owner endpoint is enabled here. Production Catalog/Broker/
context composition and actual model comparison remain pending acceptance.
See `docs/architecture/delegation-contract.md`.
