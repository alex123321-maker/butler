# Workspace physical observer acceptance — issue17

The kernel now has a reusable independent physical observer. It opens a fixed
operator-provisioned private root, accepts private exact access metadata and the
confirmation caller's PostgreSQL transaction, and verifies actual file bytes and
durable owner/session/task/revision binding. It does not grant model permission,
execute code, import environment/credentials, decide reconciliation or trust a
successful model/runtime verdict.

Create confirmation checks the exact empty namespace and data tree, private
mode-0700 directories, empty temporary directory and mode-0600 synced checkpoint
containing the allocated opaque ID. Write confirmation checks the same private
shape, a valid opaque attempt checkpoint and bounded actual regular single-link
mode-0600 file bytes against the current PG version/hash/length. Opened directory
identity is checked after observation. Evidence is content-free snapshot metadata
and a canonical SHA256 digest; the private body/root/owner/lease stay private.

The existing actual resources.Store/WorkerGuard fixture now calls the production
Observer inside mandatory confirmation, then records its digest and consumes the
real committed resource claim on that same transaction. Its exact action/source
policy and evidence table remain explicitly fixtures. They are not production
Broker authorization or an enabled workspace tool.

New actual filesystem mutation cases run after metadata CAS and before physical
confirmation. Unexpected namespace/private/temp files, shared directory/marker/
file permissions, missing/invalid checkpoint and modified body all fail. Metadata
and evidence roll back, the lease stays unconsumed, a failed creation publishes
no namespace, and a failed write blocks the existing copy. No filesystem rollback
or known success is invented. Separate checks deny foreign owner/session/task/
constraints, changed intent hash/length, private paths and wrong operations;
successful observation uses actual metadata and leaks no body/root/lease.

Lab source snapshot: `20261011-184901-970` under the isolated
`/srv/storage/butler/agents/workspace-runtime-composition` root. Disposable
PostgreSQL17, real private files, pools with two connections, CPU runtime runc,
Go1.27.2, race detection and targeted go vet were used. The original workspace
storage/resource regressions and new observer cases pass. No models were called
and main lab deployment/flags were unchanged.

Events: `reports/workspace-20261011T114951Z-899058/tests.jsonl` under that root.
All 56 distinct test/subtest names (15 top-level tests) passed, with zero skips
or failures. SHA256 of the event stream:
`9d045798d6988f962e7803f5fcd9564be6f158dfdb95ff0c56fe43472a4f1ff2`.

Remaining issue17 work includes production workspace Broker/resource/shared
budget/provenance/evidence composition and its private runtime route; owner
reconciliation and native parallel delegation acceptance remain separate gates.
This observer proof alone does not close issue17.
