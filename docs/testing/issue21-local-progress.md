# Issue21 local implementation status

Status: component implementation, not release acceptance or issue closure.
Base: main7537509d38f6ef21ae7d2641fa37ca8a2f27d2e6. The prior proposed
maintenance contract e28a53ba is retained and corrected for current migrations.
Execution stays on this PC; the working Butler deployment is untouched.

The maintenance package compares full submitted source inventories, binds
independently signed review evidence and preserves authentic negative/unknown
observations. Existing services/shared core are conservatively protected;
ordinary automatic scope requires explicitly isolated operator module boundaries.
An inventory comparison is not proof of its actual source/build provenance.

Migration105 and the owning Store preserve profiles, staged bindings, signed
reviews, physical-identity observations and stop epochs. New targets are stopped.
Configuration changes invalidate observations. Stop/resume changes the epoch;
old responses cannot refresh it. Reports never create a deployment permission.
Terminal UNKNOWN and consumed permissions cannot be rewritten/refunded; a schema
downgrade refuses nonempty history rather than delete newer owner decisions.

## Component verification

Go1.27.2 on Windows, repository-local caches, actual isolated PostgreSQL17 on
loopback55451 with database `maintenance21_test`. All state/effect fixtures and
keys are synthetic; no provider auth or native model call is used. Signing tests
run in the test process: they do not prove separate checker/node execution or
actual physical artifact measurement. No Docker deployment is exercised here.

`go test -v -count=1 ./internal/maintenance` passed9.632s, including actual PG
guard/migration down-up2.65s, observation/stop race2.81s and staged evidence2.75s.
No test skipped. `go vet ./internal/maintenance` passed. The command uses the
repository `common.ps1` environment and GOMAXPROCS2.

Exact log: [component checks](../../experiments/maintenance21/2026-10-25/component-final-v1.log),
SHA256 `579a96915817b09a38256935908fb18b3f591b49ae6b8949b0e384a44670165f`.

## Required remaining evidence

An additive component run on2026-10-26 passed50.468s, with vet exit0 and no
skipped tests: [permission/claim/outcome checks](../../experiments/maintenance21/2026-10-26/permission-claim-outcome-v1.log).
It covers concurrent exact permission registration and claim reservation,
revocation, stale/current authority, one-use dispatch, stop before/after admission,
immutable measured outcomes, fixed-deadline UNKNOWN and retained late evidence
without replay or refund. Signatures are synthetic in-process fixtures, not
independent physical checker/node proof. The one-second deadline case uses the
actual database clock; no immutable timestamp is edited to simulate expiry.

Independent source/build measurement and fixed/hidden functional checks;
separate controller/checker/node processes; authenticated exact owner permission
and one-use effect admission across real process boundaries; actual fixed-target trial/promotion/rollback;
compatible active tasks/approvals and explicit incompatible-state denial;
lost-response/stop/concurrent-version fault injection and independent benefit/
error/cost comparison. The full contract remains the acceptance requirement.

This is not authority to enable an updater or update a working process. Local
synthetic component success does not establish the remaining criteria.
