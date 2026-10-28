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

OTP, separate login origin, complete native ingress driver and latency/cost
aggregation are not implemented in this first slice. No native model attempt
or external account login has been run; there is no agent quality/speed score.

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
