# Isolated Telegram agent

Runs the ordinary App/Core/Input/Broker/provider composition through
`app.NewForTelegramCommunication`. The ordinary orchestrator binary keeps calling
`app.New`, with no communication toggle or changed production defaults.

Use a disposable database whose name ends in `_test`, an isolated Redis and
private service token, and a separately launched communication Broker. Apply
current migrations through094. Configure the normal provider, budgets and
deletion manifest; the command does not bypass those mechanisms. A private
operator JSON profile contains `OwnerID`, `BindingID`, `ExpectedBot`, `ChatID`,
`TokenFile` and optionally `AllowPrivate` and `BotAPIProxyFile`. The latter is a
private HTTP(S) proxy URL file; it enables CONNECT without weakening Telegram TLS.
`TokenFile` is an absolute operator
reference outside source. Never put the actual bot token in the profile or CLI.
`AllowPrivate` defaults false and explicitly authorizes private owned Catalog
material to this exact bot binding when true; secret/stale/revoked material is
always denied. Run `telegram-agent-lab -profile /private/operator-profile.json`.
`BindingVersion` defaults to 1; existing bindings must match the explicit selected
version. The verified numeric bot identity is durably pinned before admission or
sending. A recreated username cannot adopt another bot's binding/backlog.

The bot gets an independent canonical conversation key derived from its numeric
bot/chat IDs. `/start` is a normal owned input; a message without reply starts no
guessed Task. Reply-to-publication always uses its stored ACK mapping; invalid
reply targets cannot fall back into a fresh conversation. `/stop TASK_ID`,
`/pause TASK_ID`, `/resume TASK_ID`, `/amend TASK_ID text` and `/prepare TASK_ID text`
join the existing Task Controller. Natural restrictive controls require a unique
target or explicit reply; ambiguity does not execute a model or guess the latest
Task. Full widget/Telegram approval UI remains a separate capability; it is not
advertised by this constructor. Existing owning approval APIs remain mandatory.

This test composition is not production rollout or full two-channel acceptance.
Web work is tracked separately in #129, integration/acceptance in #130.

Native model acceptance requires the private `HTTPS_PROXY` environment reference.
The lab script limits `NO_PROXY` to internal services; the test validates effective
proxy selection for provider discovery, inference and OAuth endpoints before
execution. A request trace checks actual HTTP sockets, including reused HTTP/2
connections; kernel loopback HTTP and PostgreSQL remain local.
Diagnostics retain only connection counts and fixed auth phases/status codes,
never proxy URLs or credentials. This changes no production provider defaults.

The runner calls the existing `cmd/provider-auth -action diagnose-auth` before
native execution, under the shared operator auth lock and after checking current
production work. Normal Manager renewal persists the canonical encrypted row.
No stale credential copy is imported. During inference the normal Manager can
also renew through its ordinary one-minute margin; the private store adapter
permits only Antigravity's encrypted `provider-auth-refresh` write. Other keys,
imports, plaintext writes and deletion are denied. No test expiry rejection
preempts the Manager. Unknown refresh/persistence must be reconciled before retry.
The native Broker receives the exact same finite budget policy as Core. Its
immutable persisted scope limits are never replaced by another process's defaults.
The native fixture selects autonomy mode 2 and resolves only its exact synthetic
approval requests through the normal approval service; that does not qualify a
Telegram approval UI or change any production autonomy default.
# Optional isolated Web composition

The same private profile may set WebBindingID and WebBindingVersion to compose
the owner Web feed, full report/media renderer protocol, explicit replies and
new conversations. The configured owner-control identity must match OwnerID.
New Web targets are available without a send/read ACK; pending history is not
adopted. The ordinary command/main remains off.

`communication-agent-native-lab.sh ... application` is an explicit, bounded
acceptance of actual App.Run and the separate communication Broker command.
It uses a new test schema, Redis, fixed owner/bot and encrypted canonical auth
through a programmatic refresh-only store reference. JSON cannot inject that
store. Before workers start, only this dedicated test bot's previous backlog is
acknowledged without execution. Native inputs then require real owner HTTP or
Telegram ingress. Readiness alone spends no model turn. It stops on its private
operator stop file or at eight minutes; twelve owner-day/eight conversation
turns remain finite. The operator must retain all logs/accounting, including
failures, and separately verify client artifacts. No production service restarts.
