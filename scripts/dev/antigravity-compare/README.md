# Private, bounded Antigravity comparison

This operator diagnostic is outside production transport and Butler tool execution.
It compares the installed CLI 1.3.3 with the actual Go Provider. It does not deploy
code, run delegation, reopen baseline13, install/update clients or change accounts.
Only the original imported account is admitted. The owner explicitly authorized
the current live continuation; see `docs/ai/antigravity-next-experiment.md`.

Run only on SSH laba under `/srv/storage/butler/agents/antigravity-compare`, after
isolated source Sync. `antigravity-compare-lab.sh ROOT tests` runs race tests and vet
with a disposable actual PostgreSQL database and no Google credentials/inference.
`inspect` resolves the retained OAuth privately and permits read-only metadata,
but blocks **all** generation dispatch. `BUTLER_COMPARE_CATALOG_ONLY=1` selects
the CLI's read-only `models` command. `execute` additionally requires explicit
`BUTLER_COMPARE_APPROVED=1`, an atomic single-use marker, and PostgreSQL admission.
`native8192` is a separate single-use, native-only check: it verifies the exact
completed pair's artifact hash, starts at20, admits only attempt21 with8192, and
preserves the original serialized production body. The CLI is not launched.
Both live windows have completed and are sealed. The marker cannot be removed
to retry. These hardcoded windows are not
a general purpose model runner; a subsequent experiment must carry its results
and spending forward instead of creating another baseline18 ledger.

The private backend imports no new authorization. It checks four fixed origin
booleans against the encrypted production credential using the existing Manager.
Normal OAuth refresh may rotate and persist that credential in encrypted PG;
this is the sole routine production-state write. The backend gives a private CLI
profile the same short-lived token. It does not show credentials or profile data.
Private files are UID10001, directory700/file600; Docker environment inspection
stays in `private-env.mjs`. The CLI receives only its isolated HOME/work/logs and
public CA/prompt/model argument. Production PG/proxy environment and diagnostic
control key are **not** mounted into the CLI. Never read its raw output/logs into
model context. Only `cliDiagnostics` fixed flags/public model IDs are exported.

CLI networking is a separate internal Docker network. Its only outgoing gateway
is a fixed-host CONNECT proxy terminating private TLS. Read-only Google metadata
is allowlisted; OAuth refresh, purchase/onboarding, arbitrary hosts and extra
generation are blocked. CLI credit use, telemetry, skills, plugins, MCP,
history and user files are disabled. Its custom agent requests empty tools, but
CLI still advertises one declaration: the backend blocks any function proposal
before returning it to the executor. Telemetry/update/ACL writes get inert local
responses. Actual feature-flag GET is forwarded to avoid inventing routing flags.
The backend reaches the existing private operator proxy through the original
backend network. Google generation is admitted only after exact model, JSON,
prompt, credits, bearer, route and byte/deadline checks. The private profile
avatar URL may be fetched only when registered from Google's userinfo response,
with no bearer forwarded; its URL/image/identity are never in safe evidence.

PostgreSQL reserves attempt19/20 before the upstream write. Each client is UNIQUE;
concurrent duplicate, timeout/cancellation, process restart and an expired window
never refund an admission. All five historical reports are hash-checked before
admission and mounted/read unchanged; baseline13 is read-only. The generation
window lasts120s after metadata preparation, request45s, request/response64KiB,
output512 each, maximum2 requests and supplemental-cost policy0. Conservative
input reservation is1,048,576 tokens each; unknown measured usage and monetary
cost remain NULL. This implements the owner's finite comparison, not a claim
that subscription requests are free or their quota counter is exact.

The gate deliberately lowers each serialized maxOutputTokens to512; production
Go remains8192. It drops compression/replay headers so Go negotiates and decodes
gzip correctly without re-sending admitted POSTs. Redirects are refused. Context
and route/model/context differences between clients remain confounders. The
actual CLI catalog/capture confirms medium; it rejects tiered as a model argument.
Native Go remains tiered. A missing display alias does not broaden the admitted
wire slugs. Wrong wire model is always rejected before
Google. Read-only metadata can fail independently; execution never changes the
original account/model or substitutes a route after failure. The observed8192
request failed HIGH_DEMAND; earlier512 success was minutes before and does not
isolate the output allocation as a cause. Do not reduce production8192 on that
correlation alone. The original native8192 artifact contains a stale safe CLI
diagnostic from the prior pair; CLI was not launched in that window. New report
projection explicitly ignores cached CLI output in native-only mode.

Safe reports retain statuses/reasons from fixed allowlists, marker/finish enums,
nullable usage, public endpoint/model, structural booleans and timestamps. Raw
headers, errors, responses, thoughts, account/project/request IDs remain private.
The diagnostic ledger's SQL dump and PG data, safe backend log and report remain
under the new report directory after containers/networks are removed. Builds,
tests, Docker and private credential processing happen only on laba, never in
the user's primary checkout or main deployment.
