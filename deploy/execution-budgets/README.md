# Reproduce runtime budget verification

Use an isolated lab source root and a PostgreSQL database ending `_test`. Do not
run this against production history. Migration037 rollback intentionally refuses
spending, model attempts or loop observations. Policy changes cannot replenish
existing scopes; an isolated fresh database is required when testing new defaults.

Sync on Windows from the repository:

```powershell
pwsh -NoProfile -File scripts/dev/lab.ps1 -Action Sync -RemoteRoot /srv/storage/butler/agents/execution-budgets
```

On the lab, run the container below after creating the isolated test database.
Replace the synthetic test DSN through private shell environment. PostgreSQL,
actionauth and completion tests require the same isolated database; Redis is a
separate test transient DB. Dependencies/build caches stay on storage.

```sh
docker run --rm --user 0:0 --network butler-lab_backend \
  -v /srv/storage/butler/agents/execution-budgets/current:/src:ro \
  -v /srv/storage/butler/agents/execution-budgets/.cache:/cache -w /src \
  -e GOCACHE=/cache/go-build -e GOMODCACHE=/cache/go-mod \
  -e BUTLER_TEST_POSTGRES_URL -e BUTLER_TEST_ACTIONAUTH_POSTGRES_URL \
  -e BUTLER_TEST_COMPLETION_POSTGRES_URL -e BUTLER_TEST_REDIS_URL \
  golang:1.27.2-alpine sh -c 'apk add --no-cache build-base su-exec >/dev/null && exec su-exec 1000:1000 go test -race -p 1 -count=1 ./internal/budget ./internal/actionauth ./apps/tool-broker/... ./apps/orchestrator/internal/task ./apps/orchestrator/internal/input ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/app ./apps/orchestrator/internal/api ./apps/orchestrator/internal/completion ./internal/transport/... ./internal/config'
```

Coverage: sixteen parallel child admissions with two connections; atomic Broker
grant denial before an actual HTTP recipient; unknown recipient acknowledgement
without replay; root/mandate/day shared quotas; new run/child/restarted controller;
assigned mandate reused across independent tasks; changed authority and local
privacy denial before inference; saved continuation at zero model budget and
expired live deadline; normalized repeated errors/progress; typed byte/cost
limits; malformed runtime success; complete batch rejection; frozen completion
without fresh inference; owner-only budget read model; refusal of history rollback.

Model fixtures supply deterministic streams. These checks do not call Google or
claim authenticated model availability. Current production inference limits exist
only for native Antigravity; other adapters fail closed until limits are declared.
Tool egress reservations require issue31's independent runtime wire/session caps
to prove actual byte enforcement. Broad owner-goal UI/jobs are issue14's separate
scope, while its versioned assignment/budget adapter is checked here.
