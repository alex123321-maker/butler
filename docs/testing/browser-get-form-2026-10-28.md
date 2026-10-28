# Guarded GET browser navigation — 2026-10-28

Continuation of #163, stacked on PR #172. This implements public GET search
forms and pagination/result links. POST/login/OTP and opaque browser writes
retain their existing unverified-effect boundary and are not accepted here.

## Behavior and evidence boundary

`browser.submit_get_form` compiles named ordinary fields into a same-origin GET
URL. `browser.follow_link` follows a same-origin anchor by GET. Both use private
two-phase planning under frozen network, independent Go public-target checks,
one-use nonce, live document/version/URL checks and bounded GET/HEAD navigation.
They never run click/input/formdata/submit handlers. Hidden/password/file fields,
credentials, sensitive field names and private drafts are refused. Observations
contain bounded form selectors/methods/actions/field names/types, never values.

Any prior local/live fill/type or owner takeover permanently taints that finite
context for these new read tools. They cannot thaw an ordinary private draft
merely because its value was not a credential reference. Existing preparation,
opaque interaction and authorization contracts retain their meanings. New
independent reads do not reconcile a previous unknown mutation or revive its
grant. nav-02's original browser.fill remains effect_unknown/uncertain.

Unit checks passed for runtime/ActionAuth/registry/Broker. Real isolated PG/race
checks passed strict read-contract/release-consent validation and independent
GET admission with an old unknown write, followed by denied mutation and retained
spending/grants. Actual Chromium checks passed GET query, pagination, rejected
POST/hidden/password/file/private/foreign/credential/stale cases, and no private
draft release, form handlers or POST callbacks. The real Broker→PG→runtime→
Chromium test passed these reads and existing owner-control/cancellation fences.
Vet and Linux builds passed. Opt-in PG/Chromium checks were run explicitly;
ordinary unit-suite skips are not counted as their evidence.

The first real Broker test exposed stale fixtures: migration096's observed_url
was missing, and a delayed snapshot lacked that now-required URL. Both fixtures
were corrected to the actual owning schema/receipt. The initial dictionary field
schema was also incompatible with Broker's boolean additionalProperties subset;
the public API now uses an array of `{name,value}`, independently bounded by the
runtime. Failed logs were retained; only the corrected checks count as passed.

## Lab rollout

Only Broker/browser services were recreated with an additive Compose override.
Core, model/profile selection, provider encryption configuration, database schema,
other services and previous release directories were retained. The new catalog
was merged from the deployed one, adding/updating the two read contracts and
open guidance while preserving existing routing/domain caps and unrelated tools.

Current binary/helper/catalog hashes are in
[release-sha256.txt](browser-baseline/get-form/release-sha256.txt).
Deployed source: `1e24a910`; owning release:
`/srv/storage/butler/main/releases/browser-get-form-20261028-1e24a910`.
The current service definition is base `/srv/storage/butler/main/compose.json`
plus that release's `compose.override.json`. Using the base alone restores the
older service configuration; retain the overlay for this candidate. The
immediately preceding overlay/release was retained for rollback. Startup health
and the executable hashes inside the running containers were checked.

The first rollout failed because mounted code directories were mode0700 and the
container's `butler` user could not traverse them. Automatic rollback restored
the healthy original services. Mounted code directories were corrected to0755,
with the host release parent still private, before a successful rollout. No
credential/config permissions were broadened and no owner work was running.

## Native results, including failures

The original matrix's nav-03 stopped with invalid_tool_request after three
tools.search calls and before any browser call. Four model attempts used23,956
input/728 output tokens. This is retained as a native FAIL, not attributed to
GET-form execution. The original matrix now has6/15 attempted (three read PASS,
three nav FAIL), nine slots unexecuted.

Separate controls use the same ordinary Russian search request, no supplied
tool calls/selectors/route hints, fresh private recipient instances and the same
owner/session/model/autonomy/budgets. They do not replace old matrix slots:

- control01 used the new GET form successfully (~0.350s), then requested an
  opaque pagination click. That proposal was rejected via the normal owner API
  (one intervention); no click was dispatched. This led to the explicit read
  follow-link operation. The attempt is not a full navigation PASS.
