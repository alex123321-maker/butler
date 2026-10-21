# #101 and isolated Telegram Rich Message progress

Baseline: merged PR #127, `d307a6c6a38e376bb56665cddb2c293b09538803`.
This is an implementation/progress report, not full #101/#102 acceptance.

## Implemented scope

- Additive migration091 and retained owner/source-bound media in the existing
  Artifact Store. Read requires current policy, owning-session equality, privacy
  and content integrity. New saves require the actual owned run/session token.
- Transactional `PlanDelivery` freezes media/report parts in the existing outbox,
  with one canonical publication/transcript, deterministic part identity and
  individual ACKs. Missing required artifacts or failed planning rolls back all
  publication writes. No planner means photo/report admission remains denied.
- Separate `channel/telegramrich`, with plain message, photo and native
  `sendRichMessage` block mapping. Report splits at block boundaries, preserves
  tables/media/documents, refuses unknown blocks and silently discarded fields.
- Media is uploaded directly. Manifest pins reference/name/MIME/size/SHA-256;
  current media/claim/policy is rechecked immediately before the HTTP boundary.
- Exactly one HTTP attempt. Wrong/malformed/lost ACK, redirects and transport
  failure remain UNKNOWN; explicit rejection permits the existing bounded retry.
- Authenticated private owner reply normalization with current bot/user/chat and
  binding/version, stored ACK and canonical Input/Task Controller. No new task
  authority is inferred from message text or preference replies.
- Operator-only lab with fixed synthetic fixtures, expected test-bot username,
  explicit test chat and private token-file reference outside source.

## Live channel evidence (2026-10-21)

Created a separate `@butler_rich_lab_20261021_bot` through official BotFather.
`getMe` verified the exact expected bot username. Token was transferred privately
to an operator file outside the repository, protected to the Windows owner;
no token, raw BotFather screenshot or credential URL is published here.

| Scenario | Observed evidence | Boundary |
| --- | --- | --- |
| Message | `sendMessage` ACK 2 | Native channel send |
| Photo introduction | `sendMessage` ACK 3 | Separate ordered caption-text part |
| Photo | `sendPhoto` ACK 4 | Generated PNG plus visible caption |
| Native report | `sendRichMessage` ACK 5 | Heading, summary, native table, PNG, text-file block |
| Client rendering | Telegram Web K accessibility tree contained heading/table/cells/photo/document and visual inspection confirmed the report | One client; not Web Butler renderer |
| Ordinary reply | Bot API update for owner message 6 contains `reply_to_message.message_id=5` | Correct native transport reply target |
| Document | `butler-lab.txt`, 78 B, visible inside the native report | File block visible; independent download/hash gate remains unverified |

![Native report in the isolated test bot](../testing/communication-telegram-rich-20261021.png)

The browser's stale plugin path was repaired using a directory junction to the
installed Browser package and reconnecting its service. No Telegram session,
cookie or profile files were inspected. Existing bots were not changed.

## Validation and limitations

New HTTP/unit tests cover outcome classification, native multipart payloads,
deterministic block splitting, Unicode boundaries, missing authorization, media
replacement and revocation after reads. PostgreSQL scenarios cover atomic media
parts, restart, no replay after UNKNOWN, retained media owner/privacy/digest,
missing-required-attachment rollback, new adapter reply identity/deduplication/
canonical target and rollback refusal with retained history.

The required Linux/Go1.27.2/PostgreSQL17/race suite and regression script are
extended with artifacts and the new adapter, plus lab build. JSONL remains in
`.cache/communication-core` locally and is uploaded by the existing CI job.
Final counts/CI are recorded in the PR after completion; this file does not
substitute pending checks with a pass.

A broad Windows run was not a successful acceptance run: existing Catalog tests
cannot sync a directory on Windows, and some failed scenarios also failed their
fixture cleanup. The directory-sync error was reproduced on untouched baseline
`d307a6c6` with `TestCommunicationOwnerHTTPProofAndStrictBodiesIntegration`.
Linux is the supported environment for the complete fault matrices. Newly
selected adapter/artifact/media/reply checks also passed on Windows.

## Remaining work and status

#101 stays open: Butler Web feed/detail/SSE/rendering are explicitly deferred;
there is no live Web+Telegram canonical fanout acceptance. New Telegram polling
composition, durable cursor and new-conversation/model admission are not installed.
The lab's operator fixture is not model execution through Broker, and does not
prove a model's early message -> tools -> final flow in this bot. Existing
foundation tests retain their own independent evidence.

#102 stays open: native Telegram renderer/transport has begun, but Web reports,
complete server-limit corpus, live oversized multipart report recovery and
independent file-open/download gates remain incomplete. Widgets belong to #104.

No production deployment, explicit-mode enable, Bootstrap registration, Web or
legacy `channel/telegram` change is included. No new dependencies are added.
