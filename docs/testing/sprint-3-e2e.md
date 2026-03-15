# Sprint 3 End-to-End Verification

Purpose:
- verify the first user-facing vertical slice from Telegram input through Butler orchestration to final response delivery
- verify the resulting run reaches `completed` and transcript persistence contains both user and assistant messages
- keep the existing HTTP smoke path available as a non-Telegram fallback

Prerequisites:
- copy `.env.example` to `.env`
- set `BUTLER_OPENAI_API_KEY`
- set `BUTLER_TELEGRAM_BOT_TOKEN`
- set `BUTLER_TELEGRAM_ALLOWED_CHAT_IDS` to the numeric Telegram chat id you will test from
- start the full MVP stack with `make up`

Manual Telegram acceptance path:
1. Open the configured bot from an allowed Telegram chat.
2. Send a plain text message such as `Reply with the word butler.`
3. Wait for the bot response in Telegram.
4. Confirm the reply arrives in the same chat.

Database verification:
1. Find the latest run for the Telegram session:

```bash
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.dev.yml exec postgres psql -U butler -d butler -c "SELECT run_id, current_state, session_key, updated_at FROM runs WHERE session_key = 'telegram:chat:<chat_id>' ORDER BY updated_at DESC LIMIT 1;"
```

2. Confirm transcript persistence for that run:

```bash
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.dev.yml exec postgres psql -U butler -d butler -c "SELECT role, content FROM messages WHERE run_id = '<run_id>' ORDER BY created_at ASC;"
```

Expected results:
- the latest run for `telegram:chat:<chat_id>` is in `completed`
- transcript rows include one `user` message and one `assistant` message for the tested run
- Telegram shows the assistant final response in the same chat

Optional non-Telegram smoke path:

```bash
go run ./scripts/smoke/sprint2_event_flow.go
```

Failure signals:
- the bot does not reply in Telegram
- the latest run is not terminal or is not `completed`
- transcript rows are missing either the `user` or `assistant` message
- `make up` completes but the orchestrator health endpoint is not healthy
