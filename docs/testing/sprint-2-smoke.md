# Sprint 2 Smoke Verification

Purpose:
- verify the pre-channel vertical slice by submitting a normalized event through `POST /api/v1/events`
- confirm the run reaches `completed` in PostgreSQL
- confirm transcript persistence contains both user and assistant messages

Prerequisites:
- PostgreSQL and Redis are running from the Compose baseline: `make infra-up`
- orchestrator is running with valid OpenAI settings, especially `BUTLER_OPENAI_API_KEY`
- `BUTLER_POSTGRES_URL` points at the same database used by the orchestrator

Run the smoke script:

```bash
go run ./scripts/smoke/sprint2_event_flow.go
```

Optional environment overrides:
- `BUTLER_SMOKE_BASE_URL` defaults to `http://localhost:8080`
- `BUTLER_SMOKE_POSTGRES_URL` overrides the database DSN used for verification

What the script does:
- sends a unique normalized user-message event to `POST /api/v1/events`
- reads the returned `run_id`
- polls the `runs` table until the run reaches `completed`
- checks the `messages` table for at least one `user` row and one `assistant` row for that run

Expected output checklist:
- `submitted run <run_id> for session <session_key>`
- `run reached terminal state: completed`
- `transcript verified: user=1 assistant=1` or higher counts
- `smoke verification passed`

Failure signals:
- non-`200 OK` response from `POST /api/v1/events`
- run enters `failed`, `cancelled`, or `timed_out`
- transcript verification does not find both roles
