# Ordinary guarded tab request — 2026-10-11

The separate default-false request flag now joins ordinary model tool dispatch
to the durable owner-selection path. Exact private Broker authorization, shared
budget and current source/task/Session authority precede request creation.
Empty public args carry no binding, account, host, path, tab ref or credential.
The private grant pins the actual ingress claim hash independently of the
immutable owner-approved action: fresh approved recovery may use a new claim,
but an old dispatched grant cannot borrow it.

Actual publication atomically stores finite choices, the immutable dispatch map
of migration065, the real input continuation and run/task waiting state. Core
finds it before generic tool transcript/checkpoint/provider writes, including a
lost private ACK. It neither retries the request nor supplies its local receipt
to the old provider. Web retains protected owner choices. Telegram directs to
that page without legacy/generic selection callbacks. A selected mapped request
with an uncertain Broker outcome cannot resume. Status/click runtime activation
and human channel acceptance remain separate gates; this does not close #17.

Narrow actual PG/pool2/race/private Broker-process proof used snapshot
20261011-162752-413:
`/srv/storage/butler/agents/guarded-tab-request/reports/guarded-request-20261011T092755Z-437260/tests.jsonl`,
SHA256 `cbcff017fda45f19ddfc0eacba241a051ac129c66bde14f48d0a1ac7cb00fdde`.
The final source added fresh authority/deadline checks and core/config/channel
regressions. Final expanded proof used snapshot20261011-163750-332:
`/srv/storage/butler/agents/guarded-tab-request/reports/guarded-request-20261011T093753Z-473024/tests.jsonl`,
SHA256 `94f4689dfbc12a0777bab17faab0f19b2d1ac84d8dc3c19d334ccd4f8bc0d894`.
424 distinct passed test/subtest names, eight packages, race/vet and six explicit
skips. Actual action-journal, budget, authorization and old durable-task PG
regressions executed with their real legacy DSNs. Skips are the separate raw
browser E2E, native/delegation harness opt-ins, controlled-browser Chrome fixture,
guarded Chrome conditional and standalone full-service fixture. The new real
Broker subprocess executed independently inside `broker_process`; the guarded
Chrome conditional executed independently below. Those facts do not turn the
remaining separate skips into passes.

Seventeen actual request scenarios include private Broker gRPC/HTTP admission
and anonymous denial, unclaimed/wrong action or contract, nonempty args, a
different input action slot, replaced or expired claim/input/worker, changed
task, deleted source, metadata failure, expiry during metadata, immutable mapping
and disabled schema safeguards. Successful cases perform protected owner HTTP
selection and fresh leased/input-claimed recovery. The deliberately uncertain
case preserves the local request but denies resumed execution. Metadata here is
a fixed fixture; no browser action or actual inference occurs in these cases.
Core unit cases separately exercise completed/lost ACK and disabled composition,
proving no generic result write or provider continuation. Channel unit validation
proves the guarded request never uses legacy/generic Telegram callbacks.

The first failing live-claim test is retained at
`guarded-request-20261011T091915Z-407361`; it exposed the old dispatch borrowing a
replacement claim. Private-grant claim pinning fixed the behavior without changing
the immutable action approval. Earlier compile/fixture failures and the initial
core fixture's missing action hash are retained, not counted as passing proof.

Actual native-host/PG/Broker/Chrome regression used snapshot20261011-163247-780
and report
`/srv/storage/butler/agents/resource-runtime-bridge/reports/single-tab-20261011T093250Z-454026`:

- `pg-race.jsonl`: `22fb38428e0adbf274a4a0c7a8ffb29fd70d114411589346e619ac57ba8476b1`.
- `bridge-race.jsonl`: `939c67667a798565f09f4607efefc75b56146e97c881b99657941dd3daf38a01`.
- `chromium.log`: `dea0e1da4ac150245eb1697a5635747a72ec560a189e7ede573ebc0c8f0d0596`.

68 distinct PG/bridge test/subtest passes plus ten actual Chromium cases. The
earlier conditional Chrome skip is accounted for by the later installed
native-manifest/extension process execution. These use synthetic pages and
interference; they do not claim human use or verified external recipient success.
All validation added zero actual model calls. No main deployment pointer, auth,
PostgreSQL state or optional flag changed; the fifth native window stays sealed.
