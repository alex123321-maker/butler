# Isolated Telegram Rich Message lab

This operator command tests only synthetic channel fixtures in a new test bot.
It does not start Butler, invoke a model/tool, grant product authority or enable
the explicit communication profile. Production bots and Web remain unchanged.

Store the new bot's token in a private file outside the repository with owner-only
permissions. Supply the filename, never the token, on the command line. The bot
must match the explicit expected username. Send `/start` from your private chat.

```
go run ./apps/orchestrator/cmd/telegram-rich-lab -token-file <private-file> -bot <test-bot> -action probe
go run ./apps/orchestrator/cmd/telegram-rich-lab -token-file <private-file> -bot <test-bot> -action updates
go run ./apps/orchestrator/cmd/telegram-rich-lab -token-file <private-file> -bot <test-bot> -action send-fixtures -chat-id <explicit-owner-chat>
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

Live ACK and visual checks do not establish full #101/#102 acceptance, real model
continuation or Web readiness. Do not close those issues from a fixture pass.
