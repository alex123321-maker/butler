# Exact workspace Broker/runtime acceptance — issue17

Migration067 and `workspaceexec.DispatchStore` compose real permanent action,
current original-input/source/worker/task checks, shared budgets and exact finite
resource claims. The actual Broker Server/executor/router reaches an authenticated
private gRPC workspace Runtime using the real Store and independent Observer.
Owning readiness/context projection and exact human approval in these tests are
explicit fixtures. Application bootstrap and production rollout are not claimed.

Canonical create/read/write contracts accept only bounded DATA and file CAS.
Namespace allocation, fixed host root, original provenance, worker/input authority,
service authentication and tickets remain private. Successful physical observation,
source context, file metadata, immutable effect evidence and write-lease consumption
commit together; reads release their held lease after current final checks.

Actual disposable-PG/files/private-RPC cases prove:

- Create, write and read return the actual bounded bytes and monotonic file
  snapshot, without private host root, token or ticket in model output.
- One-use runtime start and permanent Broker permission cannot replay. An actual
  committed creation with a lost response leaves Broker effect_unknown; a new
  non-read request stops at irreversible admission and spends no second budget.
- Stale approved file CAS denies before a new admission commits. Existing bytes
  survive unchanged and grant/reservation/lease/ticket are not partially spent.
- Original input replacement/expiry, wrong checkpoint, worker expiry, changed
  task/constraints, original-source deletion and missing budget deny. Actual
  admission rolls back permission, budget and resource history atomically.
- Missing owning readiness hides contracts and denies preparation. Missing exact
  protected composition cannot borrow the browser's local selection authority.
- Anonymous/duplicate/incomplete service or ticket headers, changed exact contract,
  arguments, runtime target, action fingerprint and credential-bearing private RPC
  deny before any filesystem effect/evidence/start is published.
- Original graph/deletion-anchor, physical-tampering, pool2 wait-expiry, immutable
  provenance and privacy-erasure regressions from PR67/68 continue to pass.

Final source snapshot: `20261011-201446-150`. Public events under
`/srv/storage/butler/agents/workspace-broker-runtime`:
`reports/workspace-20261011T131450Z-1161312/tests.jsonl`, SHA256
`832f7a59080aa9f57af8765559b9d5878260b7169d45751fe2af72b48790dfde`.
299 distinct package/test/subtest names (151 top-level), seven packages, zero
failures. All targeted race and vet checks pass on isolated laba, Go1.27.2,
CPU runtime runc, PostgreSQL17 and actual private files. No inference occurred.

Six explicit unrelated opt-in skips remain: general browser E2E, native delegation
harness, generic delegation harness, actual Chromium resource CAS, installed native
host/Chromium composition and external PostgreSQL dump/restore. Previous native
browser proofs are documented separately; this report does not claim to repeat them.

Earlier successful narrow Server/runtime proof: snapshot20261011-200804-527,
`reports/workspace-20261011T130808Z-1138562/tests.jsonl`,31 distinct names, no skips,
SHA256 `139bacbc09cbcfe2c62714872bc4159aa7f5f5f12eac99b6e5e53cb843045437`.
Retained earlier broad proof before adding the authorization test DSN:
`reports/workspace-20261011T131245Z-1154124/tests.jsonl`,248 passes with more skipped
PG regressions, SHA256 `df124aca46cccdebe6784571ce7371ab902b846a37d1607d2b84ad2c4bd9cc69`.
The final run enables that existing disposable test DSN and removes those skips.

Retained failed fixture iterations:125727 omitted Broker Outcome before the next
tool;130734 used a nonexistent registry helper;130955 incorrectly expected the
unknown-effect barrier during preparation rather than permanent admission. The
fixtures now use actual registry registration/journal completion and assert the
existing irreversible stop. No policy, acceptance criterion or native window was
weakened to pass these tests.

Production flags and main lab deployment are unchanged. Application/runtime
bootstrap, operator/owning readiness, privacy cleanup scheduling, separately
authenticated owner reconciliation and remaining integrated#17/native#16 gates
are still pending. Do not close those issues from this component proof alone.
