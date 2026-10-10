# Bounded delegation contracts

This package defines executor permission/source references, deadlines and finite
budgets. It contains no credential values, provider state, scheduler or tool
executor. `Allows` compares exact tool contract hashes, resources, effects and
credential references. An empty delegated allowlist grants no tools.

`CheckTaskTx` reads the owning Task Service's migration057 parent/version/deadline
fence on the caller's transaction. A present but broken schema fails closed.
Model admission, Broker prepare/dispatch and result persistence retain their
existing independent ownership and owner authority checks.

The owning orchestration manager and private Broker control adapter are still
being implemented. No child scheduler, runtime tool or new owner endpoint is
enabled by merely importing these types. Do not treat this foundation as issue16
acceptance. See `docs/architecture/delegation-contract.md`.
