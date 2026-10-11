# Private delegation admission acceptance

This is component evidence for issue16, not a complete delegation release. The
fixed revision1 `task.delegate` route grants no child tools. Optional production
bootstrap now composes private admission, owned context transitions and bounded
maintenance over existing input workers; operator defaults remain off. Actual
native scheduling, deployment and single-versus-bounded-parallel model comparison
remain pending. Existing failed/blocked report limitations are unchanged.

The composed root delta passed actual lab snapshot `20261011-090359-595`:
config, delegation, action authorization, Broker and Orchestrator app PG/race
checks, followed by input/task/core race checks and Broker app compilation.
The private separate-process runner repeated all fourteen selected top-level
groups with no failure or skip. Event reference:
`/srv/storage/butler/agents/delegation-resources/reports/delegation-20261011T020802Z-2992251/tests.jsonl`;
SHA256 `fd6a0c800e8a3f370ee7e6815e05940fe320909f807f5cb91adada7692fefb4a`.
This proves the component boundary and readiness policy in isolation. The runner
uses a compiled Broker test harness, not production bootstrap startup, and does
not establish native scheduling or main deployment acceptance.

Actual laba snapshot: `20261011-085741-646`. A disposable PostgreSQL17/pgvector
container on an internal isolated network applied all migrations, including061.
Every fixture used its own schema, private initialized deletion anchor and
`MaxConns=2`. Go1.27.2 ran the selected tests with `-race`. A separately compiled
Broker test harness ran as a real gRPC process with its own pool2 connection set,
fixed owned registry and actual actionauth/budget/Catalog stores. The main lab
Compose, database, model service and credentials were untouched.

Private event evidence:
`/srv/storage/butler/agents/durable-tasks/reports/delegation-20261011T015745Z-2948615/tests.jsonl`.
SHA256: `9d74ca814943bfecd896e99080cb8173e2da716d6f497e8a19a89c4d4234dfd5`.
All fourteen selected top-level groups passed: eleven actual PG admission groups,
including seven stale-state and two assigned-mandate subcases, plus three
contract/HTTP/router unit groups. No selected group skipped or failed. The
separate process harness is explicitly invoked by the actual Broker group; it is
not a mock RPC or an unselected skipped acceptance scenario.

| Boundary | Actual evidence | Practical limit |
| --- | --- | --- |
| Private transport | Anonymous gRPC and HTTP denied; token plus an unused grant denied; redirects refused; a real Broker process prepared, authorized, claimed, called private admission and received committed metadata | Standalone composition, not main deployment wiring |
| Exact approved action | Real owner approval Service plus current action reader; changed args/budget/contract/effect/run/call and authority-bearing fields denied; empty child permissions fixed | Approval checkpoint fixture isolates this boundary; restartable approval lifecycle is proved by its separate suite |
| Durable publication | Child/session/source grants/input/snapshot/shared child allocation committed together; input failure rolls everything back; snapshot updates/deletion refused | No successful child execution inferred from admission |
| Replay | Exact current inflight HTTP retry yields the same child; actual tree wait crossing expiry or a dispatch transition denies an existing-child retry while preserving its history; ordinary lifecycle still works | Lost response after commit remains uncertain and needs observation, not automatic retry |
| Current authority | Real Catalog correction/deletion, task revision, terminal run, lease/dispatch expiry and changed autonomy deny admission; real owner stop preserves executing journal while denying child publication | Existing effect is not undone or labelled absent |
| Owner assignment | Actual goal/mandate/task assignment permits exact bounded research; authenticated goal pause denies admission and accepted lifecycle before async sweep | No broad permission inferred from registry metadata or fixture capability rows |
| Deadline wait | Actual dispatch-row lock held across exact grant deadline; PG clock reread after lock; pool2 caller returns bounded denial with no child/input/snapshot | Tests the admission boundary, not every runtime interruption |
| Outcome ordering | Actual `RecordDispatchOutcome` pauses after its effect lock; concurrent existing-child admission waits in effect-before-authorization order, then denies the spent dispatch; confirmed evidence and loop observation persist | Two independent pool2 stores model Broker and Orchestrator; the pause trigger is fixture-only |
| Durable child identity | Reconstructed admission manager validates the child snapshot after parent run completion, expired parent lease/grant and changed old grant state; actual independent child lease loads only selected DATA | Explicit durable state change/reconstruction, not a parent SIGKILL experiment |
| Shared bounds | Actual root reservation records charge one external admission call and one child; accepted child has no delegate or other tools | Reservations are conservative allowance, not measured/billed model cost |

The admission reader does not use the guard fixture's capability table. Proposal
capabilities derive the actual inflight intent/grant/permanent-effect/dispatch
rows, current worker and exact approved JSON. Lifecycle capabilities derive061
plus immutable057 contract/current source/owner authority. Catalog source body
reads, shared budgets, durable approval decisions and manager stores are real.
Fixtures create the initial parent execution state explicitly, without claiming
native generation. No model call or external recipient effect occurs here.

Repeat on the isolated lab root after source Sync:

```sh
bash /srv/storage/butler/agents/durable-tasks/current/scripts/dev/delegation-admission-lab.sh /srv/storage/butler/agents/durable-tasks
```

The runner bounds disposable database/test containers, uses only storage-local
caches/TMP and builds the separate Broker harness before running selected tests.
Without `BUTLER_TEST_MEMORY_POSTGRES_URL`, PG fixtures explicitly skip; without
`BUTLER_DELEGATION_BROKER_BINARY`, the actual-process group explicitly skips.
Neither skip is acceptance. This runner supplies both, and uses no main service.

The component includes no public admission endpoint, runtime credential path,
model permission field or new dependency. Optional scheduler composition is an
operator environment choice and cannot be enabled by stored settings. Converter child
capabilities require a separate reviewed fixed contract/reader; initial
context-only revision1 is not widened by an existing converter catalogue. Full
issue16/issue33 evidence still needs real independent/conflicting research and
the shared-model quality/resource experiment, plus the pending product runtime
and scheduling criteria.
