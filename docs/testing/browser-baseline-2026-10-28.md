# Browser baseline: recipient and deployment inventory

First implementation slice of #162 under plan #161. This is preparation for
native measurement, not completion of #162 or acceptance of a browser agent.

## Executed evidence

On 2026-10-28 the read-only inventory reached `laba`. Core, Broker, browser and
Web were running and healthy. Core/Broker exposed tool discovery enabled, and
Broker exposed public web reads enabled. Image IDs and capture time are retained
in [the content-free inventory](browser-baseline/deployment-20261028.json).
All four source revision labels were absent. Image IDs identify observed bytes,
but cannot prove a source commit, migrations, effective model/effort, exact
per-run tool catalog or autonomy policy. Those remain unverified. No deployment
or configuration was changed by this inventory.

The new standard-library [recipient](../../cmd/browser-eval/README.md) has these
independent HTTP checks, all PASS through the Windows repo-local Go workflow:

- protected data requires actual login; wrong credentials fail; contexts isolate;
- applied form plus lost HTTP response retains an accepted event, and a second
  POST is recorded as another effect rather than hidden by fixture idempotency;
- no config/oracle/journal route, credential query rejection, catalog pagination;
- journal failure prevents login and cookie creation;
- private config is strict and existing config cannot be overwritten.

`go test -race -count=1 ./cmd/browser-eval` and `go vet ./cmd/browser-eval` passed
with Go 1.27.2. `scripts/dev/verify-changed.ps1` maps this command to its targeted
checks. The fixture uses no new dependencies and changes no product authority,
runtime contract, provider auth, PostgreSQL state or unknown-effect semantics.

Actual Chromium 136.0.7103.25 / existing Playwright 1.52.0 also passed the direct
scripted recipient smoke: public reference, catalog search → page 2 → record,
delayed form help → save, unauthenticated redirect, login → private reference.
The private journal contained exactly five accepted events in order:
`read, record_read, form_submission, login, account_read`.
The journal excluded the synthetic username/password and protected answer.
No screenshot, trace or DOM export was produced. This journal check is not a
complete product leak audit because no model, Broker or channel participated.

Private configs/journals and the content-free smoke report stay under the
worktree `.cache/browser-eval/recipient-*`; they are excluded from Git. The smoke
reports `scope: direct-chromium-recipient`, `native_agent: UNEXECUTED` explicitly.

## Remaining #162 gates

1. Establish source provenance and effective production composition using safe
   owning diagnostics; do not dump Config.Env, settings or auth material.
2. Expose an isolated attempt-scoped HTTPS fixture origin through ordinary
   public egress. The loopback development listener is intentionally blocked by
   production Butler; no SSRF exception or network policy weakening is added.
3. Join ordinary ingress/Input/run receipts, model/tool timings, independent
   recipient events and actual channel delivery. Keep the oracle outside model
   context. A request reaching the site is not proof of correct model output.
4. Execute three attempts each of the five documented prompts. Case 5 is a real
   protected Butler consent interaction using the same owning Input, not a site
   simulation. Test credentials enter only the protected vault.
5. Record all successes/failures/timeouts/unknown effects, cost (unknown stays
   unknown), owner waiting and interventions; compare against the short manual
   path. Initial baseline must remain usable even when #163 exposes a blocker.

At the end of the first slice no native model attempt or external account login
had been run. The follow-up below adds one separate native control; OTP, separate
login origin, complete native matrix and latency/cost aggregation remain pending.

## Follow-up: one real public control

The follow-up added a Go native ingress observer, exact-attempt metadata collector
and fixed public-result verifier. It used ordinary owner Web ingress, actual
provider, Tool Broker, browser runtime, publication and channel delivery, without
injecting expected answers, selectors or tool calls. The queue was idle before
submission. No approval was auto-answered and no provider auth was copied.

One control requested opening `https://example.com/` in the browser and reporting
its title and domain purpose with a link. It is **outside** the planned controlled
3×5 fixture matrix; that matrix still has zero native attempts.

The native control Input completed. The [exact-run metadata](browser-baseline/control-01-metadata.json)
records two tools.search, one tools.load, one browser.open and one
communication.message, all completed with no stored errors and no pending approval.
It records model profile `antigravity / gemini-3.8-flash-high`, six model attempts,
44094 input tokens and 450 output tokens, with zero unknown usage pairs at the
final observation. These are reported counters, not a monetary invoice or a new
claim about undisclosed reasoning/cache accounting. Monetary cost remains unknown.

