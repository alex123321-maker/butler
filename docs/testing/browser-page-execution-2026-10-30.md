# Managed page execution evidence — 2026-10-30

Scope: #164 exact page authority, runtime and owner UI wiring, stacked on the
migration107 ledger. This is same-origin and credential-free. No native
model/provider call, Google sign-in, shared lab deployment or merge occurred.

## Timing and evidence boundary

The operator work window began **08:18:55 UTC / 15:18:55 Tomsk**. It includes
model deliberation, implementation, failed iterations, build/setup, validation
and CI waiting until the final report. It is not a website-login latency.
Previous native slots were not replayed to reconstruct metrics.

Mechanical test timers include the whole named fixture, not only browser RPCs;
their `model_calls=0` explicitly excludes native-agent latency claims.
Local reports remain in `.cache/browser-page-execution-ci/`; each owner UI run
gets a fresh directory and earlier failures are retained.

| Qualified local evidence | Wall time | Result |
| --- | ---: | --- |
| Windows real Chromium, seven page cases | 5147.648ms | PASS |
| Actual Web/Core owner API/Broker/PG/resource/Chromium fixture, including owner UI wait | 19045.907ms | PASS |
| PG page lifecycle/fence/race/retention package, race enabled | 5.656s | PASS, no required skips |
| Seven changed Go subsystem packages, race enabled | individual package results retained | PASS |
| Web typecheck/lint; targeted Go vet | process results retained | PASS |

The real owner UI report is
`local-20261030T095204-20768/{broker.jsonl,web-real.json,web-component.json}`.
The final seven-case runtime report is `runtime-final.jsonl`.
PG evidence is `pg-ledger-final.jsonl` and units `unit-race-final.log`.
The final CI profile separately qualifies the committed Linux source, ordered
production migrations and actual owner UI with required no-skip checks.

## What passed

- Exact page/document/public-version/private-counter action binding; changed
  Task/constraints/mandate/lease/deny and foreign page metadata reject old grants.
- Real Chromium same-title pages target by opaque identity; fill/type hit the
  intended document; literal password input is denied. Exact close retains the
  closed identity and leaves the other page usable.
- Foreign popup first request and outside-gesture popup make zero recipient
  requests; delayed popup admission and four-live cap execute in real Chromium.
- Owner takeover fences every page. Switching to the second identical popup,
  private input, one owner POST and return run through the production Core owner
  API. Old frame/grant/ref use is denied; fresh PG/runtime counter translation
  permits the intended read after return. UI queues discard stale documents.
- One actual agent POST remains UNKNOWN under unsafe/none. A valid write on
  another page is stopped, recipient count stays unchanged, and exactly one
  context resource mapping remains. Runtime loss does not restore page input.
- Existing single-page owner UI regressions pass alongside the managed selector.

The local runtime fixture uses a private test-only egress policy with public
synthetic hostnames and an independent loopback recipient. It is actual
Chromium, not public HTTPS/Google acceptance. Approval setup in the Broker
fixture is synthetic test authority; it does not claim real owner consent.

## Failures retained and corrected

Early Linux RAM-only attempts exposed bind permissions, Node module ancestry,
the initial Request.frame race and pending-document refresh issues. After fixes,
the then-current six-case Linux runtime passed in 3351.981ms. Subsequent Windows
startup exposed the command-line length limit; the reviewed embedded helper now
loads through bounded private stdin. The final seven-case Windows test passed.

Two early Web profile attempts failed: the Windows npm shim invocation, then an
obsolete shared `.mind-advanced` selector despite a correctly rendered browser
panel. The latter backend truthfully timed out awaiting UI completion. The
corrected run passed actual UI and backend; abort signaling now cleans up failed
UI attempts. Prompt regression caught outdated required wording/section budget;
the complete compact browser strategy now passes assembly checks.

Initial committed-source Linux CI passed all three page jobs (ordered PG
migrations/ledger, recipient fixture, actual page execution/owner UI), Web and
discovery. The neighbouring vault profile caught a legacy fixture's positional
INSERT after adding page_mode: its URL landed in a boolean column. The fixture
now names columns explicitly and also proves managed contexts cannot create a
legacy vault request. The targeted real PG/race case passed in 0.44s. This was a
fixture correction, not a credential policy bypass; final-head CI is checked
separately in the PR/issue report.

## Remaining #164 gates

Managed-page UNKNOWN stays blocking; this stage does not silently apply legacy
frozen-snapshot owner review to another contract or replay input. Page-specific
verification/review and protected credential consent remain separate work.
Cross-origin RP/IdP transitions, public HTTPS ordinary egress, native model
deliberation/delivery and dedicated test Google acceptance are outstanding.
No issue closure or full SSO claim follows from these mechanical results.
