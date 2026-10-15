# Candidate extraction evidence

These records contain only synthetic Russian acceptance inputs/results, fixed
criteria, usage/reservation accounting and content-free PostgreSQL diagnostics.
Private provider credentials and environment files stay on the isolated server.

`baseline-v1-summary.json` combines native-v2's3 admitted attempts and native-v3's
exact21-case remainder. The original 24-case version1 screen failed; the unknown
paid attempt and all conservative reservations are retained, never treated as0.
The two known semantic failures omitted current-time/project applicability despite
exact source quotations. The accepted version2 `native-v6` window used the same
original 24 cases plus the independently frozen 8-case holdout, with prompt/corpus/
thresholds unchanged after the earlier incomplete version2 screen.

- `quality-native-v4.json`, `observations-native-v4.json` and
  `ledger-native-v4.json` preserve 26 admitted attempts, including the second
  unknown paid attempt. Caps/binary/source hashes are named `native-v4-*`.
- `setup-native-v5.json` preserves the 0-call typed-policy fixture failure. Its
  original cap/log/binary are retained on the server, and no consumed label was
  reused.
- `quality-native-v6.json`, `observations-native-v6.json`,
  `ledger-native-v6.json` and `native-v6-*` seal the full32-case High32768/300s
  screen: core23/24, holdout8/8, exact citation32/32, unsafe0, authority0 and
  current-window unknown0. The one core type error remains in the observations.
- `owner-lifecycle-native-v6.json` records the separate explicit owner acceptance,
  correction and source-revocation stage:0 additional model calls,82 attempts
  before and after. Frozen quality artifacts retain the then-false production
  qualification flag; they are not rewritten after later release review.
- `cumulative-native-v6-cost.json` totals all 82 attempts/80 known/2 unknown,
 87,441,408 conservatively reserved tokens and176,553 known observed total tokens.
- `authdiag-v1-*.json` records safe current auth/discovery health, without raw
  credentials, account identifiers or arbitrary provider errors. It does not
  assign a cause to a historical `provider_unavailable` attempt.
- `pg-observations-native-v*.json` keeps content-free SQL shape/hash, bounded
  latency/diagnostic and pool counts. No SQL text, arguments or raw errors appear.

The embedded ledger filename `ledger.json` in frozen quality records refers to
the matching `ledger-native-v*.json` in this archive. The original source/binary
manifests retain their isolated-server paths. `controlled-summary.json` is a
legacy aggregate of 196 pass records across overlapping runs, not 196 distinct
tests; its optional skips did not qualify native or actual dump/restore gates.

The raw ledger aggregates durable attempt quotes/usages and budget scopes. Scope
reservations repeat the same operation across applicable scopes; do not sum all
scope entries as if each were a separate provider request. Count each attempt
once. Usage total can exceed visible input+output because provider thinking is
included. Subscription allocation, actual HTTP/OAuth bytes and CPU/storage cost
are unmeasured; declared no-overage cost is not a claim of zero total cost.

See [the per-criterion report](../../../docs/testing/issue-85-memory-candidates.md)
for controlled composition, release limits, later windows and final decision.
