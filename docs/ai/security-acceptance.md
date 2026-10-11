# Programmatic security acceptance — issue 31

The complete integrated matrix also passed on commit
`159934737a42eeec7a1deef2b90f6a9a6fd4c0b1`, snapshot
`20261011-122704-930`: 28 groups and 137 required executions, zero real model
requests. It includes the optional model integrations and strict V3 measurement
parser; feature defaults remain disabled. Private proof:
`/srv/storage/butler/agents/issue-completion/reports/fault-20261011T052708Z-3845374`.
The retained content-free [integrated report](../../deploy/security/acceptance-integrated-20261011.json)
has SHA256
`05f587f517e1c21f459465ec2eef58b7c070c98ead52bd22947bde9654ce66bf`.
It retains eight passing scenarios and five pending product scenarios; the
report still has `issue_33_ready=false`. The earlier proof below is preserved.

The Full lab matrix on commit `7b1e1e2bd703d33357a9b1ce0bd420f2813eef13`
passed 28 groups and 137 required top-level executions. Its read-only source
snapshot is `20261011-113533-470`; private artifacts are retained at
`/srv/storage/butler/agents/issue-completion/reports/fault-20261011T043536Z-3667778`.
The content-free [report](../../deploy/security/acceptance-20261011.json) has
SHA256 `082bf9e4185f8f949c3434e6b366e10595496b97d44590cb4857c9aecdcf5812`.
It pins source/schema/corpus/criteria, container identities, hardware and each
group's raw-event digest. There were zero real provider requests or external
effects. These are implementation proofs, not a model accuracy measurement.

| Issue criterion | Required passing evidence |
| --- | --- |
| External instructions retain provenance through derivatives | security-content, context and delegated-sources: document/tool/summary/child bodies remain DATA, secret sources fail before input, grants stay narrow/current |
| Runtime/files cannot expand granted scope | security-workspace and generated-isolation: sibling/symlink/nonregular file denial, fresh isolated workcopy, private clients cannot select network runtimes; actual bounded Chromium/private converter evidence remains in their subsystem audits |
| Network destinations remain checked | security-egress and actual browser: mixed/rebound DNS, literals, redirects, CONNECT, origin restrictions, expiry and frozen draft windows |
| Data release/volume is independent of model choices | effects and budget: exact release approval, changed-query denial, revocation before credential inspection, atomic shared byte reservations; context/provider serialization bounds |
| Credential values cannot enter ordinary outputs/errors | security-redaction, provider-diagnostics, source-index and actual credential-e2e: synthetic reflection masking, fixed diagnostic allowlist, no secret-source model/index admission |
| Reviewed code/loader provenance is pinned | model-provenance, generated-isolation, extensions and actual extension-e2e: exact code/metadata checks, author cannot change independent criteria, immutable installed package and current permission |
| Optional model signal cannot authorize | Broker remains the independent mandatory authority boundary; no detector has a bypass/authorization interface |
| Sensitive source/derivative retention agrees with issue22 | memory, source-index and actual backup-seed/delete/restore: transitive privacy cleanup, stale publication denial, anchor history and physical restore reconciliation |
| Owner stop/revocation is independent | owner-stop and authority: actual PG dispatch/stop order, descendant revocation, idle-provider cancellation and late result denial |

Every selected group passed, including actual Chromium, private authenticated
Broker/installed extension subprocesses and three physical PostgreSQL
dump/delete/restore phases. The untrusted-content scenario is now PASS with
mandatory named tests, rather than a catalogue assertion. Earlier failed and
successful artifacts remain unchanged.

Eight available scenarios pass. Five required product scenarios remain PENDING:
delegated resource conflict, actual model pressure, extension release/rollback,
independently reviewed Russian model evaluation and the full release chain.
`selected_passed=true` does not imply `issue_33_ready=true`. Optional Prompt Guard
or PII classifier quality is not claimed; they are unnecessary for this security
boundary and cannot supply execution permission. No draft opt-in was enabled in
the running main deployment by this proof.