| Measurement | One observed control |
| --- | --- |
| Accept → first recorded model attempt | 14.37 s |
| Accept → first browser call begins | 45.66 s |
| Browser call duration | 2.01 s |
| Accept → publication created | 73.38 s |
| Accept → Telegram delivery updated as sent | 74.66 s |
| Accept → Input completed | 79.69 s |
| Runner start → completion observed by polling | 81.17 s |
| Sum of five recorded tool durations | 3.07 s |

The tool-duration sum is not a complete wall-time decomposition. Provider request
latency, admission/queue time, context preparation and uninstrumented intervals
are not separated here. This one run indicates most observed latency occurred
outside the actual browser call; it does not establish typical latency or p95.

Web had `result_ready`, which is availability rather than proof of a client read.
Telegram had `sent`. A later protected owner GET returned 200 for the exact
publication. The [content-free rubric result](browser-baseline/control-01-verification.json)
confirmed the title, source link and documentation purpose in the published text.
Together these qualify this narrow public-read control; they do not establish
account login, form handling, full privacy acceptance or general task quality.
Raw response text, model context and secret material were not exported.

### Executable provenance correction

The process binaries are mounted under `/candidate`; image identity alone is
insufficient to identify the executing code. The manifest now includes PID 1
executable hashes where readable, retaining null when unavailable. See the
[updated inventory](browser-baseline/deployment-executables-20261028.json).
Core's copied executable build metadata reported revision
`9974b301a44c6a95e203e827de34c818c6d91bbb`, `vcs.modified=true`. This is a dirty-build
provenance hint, **not** proof that deployment matches that commit or current main.
The control and verifier binaries are retained separately on the task root.

The controlled native form/login tests still need an isolated externally
reachable HTTPS origin; a domain has been requested from the owner. Loopback
remains blocked by production egress. No workaround weakens that boundary.

## Follow-up: assigned devops host prepared

The owner explicitly selected SSH alias `devops` for the fixture and authorized
adding a test subdomain in REG.RU. On that host (`46.8.21.229`, Ubuntu 24.04),
the isolated recipient is installed under `/opt/butler-browser-eval` with a
dedicated non-login system user and the bounded systemd template in
`deploy/browser-eval`. This uses the existing Nginx host without adding a Docker
installation or modifying Butler's Compose topology. Certbot 2.9.0 was installed
from the host's configured Ubuntu packages.

The `infra-smoke` instance is active on loopback port 18097. The local HTTP root
returned the expected heading; this is an infrastructure check, not a native
attempt. Its private synthetic config and journal live only in its mode-0700
directory on devops. Secure cookies are forced for deployment behind TLS without
trusting forwarded headers. Targeted Go race/vet and the cookie regression pass.
`systemd-analyze verify` and actual service startup passed.

At that stage no DNS record, public virtual host or certificate had been created.
REG.RU required a login CAPTCHA; action-time confirmation was requested through
the browser workflow. The existing Nginx sites were unchanged.
`nginx-https.conf.template` is prepared for the actual selected domain after DNS
verification and certificate issuance. Public form/login native attempts remain
UNEXECUTED until those gates are complete.

## HTTPS enabled and limited native matrix

After the owner confirmed the CAPTCHA, it was solved through the normal REG.RU
login UI. A single new A record `butler-eval.report-template.ru → 46.8.21.229`
was saved and subsequently confirmed on both authoritative nameservers and
1.1.1.1. Existing zone entries/TTL and existing Nginx virtual hosts were retained.
Let's Encrypt issued a public certificate expiring 2027-01-26. HTTPS root returned
200 from `laba` with ordinary certificate validation; `/oracle` and `/private.json`
returned 404. There was no egress exception or TLS-warning bypass.

The new HTTPS host proxies only the loopback recipient, disables access logging,
bounds request bodies/timeouts and supplies noindex. Certificate renewal has a
domain-specific Nginx reload hook after syntax validation. Captured UI proof is
kept locally at `.cache/reg-dns-created.jpg`; private config/key material is not
included in Git. Certificate private keys remain under the root-owned host path.

Each native attempt used a new synthetic config and separate recipient process,
journal and browser context. Four of the planned 15 matrix Inputs were submitted:
three reads and one search/navigation. The separate example.com control is not
counted among these four. [Content-free evidence and summary](browser-baseline/matrix/summary.json)
join exact Input/run refs, tool events, protected publication comparison and
delivery state. The native driver preserves attempted/failed/unexecuted states.

| Attempt | Result | Telegram delivery from acceptance |
| --- | --- | --- |
| read-01 | narrow read PASS | 68.505 s |
| read-02 | narrow read PASS | 72.351 s |
| read-03 | narrow read PASS | 52.651 s |
| nav-01 | waiting for exact fill approval, then failed at guard | no result publication |

