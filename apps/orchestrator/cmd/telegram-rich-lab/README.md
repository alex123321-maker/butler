# Isolated Telegram Rich Message lab

This operator command tests only synthetic channel fixtures in a new test bot.
It does not start Butler, invoke a model/tool, grant product authority or enable
the explicit communication profile. Production bots and Web remain unchanged.

Store the new bot's token in a private file outside the repository with owner-only
permissions. Supply the filename, never the token, on the command line. The bot
must match the explicit expected username. Send `/start` from your private chat.
To use an operator proxy, add `-proxy-file <private-proxy-file>` to each command.
The file contains an HTTP(S) proxy URL and must have owner-only permissions;
never put a credential-bearing proxy URL in command arguments or source.

```
go run ./apps/orchestrator/cmd/telegram-rich-lab -token-file <private-file> -bot <test-bot> -action probe
go run ./apps/orchestrator/cmd/telegram-rich-lab -token-file <private-file> -bot <test-bot> -action updates
go run ./apps/orchestrator/cmd/telegram-rich-lab -token-file <private-file> -bot <test-bot> -action send-fixtures -chat-id <explicit-owner-chat>
go run ./apps/orchestrator/cmd/telegram-rich-lab -token-file <private-file> -bot <test-bot> -action verify-document -chat-id <explicit-owner-chat> -reply-id <report-ACK>
```

`updates` emits only update/chat/user/message IDs and reply presence, never message
text. It does not advance a durable cursor or infer an owner from the first user.
The operator explicitly selects the test chat. `send-fixtures` sends four channel
parts: a message, photo introduction, generated PNG photo and native report with
table/photo/text-file attachment. It prints only channel outcomes/ACKs. The fixed
operator fixture authorization is not a production policy implementation.

There is no automatic retry, even on a second command invocation: invoking
`send-fixtures` again is a new operator send. After an UNKNOWN outcome reconcile
the chat before any new run. Use the existing durable Communication/outbox worker
for actual publications; this command is not an outbox replacement.

`verify-document` selects an owner reply to the exact report ACK and verifies
the uploaded synthetic file through Bot API download, comparing retained bytes.
It prints filename/size/SHA-256 only, not the remote file ID/path/token URL.

The opt-in `TestNativeTelegramRichOutboxProcessRestartIntegration` uses a real
disposable PG test schema, canonical report/outbox and the main explicit adapter.
It sends a two-part native report from separate processes which exit after their
durable ACK. Explicit `BUTLER_TEST_TELEGRAM_RICH_TOKEN_FILE`, `_CHAT_ID` and `_BOT`
environment variables authorize only that test bot. CI has no such variables.

Live ACK and visual checks do not establish real model continuation or Web
readiness. Scoped closure of #101/#102 preserves those gates in #129/#130;
it does not close the complete product profile or authorize production enable.
