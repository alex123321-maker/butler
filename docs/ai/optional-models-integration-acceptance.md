# Combined optional voice and command code

Commit `1d160e9` integrates the existing FunctionGemma and reviewed voice caller
branches into the delegation/converter integration branch. It preserves current
delegation/converter source and final guards. No optional model/caller flag was
enabled and the running main deployment was not changed.

The isolated actual PG/race run on snapshot `20261011-115728-601` retained its
original failure at
`/srv/storage/butler/agents/issue-optional/reports/optional-models-20261011T045731Z-3744467`.
Eight packages passed; orchestration failed one outdated expected error message
in `TestExecuteRetainsWorkingMemoryOnFailure`. The fixed transport diagnostic
intentionally stores `internal_transport_error` without private provider text.
The regression now checks this code and still denies the raw provider message.
Its narrow repeat passed with the owner-observation suite. All nine affected
packages passed `go vet` on snapshot `20261011-121430-072`.

The complete orchestrator package subsequently passed with the same disposable
PG/race owner suite on snapshot `20261011-121749-585`, proof
`/srv/storage/butler/agents/task-observations/reports/task-observations-20261011T051758Z-3815005`,
events SHA256 `1d7e985d2a71374fb2239d26975a147b0f539346c4d301c6a7cc72e1ca8afad7`.
The earlier failure remains preserved; it is not relabelled as a passing run.

The original run contains 490 passing named executions and 42 explicit skipped
tests. Legacy integration tests without their separate PG/Redis variables and
actual voice/model opt-ins were skipped; they are not quality/deployment proof.
Existing actual model quality reports remain authoritative: voice and command
quality gates failed. Both features remain default-off. Scorer unit fixtures,
synthetic provider responses and component integration do not override those
reports or provide real Russian recognition/command accuracy.

`scripts/dev/optional-models-integration-lab.sh` reproduces the combined component
suite with a disposable PG/network and read-only source. Provider credentials,
the shared Compose stack and persistent deletion anchor are not used.
