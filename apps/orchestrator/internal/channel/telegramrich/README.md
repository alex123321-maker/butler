# Telegram communication adapter (isolated opt-in)

The explicitly gated native format corpus pins the dedicated lab bot/owner and
requires a private proxy reference. It makes one attempt per fixed synthetic
request, including the documented text rejection boundary, with no paid broadcast,
inference, production activation or automatic retry. It qualifies transport
formats, not agent authorization or task completion. See the dated integration
evidence for tested client bundle fingerprints, server observations and limits.

This codec is consumed by the explicit constructor in `../telegram` while
remaining separate from its legacy text client. Normal Bootstrap, production
flags and Web do not install the new composition.

`Planner.Plan` freezes one deterministic manifest per outbox part inside the
Communication publication transaction. `Store.PlanDelivery` is required for
photo/report admission; message/widget-only compositions retain their baseline.
Only the Telegram binding is implemented here. Do not install this planner for
a profile which advertises Web rendering or widgets through this adapter.

- Message uses plain `sendMessage`, split at 4096 UTF-16 units.
- Photo saves the accompanying text first, followed by individual `sendPhoto`
  parts, each with its own ACK. Captions are bounded to 1024 UTF-16 units.
- Report uses native `sendRichMessage` with `InputRichMessage.blocks`: headings,
  paragraphs, preformatted text, quotations, lists, tables, dividers, photos and
  documents. No HTML execution, link/PDF fallback or implicit final publication.
- Local report limits: 32 KiB JSON manifest, 50 blocks and 20 MiB of uploads per
  part; 100 parts per publication. These are conservative Butler limits, not
  assertions about Telegram's undocumented server limits. An indivisible block
  exceeding a bound fails before publication writes; no content is truncated.
- Required retained media is resolved during planning and again before sending.
  The manifest pins reference/name/MIME/size/SHA-256, without bytes or public URLs.
  Missing, changed, redacted or forbidden media refuses the send.
- Authorized publications allow native file opening/saving. They do not force
  Bot API protect_content, which prevents saving and contradicts report file
  acceptance. Source/export authorization is the privacy boundary; already
  exported client copies cannot be recalled by later Butler redaction.

`Sender.Authorize` is mandatory and must use the owning `DeliveryStore` guard.
It runs before media reads and again after multipart construction, immediately
before a single HTTP attempt. `Resolve` must enforce current Catalog and export
policy, not only owner equality. No constructor supplies an allow-all guard.
The ordinary outbox worker retains claims, per-part ordering and UNKNOWN policy;
no second delivery queue or retry loop exists. An expired sending claim or lost/
malformed/wrong-chat ACK remains unknown and is not automatically resent. Only
an explicit Bot API rejection can establish absence and bounded retry.

`Ingress.Reply` normalizes private owner replies to the existing canonical Input
and owner-control paths. Trusted composition must pin owner, user, chat, bot,
binding and version; the replied-to message must belong to that bot and have a
stored sent ACK. A message without a reply is ambiguous unless trusted
composition supplies `Conversation`, which joins the kernel's normal Input path.
Invalid replies never fall back into that hook. Transport does not run a model
or implement widget/approval decisions.
`Poller.ProcessOnce` serializes each bot's transport read and stores its cursor
in PostgreSQL migration092 only after owning Input/control commit. Lost cursor
commit replays stable delivery identity; generic denial/storage errors retain
the cursor. Out-of-order batches are rejected before acceptance. There is no
startup poller/webhook registration. Consumption history prevents rollback.
The isolated App command is the explicit opt-in startup composition. Its numeric
bot identity is pinned before admission/sending; a recreated username cannot
adopt another bot's binding. `Notices` uses fixed source-free clarification text
and migration094 pre-send receipts; unknown outcomes are never resent. A proven
missing reply target can be consumed with clarification, while generic guarded
SQL/policy failures preserve the cursor.
The explicit adapter retries failed read-only polling with cancellable1–30s
backoff. A Telegram transport/identity/storage failure cannot stop Web/Core.
Delivery remains the existing independent outbox worker; this polling retry
does not replay sends, regenerate an answer or advance an uncommitted cursor.

