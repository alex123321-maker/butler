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

No DNS record, public virtual host or certificate has been created yet. REG.RU
requires a login CAPTCHA; action-time confirmation was requested through the
browser workflow and remains pending. The existing Nginx sites are unchanged.
`nginx-https.conf.template` is prepared for the actual selected domain after DNS
verification and certificate issuance. Public form/login native attempts remain
UNEXECUTED until those gates are complete.

## Reproduce

Follow `cmd/browser-eval/README.md` for configuration, serving, unit checks and
direct Chromium acceptance. On `laba`, the deployment inventory command is:

```sh
bash scripts/dev/browser-baseline-manifest.sh /path/to/new-private-report.json
```

It reads the four named containers and only exports image/start/health, optional
source revision and the two allowlisted booleans. Missing env flags remain
unavailable, not an assertion of their effective value. Existing output files
are rejected. The inventory is separate from test reports and native acceptance.
