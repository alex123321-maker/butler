# Telegram communication adapter (isolated opt-in)

This package is separate from `../telegram`, which remains unchanged. Bootstrap,
production flags, legacy delivery and Web do not install this package.

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
stored sent ACK. An ordinary message without an explicit target is ambiguous.
This does not implement new conversation admission, widgets or provider auth.
`Updates` is a transport read only: the future polling owner must durably accept
updates before advancing its cursor. There is no startup poller/webhook here.

The client accepts a private operator token-file reference outside source. It
uses the fixed HTTPS Bot API host, refuses redirects, disables environment
proxies and exposes fixed errors without URLs, token values or remote bodies.
Paid broadcasts are never requested. Different bots must have different binding
identities; do not attach the new token to a legacy binding/backlog.

API mapping checked 2026-10-21 against [Telegram Bot API 10.3](https://core.telegram.org/bots/api#sendrichmessage),
[input blocks](https://core.telegram.org/bots/api#inputrichblock) and
[document blocks](https://core.telegram.org/bots/api#inputrichblockdocument).
Unit/HTTP tests and PostgreSQL manifests are synthetic evidence. Actual channel
ACK, client rendering, model behaviour and task completion are separate gates.
See the [lab command](../../../cmd/telegram-rich-lab/README.md).
