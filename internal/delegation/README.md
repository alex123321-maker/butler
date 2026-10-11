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
Report parsing rejects duplicate nested keys, including escaped and case aliases;
ambiguous conflicting values never acquire last-value-wins semantics.
No runtime tool or owner endpoint is enabled here. Production Catalog/Broker/
context composition and actual model comparison remain pending acceptance.
See `docs/architecture/delegation-contract.md`.

`AdmissionContract` now defines the optional fixed `task.delegate` prepare route.
Strict decoding accepts only current Catalog references, finite allocation,
deadline and dependency IDs; it cannot express owner/task/lease/permission or
credential authority. `AdmissionHandler`/`AdmissionClient` provide bounded private
service traffic and refuse ambiguous JSON/redirects. Their direct private HTTP
transport ignores ambient provider proxies, keeping service tokens on the
operator-selected internal connection. Authentication does not
authorize admission: the owning Orchestrator service must recheck the actual
inflight dispatch. Migration061 persists its immutable accepted child snapshot.
The separate real Broker process acceptance proves committed admission only;
tools/default enabling, child model success and parallel-model comparison remain
pending. See [admission evidence](../../docs/ai/delegation-admission-acceptance.md).
