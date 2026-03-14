# Telegram Manual Verification

Purpose:
- verify the in-process Telegram adapter can receive a user text message, submit a normalized event to the orchestrator, and send the final assistant response back to the same Telegram chat

Prerequisites:
- copy `.env.example` to `.env`
- set `BUTLER_OPENAI_API_KEY`
- set `BUTLER_TELEGRAM_BOT_TOKEN`
- set `BUTLER_TELEGRAM_ALLOWED_CHAT_IDS` to the numeric Telegram chat id you will use for testing
- start PostgreSQL and Redis with `make infra-up`
- run the orchestrator with `go run ./apps/orchestrator`

Manual test:
1. Open the configured bot in Telegram from an allowed chat.
2. Send a plain text message such as `Reply with the word telegram.`
3. Wait for the bot response.
4. Confirm the bot replies with a final assistant message.

Expected results:
- orchestrator logs show the Telegram adapter starting and polling
- the incoming update is normalized into a `user_message` event for `telegram:chat:<chat_id>`
- a Butler run is created and completes
- the assistant final response is sent back to the same Telegram chat

Failure signals:
- the bot does not reply to an allowed chat message
- the orchestrator logs show a Telegram polling, normalization, execution, or delivery error
- the run remains non-terminal in PostgreSQL or the assistant transcript message is missing