`Capabilities` is an explicit trusted readiness profile; its zero value exposes
no methods. Native report requires rich/photo/document support and cannot fall
back to a link/PDF. `VerifyDocument` compares a selected uploaded file with
retained bytes through fixed `getFile`/download endpoints; untrusted returned
paths cannot select another host, dot segment, query, fragment or redirect.

The client accepts a private operator token-file reference outside source. It
uses the fixed HTTPS Bot API host, refuses redirects, disables environment
proxies and exposes fixed errors without URLs, token values or remote bodies.
Trusted operator composition can explicitly supply a private HTTP(S) proxy URL
file with `NewClientFromTokenFileWithProxyFile`. This uses CONNECT to the fixed
official HTTPS host and keeps normal certificate verification enabled. Proxy
credentials remain inside the client, outside model inputs and diagnostics.
Paid broadcasts are never requested. Different bots must have different binding
identities; do not attach the new token to a legacy binding/backlog.

API mapping checked 2026-10-21 against [Telegram Bot API 10.3](https://core.telegram.org/bots/api#sendrichmessage),
[input blocks](https://core.telegram.org/bots/api#inputrichblock) and
[document blocks](https://core.telegram.org/bots/api#inputrichblockdocument).
Unit/HTTP tests and PostgreSQL manifests are synthetic evidence. Actual channel
ACK, client rendering, model behaviour and task completion are separate gates.
See the [lab command](../../../cmd/telegram-rich-lab/README.md).

## C05 widgets

Trusted `Capabilities.Widget` enables single/multiple choice and explicit text
reply. Short callback data is a locator; the actual owner, numeric bot and sent
ACK mapping remain mandatory. Multi-choice toggles share a durable draft CAS,
Submit alone resumes. Full option labels/descriptions and retained images precede
the controls; long button labels have a short preview with the complete text
retained above. Text uses Bot API10.3 InlineKeyboardMarkup with one disabled status button
(no callback) and `force_reply: true`, which remains editable; terminal edits
preserve the nonempty keyboard and immutable flag. Empty keyboards are treated
as standalone ForceReply by the real API and cannot be edited. Unknown/ambiguous replies never select the
latest question.

The sender reads current state before initial I/O. Closed questions have no live
buttons. Projection edits target a known message ID, serialize per ACK and record
sent/failed/UNKNOWN separately. Interrupted edits never resend a publication.
A stale client is always fenced by current backend state. Mapping checked against
[Bot API](https://core.telegram.org/bots/api#inlinekeyboardbutton), 2026-10-22.

## Public emoji research reads

The operator-only GetCustomEmojiSet and DownloadCustomEmoji helpers retain the
fixed host/proxy/secret boundaries for bounded public catalog imports. They are
not registered as tools and do not poll or send messages. See
[research protocol](../../../../../docs/architecture/telegram-emoji-research.md).


## Optional reviewed emoji decoration

The owner-profile orchestrator accepts `-telegram-emoji-decoration` (default off).
Planner uses an embedded, reviewed Kolobkitop concept whitelist; no inference,
network access or new dependency. Every content word must be supported; negative,
uncertain, quoted/code/link/question, ambiguous and unknown content abstains.
Labels containing an existing Unicode symbol/emoji also abstain, including widget
options; the decorator does not add a conflicting second icon.
`Part.Text` and canonical payload remain unchanged. Optional `Part.Decoration`
pins policy hash, original text hash, at most three insertions and stable option
icons. Sender and widget edits replay these choices, using UTF-16 entities and
`icon_custom_emoji_id`; callbacks and authorization are unchanged. Photos/reports
are outside v1. Stored invalid choices reject, oversized decoration falls back
before persistence. Keep a compatible reader until decorated outbox Parts drain.
See `experiments/emoji140/abstention-rollout.md` for measured limits.
