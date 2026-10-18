# Issue88: complete temporal native v5 evidence

The new frozen twelve-original cohort completed mechanically. Independent
[semantic and citation-entailment review](issue-88-native-v5-independent-review.md)
found11 passing originals and one unsupported acceptance-state assertion:
**NO_GO**, release qualification remains false. The earlier [v4 NO_GO](issue-88-native-v4-independent-review.md), its
answers, corpus, rubrics, costs and nullable observations remain unchanged.

## Exact frozen window

- Source: `87ab471e8aa84789d0ff9e56ccff3c789afe0ccf`.
- Server evidence: `/srv/storage/butler/agents/issue88-20261016/reports/native-v5-r5`.
- Prepared-pins SHA256: `61b0f0c41426f4d6d5515133736daf33fe3620076000b099d5d44c64dfb8ec6b`.
- Collector SHA256: `2654ced2a92d283e0bebecb1ff3f4df3da432cc0675994732e9bf4380f5decc0`.
- Broker SHA256: `8ab7816aa2c7b55e3940561992402cc15ca3ea6c9b1e6d8dc6efde8d3b68f38e`.
- Standalone standard Go test2json SHA256: `8f352c19877e12d5b0305b9eebac875aa63734fe03b73127ee01fae2c833ad68`.
- Historical/current manifest SHA256: `50bcbf547df876cd92c36571336dd8a558bbf688bec5c81729144146c39b50d4`.
- Facts contract revision2 SHA256: `535e5d4730124467b5603d99e5f95e30fccd93627aa44b2c86de719ef868bd5c`.
- Complete twelve-slot schedule SHA256: `3701076fecd4cdc13074c09800b5f92ab2dbf47d8859cf9a26ac46c10ec22b30`.

The original corpus, questions, frozen rubrics and ordering are unchanged.
The prospective capacity remains 144 model entries and 96 tool calls, at most 12
entries/eight tools per original, with High 32768 output and 300000 ms model timeout.
The current UTC day is 2026-10-18, with the original deadline 2026-10-19T00:00:00Z.
The execution used the separately reviewed seal and a fresh serialized main auth
clone. Every actual model entry retains its durable operation/reservation IDs,
with owning foreground-row corroboration and private-cohort FK/unique checks.

## Mechanical observations and independent usage populations

| Observation | Result |
| --- | ---: |
| Scheduled / completed / failed / not executed originals | 12 / 12 / 0 / 0 |
| Actual native entries: known / unknown / missing full total | 36 / 0 / 0 |
| Known input / visible output / full native tokens | 192527 / 13346 / 248587 |
| Product foreground attempts: observed / NULL | 36 / 0 |
| Admitted product attempts without a native entry | 0 |
| Confirmed Broker effects: facts / original opens | 13 / 11 |
| Provider time / owning execution time | 394682 ms / 405500 ms |
| Serialized native request bytes | 225607 |
| Invoice cost | unknown |

The current inclusive owner/day scope conservatively retains 38 model turns,
41091072 tokens, 14280000 ms, 318767104 provider-input bytes, 24 tool calls and
100663296 egress bytes. This is the exact prior89 two-turn quoted vector plus
the 36 new model quotes and 24 tool quotes, imported once. Reserved dimensions
are not measurements of actual network egress or invoice cost.

The older-day immutable history remains 161 original model attempts, 153
native-known/eight native-unknown, 758525 known full tokens as a lower bound,
and separately 152 product-observed/nine NULL attempts. Its 137-turn temporal-v4
inclusive leaf is retained once. It grants no current authority and was not
rebilled into today's scope. The original current-day89 cohort separately
retains two native-known/zero native-unknown entries, 15625 full tokens and two
observed product attempts. Its earlier NO_GO and unexecuted denominator remain.

## Controlled preflight findings and preserved unsuccessful starts