- control02 stopped on provider_unavailable before any tool; one model attempt
  has unknown token/cost usage. The API showed stored credentials with expired
  access, not proof of backend availability. A credential-free OAuth endpoint
  connectivity probe returned404, establishing reachability only.
- the existing private `provider-auth -action diagnose-auth` then refreshed
  OAuth with HTTP200 and persisted it encrypted, resolved=true,
  unsafe_refresh=false. No inference/account switch/import or model selection
  occurred; no owner action or separate “Manager” application was needed.
- control03 completed the real GET search→pagination→record→publication→Telegram
  flow with zero in-run owner interventions. The fresh exact record reference
  matched the protected publication. Native tools included GET form, two link
  follows and close; subsequent publication validation caused an extra web.fetch
  before final delivery. All read/grant/effect history remained separate.

control03 delivery was102.498s; Input completion105.683s. Six browser calls took
~2.437s in total (not wall time), including1.064s for GET form plus the two link
follows. There were16 model attempts,180,205 input/1,472 output tokens. Monetary
cost is unknown. The answer-only credential-canary scan passed; full privacy
qualification is not claimed. The recipient recorded three record reads;
receiver counts alone do not attribute every request to a particular tool.

Two further revision1 navigation trials (control04/05) were fixed in advance
to assess stability after control03, with no extra retries to replace failures.
Both failed; no extra retries replace them. Cold/warm behavior differs:
control03 started the deployed Chromium generation; subsequent trials reuse its
process with fresh empty contexts and recipient instances.

| Revision1 trial | Native result | Input duration | Model attempts | Input/output tokens |
| --- | --- | --- | --- | --- |
| control03 | PASS: exact record + protected publication + Telegram | 105.683s | 16 | 180205 / 1472 |
| control04 | FAIL: effect_unknown, no publication | 51.081s | 6 | 44322 / 674 |
| control05 | FAIL: effect_unknown, no publication | 51.537s | 6 | 43381 / 613 |

This is 1/3 native completion on this revision, not a reliable navigation release.
Both failures supplied the future search URL as expected_url instead of the
current observed /catalog. Runtime correctly rejected before navigation; Broker
replaced the owning diagnostic with uncertainty and Core stopped. The old
revision1 effects/Inputs stay failed/unknown; they are not rewritten by a fix.

## Revision2: prove a read was rejected before navigation

Both new GET tools now pin revision2 / guarded_browser_get_v1. A correlated
failed runtime response with exactly read_rejection=pre_navigation_v1,
navigation_dispatched=false and a fixed reason can establish local non-dispatch
of THIS read. Broker sanitizes the diagnostic, allowing observation/reassessment.
PG rechecks the owning read contract, dispatch/call/source correlation and a
normalized digest of the fixed proof. The read becomes effect_absent with failed
tool status; its grant stays consumed and spending is retained. This does not
reconcile any previous effect or confirm any user task. Old contracts, opaque
writes, missing/foreign proof, transport loss and navigation already started
keep unknown outcomes.

Targeted units passed all four changed packages. Real PostgreSQL/race checks
passed the proof validation/forgery cases plus retained old-write isolation.
Real Chromium passed rejected future URL -> zero query reads -> correct GET.
Real Broker/PG/Chromium passed absent failed read, denied grant replay, fresh
snapshot, correct GET and follow-link, plus owner/cancel regression guards.
An initial PG test correctly rejected absence because its persistence verifier
had not yet been extended; the owning revision2/digest check was added and a
fresh test passed. Prior failed test logs remain retained. Revision2 deployment
and native outcomes are recorded after verification below.

Forms/login/consent remain unaccepted. Their nine original slots must not evade
the retained opaque nav-02 effect via new session/Task identities. Full #163 and
general browser-assistant acceptance remain incomplete.

## Reproduce narrowly

Use scripts/dev/server-browser-lab.sh OWNED_ROOT browser/broker for the real
Chromium and PG/process gates. For portable PG/race use
scripts/dev/stopped-local-tests.ps1 -Run '^TestGetForm|^TestBrowserGetForm|^TestGuardedBrowser'.
The mandatory Communication CI selector includes the new PG tests. Native
controls use the existing browser-eval native ingress observer, stable delivery
IDs, new report directories, bounded5m observation and protected verify-fixture
comparison. Public artifacts are under browser-baseline/get-form; private
configs, bearer copies and raw publications never enter Git or model context.