All three read answers contained the exact fresh reference read from that
attempt's private fixture configuration. Protected owner publication GET passed,
the model used one browser call per Input and Telegram recorded sent. The
answer-only canary check found no fixture username/password. This is not a scan
of all provider input, logs or memory and is not full credential acceptance.
Median observed Telegram delivery latency was 68.505 s (range 52.651–72.351 s,
n=3). Tool-duration sums were 1.299–1.731 s, with 6–8 model attempts per read.
These limited data identify substantial overhead outside the browser call but
do not isolate model/admission/context/queue time or establish p95.

Recipient request counts are **not** automatically attributed to the model:
read-02's server journal had three read events for one browser tool call. The
open test origin can receive other requests. Actual returned fresh-reference
comparison and current-run browser receipt are therefore also required; the
test does not claim three separate agent actions from that count.

### Concrete blocker for #163

Navigation made 13 model attempts, including repeated discovery and an
http.download, before requesting exact `browser.fill` approval for the search
field at `/catalog`, value `Arctic delivery`. Without intervention it was
`waiting_for_user` at about 78 s. The operator reviewed that exact public-site
search action, approved it through the ordinary authenticated owner API and
recorded **one intervention**. No broad mandate or policy override was created.

The same Input was observed using the GET-only resume path, without another POST.
`browser.fill` then returned execution_stopped with validation reason
`stopped_attempt_uncertain`; the Input transitioned through blocked to failed.
Read-only inspection found four pre-existing uncertain stopped attempts in the
same owner/session, not this new navigation run. This matches the receiving
budget guard in `internal/budget/stopped_attempt.go`, which permits reads but
denies ordinary effects across that retained uncertainty even after exact
approval. The fill did not reach runtime execution.

This demonstrates a blocker in the **current deployed composition**, not a
failed search algorithm, bad site credentials or proof that every fresh isolated
composition fails. The matrix stopped immediately; remaining 11 slots are
UNEXECUTED, not failed or passed. No old UNKNOWN was erased/reclassified, no
budget refunded and no new session/run was used to evade the guard. General
browser-assistant acceptance is **NO_GO**. Formal outcome verification and
supported reconciliation/continuation belong to #163 before repeating effects.

The failed recipient was stopped only after Input failure was confirmed, keeping
its journal and config. A separate `infra-ready` instance now serves the public
root; it is not a matrix attempt. Source provenance limitations above remain.

### Harness corrections and reproducibility

Before read-02 submission, systemd reported active just before its socket was
ready. The matrix stopped with no read-02 Input accepted. Readiness now waits at
most five seconds; guarded recovery verified an empty journal and zero accepted
Inputs before the first actual submission. This infrastructure failure is not
hidden as an agent failure or counted as a native attempt.

Accepted form receipts now record bounded quantity, enabling rejection of an
incorrect valid quantity or duplicate submit. That instrumentation was deployed
for future recipients; it does not change prior read results. Fixture search
also accepts case-insensitive `Arctic delivery` as well as `Arctic`, correcting
an overly narrow fixture condition found during review; no search submission
had executed in nav-01. The updated query behavior has not been native-qualified.

`browser-public-matrix.ps1` handles the ordered public subset and stops on a
noncompleted Input. `browser-fixture-verify.sh` compares private expected values
and recipient receipts inside the operator process, exporting booleans/counts
only. `browser-baseline-summary.ps1` recreates the checked-in summary and refuses
an existing output file. Go race/vet, direct Chromium smoke and oracle-negative
tests passed after these changes. Forms, login, OTP, full privacy scan, complete
cost/latency breakdown and the remaining matrix slots remain unfinished.

## Reproduce

The next #163 slice formally recovered four historical stops and ran nav-02.
That attempt reached browser.fill but produced a new actual effect_unknown,
so the matrix remains stopped with ten slots unexecuted. See the
[continuation report](browser-continuation-2026-10-28.md) and its exact-run
evidence; the original 4/15 snapshot above is retained as historical evidence.

Follow `cmd/browser-eval/README.md` for configuration, serving, unit checks and
direct Chromium acceptance. On `laba`, the deployment inventory command is:

```sh
bash scripts/dev/browser-baseline-manifest.sh /path/to/new-private-report.json
```

It reads the four named containers and only exports image/start/health, optional
source revision and the two allowlisted booleans. Missing env flags remain
unavailable, not an assertion of their effective value. Existing output files
are rejected. The inventory is separate from test reports and native acceptance.