The first unpaid freezes were withdrawn before auth after independent controlled
review reproduced inconsistent usage aliases, invalid native usage and valid
usage totals diverging from their original immutable witnesses. A subsequent
controlled baseline also reproduced coordinated omission of nullable fields
from both prior aliases and current metadata. Exact safe product keysets,
complete scope/reservation equality and both original native witnesses now deny
all these mutations. The omission baseline JSON SHA256 is
`c8d5d65265a23b737a1bf64598472ab2eea5254356c9127dbaea5c1dc0c02089`.

The r3 private-PG carry passed; its added tool-flow control initially failed
before provider setup because the runner omitted the sealed Broker path. That
runner wiring was corrected before r4. R4 then passed the controlled PG flow,
but its nonroot `go tool test2json` process failed to initialize `/.cache` before
the collector started. All twelve r4 slots remain not executed, with zero native
entries, no v5 schema/window receipt and unchanged six-database attempt inventory.
The unused opaque clone was returned by exact CAS. A separate proxy/upstream
timeout temporarily left main HTTP200 pending; that fact is preserved in its
original abort report and resolved by a later restoration receipt.

R5 builds and hashes the standard runner offline and tests its exact nonroot
execution before auth. Actual private PostgreSQL/race controls passed both the
zero-delay and five-second facts→open→open→final flows, exact operation/run/
reservation references, foreign/missing-reference denials and one-use carry:
four test results passed, zero failed/skipped. JSON SHA256:
`47395f6aced7e8770405e2619b3109b09cd26a5236fa3af143ff777272e3e0f0`.
Six additional fallback controls passed using only synthetic private bytes and
actual unchanged six-database metadata: positive unused return plus repeat
denial, collector-start denial, mismatched import, symlink, public permissions
and oversized input. No real auth or native call was used. Report SHA256:
`50629c1e3c5d68805372a1c37566ba7c935ee95a5437729de734dd3497c14124`.

## Proxy recovery and final auth handback

The original proxy process/listener recovered after reboot, but all five tested
public HTTPS destinations stalled after CONNECT 200, including GitHub and the
exact provider host. The error was retained as proxy/upstream unavailable,
independent of connected/nonexpired auth status. A cached alternate XHTTP peer
passed GitHub 200, example 200 and unauthenticated exact-provider-host 404 on an
isolated loopback 18081 proxy. The primary 18080 configuration was then restored
with a private exact-byte backup and rollback, preserving the application
deployment, container identity and model runtime. The temporary trial was
removed only after checking its created ID/label. New subscription material
remains private and was not used to change the transport/runtime.

After the complete r5 window, exact encrypted-auth CAS, actual main
paid-discovery HTTP200 and a fresh private main snapshot all passed. The common
auth lock was released. No main memory feature was enabled.

## Review artifacts and acceptance boundary

[Safe complete review JSON](fixtures/issue88/native-v5-review.json) retains every
answer with its UTF8 hash, sealed fixture, frozen rubric, tool/result receipts,
source-reference checks and native operation/reservation/usage observations.
It omits credential material, raw transport events/errors and hidden reasoning.

- Safe review SHA256: `b05089d6c911f1b20b10501b3e8f4b8b931541bb331f33923ccd9745afb66b88`.
- Full ledger SHA256: `954157ed614adbe3246064c9a79983739b295fff7cdda353f2b02bb929414e28`.
- All-original outcomes SHA256: `6564c52b6641a62e7023df5ae17c81211a391e99497b3c4dbe611142268c11cf`.
- Read-only Broker receipts SHA256: `bd461684bb4a4b4198697eb22b243fdeca04bfbce0b78d9c90ec68aad2c68ae0`.
- Evidence hash-list SHA256: `11dbc489296fd02cc8ab75e7a6aa9d9bdea669b6724ed60a142e2bcf2bcbeca1`.

These observations cover explicit exact-reference temporal lookup. They do not
qualify adaptive retrieval, natural-language personal-history recall or overall
issue77. The immutable safe JSON retains its original pending-review fields;
the separate independent review records the final NO_GO without rewriting it.
