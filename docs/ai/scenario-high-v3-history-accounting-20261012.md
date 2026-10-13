# Closed V3 High history: 266 spent attempts

`readScenarioHighV3History()` is a new pure reader after the unchanged
`readScenarioHighHistory()` 248-call view. It reads only the two exact committed
content-free V3 artifacts. No provider, private report, credential, database
mutation, scoring or window admission is involved. The separately reviewed
[two-context diagnostic](scenario-structural-json-diagnostic-20261012.md) now
uses this pure view only as its pinned266 baseline. Its own finite approval,
counter and deadline govern new entries; the reader grants no authority.

The required V3 audit SHA256 is
`2561b8486c40ee3d2d23d01709b5b06962f3407165c54133e04ab5bd29cd9a16`;
the required independent PG correlation is
`805a168c42e8531a0965994720a776aa505fb7d917e1f908b7b66338dfa9b5e2`.
The reader checks regular-file exact sizes/bytes and binds original private
report `2e35fa16…`, SQL witness `01bbc2c1…`, actual prepared release `b4f59489…`,
source `ffde6d8f…`, runtime `f022b2b9…`, hardware `d3632dff…` and the frozen
V3 corpus/schema/policy/resources. Failure, NO_GO, incomplete quality and absent
comparison remain required; equality cannot establish inference or acceptance.

All 18 ordered ordinals 249–266 survive, including 17 completed observations and
the final known-cost `transport_error`. Every new version is
`gemini-3.8-flash-n` with the separately requested High profile. New measured
token subtotals are 9,799 input / 377 output / 21,381 total. The last failed entry
retains 526 / 9 / 807 tokens rather than being dropped or changed to NULL.

The resulting history contains **266 calls, 240 known input/output pairs and
26 NULL pairs**, with known input/output subtotals **154,345 / 5,914**. The known
total-token subtotal is **271,108** with **46 unknown total-token attempts**
from prior windows. The existing historical DTO stores input/output pairs;
total-token measurements and their unknown count remain in the exact pinned
artifacts. No missing total is zero and no subtotal is full historical cost.

Artifact 18 is the unchanged V3 safe audit. The new baseline hash is
`fa9cf46d603db8c2b9155e32d49f0e5bde41b0fdfa39bc95cf9a8a7a74b84d03`:
SHA256 of the complete prior 248-call hash, original V3 report, safe audit,
SQL witness and correlation hashes, each separated and terminated by LF. Prior
45/228/248 readers, artifacts, usage, deadlines and verdicts remain unchanged.
This fold grants no refund, retry, future scope, quality claim or execution.

Pure regressions check exact accounting and old NULL preservation, the known
failed entry, missing/changed artifacts, omitted/reordered entries, false PASS,
changed cost/version/source/schema/policy/history/parent and false PG claims.
They run only on isolated laba Docker with race/vet, network disabled and zero
native calls.
