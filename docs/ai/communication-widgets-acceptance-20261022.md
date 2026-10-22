# C05 widget acceptance — #104

Date: 2026-10-22. PR [#136](https://github.com/alex123321-maker/butler/pull/136),
base `b98665266f9ff0fbe1f71957a83d8c84ca59e8d5`. This is acceptance of blocking
single-choice, multi-choice and text widgets in Web and Telegram. It does not
accept the remaining C07/C08/C09 work (#105–#107), payment approval, nonblocking
questions or arbitrary forms. Closing #104 requires successful final-head CI
and review recorded on the PR; this document does not substitute for that gate.

## Composition and preserved boundaries

The isolated `ssh laba` application uses actual App.Run, Core, a separate Broker
process, canonical provider authentication, PostgreSQL17, its own Redis and a
dedicated test bot. Telegram and model traffic both use the configured proxy.
The Web client uses the authenticated HTTP API through a loopback SSH tunnel.
Native input comes from the visible Web or Telegram client; publication actions
still require exact action approval in mode2. There is no purchase.

The existing working owner profile remains message/photo/report. Widgets are an
explicit qualified capability: existing binding snapshots and accepted method
sets are not upgraded. Default startup, production bot, production history,
budgets and delivery outcomes are not rewritten by these tests. Native evidence
and disposable databases live under
`/srv/storage/butler/agents/communication-104-20261022`; private proof, tokens,
provider environment and complete owner records are not committed.

Migration095 adds the versioned shared draft, idempotent channel action receipts
and a separate edit journal for already acknowledged Telegram messages. It adds
no execution queue, approval path or alternate source of truth. First Submit
uses the existing RespondTx and owning Input continuation. Preference DATA cannot
grant authority. Canonical answer sources are captured through Catalog before
model release and are fenced by later deletion/revocation.

## Issue criteria

| #104 criterion | Evidence |
| --- | --- |
| Pizza choice in both channels, including different nearly simultaneous answers | Native windows1/2/3 below; `TestCommunicationWidgetConcurrentCrossChannelSubmitIntegration`; actual owner HTTP/feed test |
| Draft does not resume; Submit resumes once; later answer is already_resolved | Native multi-choice draft remained open with unchanged model-attempt/response counts; Web Submit resolved `[a,b]`; `TestCommunicationWidgetDraftCASLostACKAndOneResumeIntegration`; immutable action replay receipts |
| Disabled/stale projections cannot repeat execution | Render matrix for three input kinds and five states; callback/ACK/binding validation; terminal initial delivery test; projection loss/rejection journal; native failed/UNKNOWN outcomes retained below |
| Blocking only; false rejected before commit; no forms or payment authority | `TestCommunicationStrictIntentAndBlockingQuestions`; `TestCommunicationWidgetDraftRollbackAndPolicyMatrixIntegration/false_wait`; `TestCommunicationMultiAndTextResponsesRemainDataIntegration`; action-authorization count remains zero after preference Submit |

`TestCommunicationWidgetActualWebControlAndFeedIntegration` exercises the real
owner HTTP handler, canonical encoded interaction reference, owner rejection,
shared draft, Submit, ACK replay, terminal feed and noncanonical path rejection.
It also proves the response is displayed in the question rather than duplicated
as raw internal JSON in the conversation.

## Fault matrix

| Boundary | Required result and verification |
| --- | --- |
| Every draft/Submit control write, SQL error and abrupt child process death | Entire control transaction rolls back. Question/wait/draft remain consistent; safe replay commits once. `TestCommunicationWidgetEveryControlWriteFailureAndProcessCrashIntegration` enumerates actual write boundaries. |
| Commit before lost owner ACK | Same delivery ID and body return the immutable receipt; no second response/wakeup. Changed body conflicts. |
| Two channels, duplicate callback, stale draft CAS | One winner; stale Submit/toggle rejects and refreshes. Draft updates never create transcript responses or action grants. |
| Invalid draft, missing minimum, expiry, wrong binding, source revocation | No control commit or continuation; current backend policy wins over UI state. |
| Foreign bot/user/chat, invalid callback, wrong/ambiguous retained ACK | Deny before mutation; do not guess the newest question or create a conversational Input from an invalid reply. Ambiguous text asks for an explicit reply to the intended question. |
| Edit success, lost ACK, wrong ACK, HTTP429, interrupted process | Separate sent/failed/delivery_unknown edit receipt; original publication ACK remains intact; no new send or replay of inference. `TestCommunicationWidgetProjectionFailureJournalIntegration`. |
| Concurrent edits of the same message | Per-message PostgreSQL advisory transaction lock prevents an older edit overtaking a newer one. `TestCommunicationWidgetProjectionEditsCannotOvertakeIntegration`. |
| Closed question before initial send / expiry | Qualified renderer reads current terminal state; no active choice controls. Expiry is fenced by the DB clock and the existing deadline/review path. Dormant foundation without a qualified renderer still denies. |
| Answer survives subsequent exact-action approval/recovery | Canonical private DATA remains in prepared context without duplication, instruction promotion or restored grants. `TestCommunicationWidgetAnswerSurvivesApprovalProviderRebuild` and ready-marker retention test. |
| Answer deleted before model/publication release | Real Catalog access captures `message:response:<interaction>`; deletion revokes subsequent release. `TestCommunicationWidgetResponseCapturedByCatalogAndRevokedIntegration`. |
| Privacy erase / schema rollback | Derived drafts/receipts are erased. Down migration refuses accepted widget history and requires compatible drain. |
| Web lost ACK / rejected CAS | Retry retains exact body and delivery ID. A rejected checkbox toggle visibly returns to the server draft; a new answer is not submitted. |

## Native observations, including failures

All windows retain full evidence before disposable schema cleanup. None of the
failures below was reclassified as sent/completed, and no unknown send was reset.

1. Window1 (`6f2a3df1`): Web-origin single choice was delivered to both channels,
   survived a restart after Telegram ACKs, accepted Pepperoni in Telegram and
   showed the same choice in Web. A later approval rebuild lost answer DATA and
   proposed another question. That proposal was rejected. The fix retains the
   answer in the owning prepared context. Its partial multi-choice test is not
   counted as a completed scenario.
2. Window2 (`791099ef`): actual concurrent Web Margherita and Telegram Pepperoni
   actions produced one canonical response/action; Web won. Restart preserved
   question identity, wait and model count. The subsequent model used an
   interaction locator as a Catalog source; publication was denied and the run
   remained effect_unknown. Canonical response Catalog capture/provenance was
   corrected; this historical outcome was preserved.
3. Window3 (`2866cddd` product code): a restart during an
   in-flight initial send produced `delivery_unknown/sending_claim_expired`.
   Its suffix/result remained pending behind that unknown delivery, while Web
   accepted the answer. This is the negative crash case, not a clean Telegram
   success. A separate healthy single choice delivered question ACKs12/13,
   accepted Pepperoni in Telegram, updated Web and delivered result ACK14;
   its owning Input completed without another question. Multi-choice with an
   option image delivered ACKs16/17/18: Telegram selected cheese, Web observed
   that draft and added mushrooms. Draft versions2/3 left model attempts at8
   and the question open. Web Submit committed one `[a,b]` response; Telegram
   edits for both drafts and resolution were sent, result ACK20 was delivered
   and the Input completed. Text reply to ACK21 resolved the question without
   another Input; result ACK23 was delivered, but terminal edit failed.
4. Window4 (`59ecef16`): empty inline keyboard plus force_reply opened the native
   reply and survived restart, but still failed terminal edit. Real operator
   Bot API probes proved the cause: empty keyboard ACK27 returned HTTP400
   `message can't be edited`; a nonempty inline keyboard ACK28 was editable.
   A disabled status button without callback also edited successfully (ACK29).
   The final mapping preserves a nonempty disabled status and the immutable
   force_reply flag on terminal edits. These probes are channel-contract tests,
   not canonical publications or model calls.
5. Window5 (`a076feba`): the corrected text widget ACK32 opened native reply,
   survived restart with identical question/Input/wait/ACK/model count, accepted
   the explicit reply and edited `2:1:resolved` successfully. Web showed the same
   answer and Input count remained1. The model then confused interaction,
   publication and source references in its proposed result. The proposal was
   rejected without publication. The response DATA now provides distinct
   publication_ref/source_ref, and the tool schema explains their types.

Telegram mapping is checked against
[InlineKeyboardMarkup](https://core.telegram.org/bots/api#inlinekeyboardmarkup),
[DisabledButton](https://core.telegram.org/bots/api#disabledbutton) and
[editMessageText](https://core.telegram.org/bots/api#editmessagetext).
An inactive text status has no callback and creates no new control semantics.

## Automated validation and review

- Real Linux PostgreSQL/race targeted suite: 491 passing test/subtest events,
  no failures/skips on the earlier full slice; the final PR repeats the required
  matrices and regressions on its own head. Broad environment-dependent skips
  are retained as skips, not counted as acceptance.
- Final product source `4dff78b54984f88d3a61182c8c3837af7f264977`:
  86 focused PostgreSQL/race passes, zero failures/skips. The earlier follow-up
  suite had 81 passes, zero failures/skips;
  canonical Catalog deletion and concurrent projection tests also passed.
  One earlier deletion test used an invalid operation name in the fixture;
  that failed log is retained and the corrected test was rerun.
- Web: all five widget Playwright scenarios passed, including a reproduced
  failing CAS checkbox regression before its fix. Earlier combined feed/widget
  checks also passed. Typecheck/lint and full Web CI run on the final PR head.
- Self-review covers ownership/lock order, current source/export/binding checks,
  idempotence, privacy erasure, per-ACK edit ordering, late initial delivery,
  secret masking and answer retention through approval. It is not independent
  reviewer approval. Final CI results and the review are recorded on PR136.

The final native continuation outcome and the aggregate ledger of all windows,
including failures and unobserved usage, must be recorded on PR136 before issue
closure. Source snapshots, binary hashes and original private evidence remain on
the isolated host; a later success never overwrites an earlier failed outcome.

The remaining cross-format obligations/model behavior campaign belongs to #105;
operational diagnostics/cutover to #106; full AC01–AC22/profile rollout to #107.
