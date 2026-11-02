# Native-form SSO discovery qualification

Issue #164 / PR #185. **Native06 PASS at `4774f34c`**, including independent model
selection, one protected consent, independent RP proof and same-Input Web /
Telegram delivery. The run occurred November 3 in Tomsk, November 2 UTC.
`browser.sso_inspect` supplies the missing observation
step before the existing single protected native-form SSO decision. It is a
credential-free read with current all-of RP/IdP authority, one ephemeral child,
query-free candidate form paths/positional selectors, fresh PG RP refs and
freeze/cleanup. It does not authorize a callback, POST or generic IdP DOM read.

## Mechanical evidence

At source `4774f34c`, an isolated Linux container using the existing browser image
passed all 21 actual HTTPS/Chromium cases in 39.60 s. There were zero model calls;
this local portable binary did not use race instrumentation. Per-case timings
are retained in [the safe local report](evidence/browser-sso-discovery-2026-11-03/chromium-local.json).
The production containers were not changed by this mechanical run. The
disposable container used its own NSS/Node CA and synthetic recipient; TLS
verification and public-DNS/pinned-proxy policy remained enabled.

Coverage includes ordinary/delayed native login; wrong password; denied IdP
POST; a post-consent redirect to a denied RP path; ambiguous twins and popup
storm; stale RP; actual helper loss before/after accepted IdP login; inspection
of ordinary/fresh-path/delayed forms; inspection twins/storm/stale/denied form
and origin; a pre-consent automatic callback; reflected hidden-token paths; and
actual helper loss during discovery. Positive discovery cases subsequently
perform one native POST and require authenticated/anonymous RP probes.

The real PostgreSQL inspection profile passed all seven dispatch/release modes:
allowed fresh scope, stale page, owner epoch change, revocation before dispatch
and before release, newly discovered denied path, and foreign provider form.
Denied output is scrubbed. Unit coverage separately requires complete current
RP/IdP/verification authority, exact contract/ref/epoch and no credentials or
delegation. The browser prompt assembly regression passes with all rules inside
the existing section budget.

Required [SSO CI](https://github.com/alex123321-maker/butler/actions/runs/41491592049)
passed 289 tests/subtests across 17 stages with zero failures/skips, including
race instrumentation, ordered migrations,
permanent effect/UNKNOWN/one-use controls, same-Input continuation, real
Core/Broker/resource/PG/browser/owner UI and Web checks. The separate
[page/owner UI workflow](https://github.com/alex123321-maker/butler/actions/runs/41491592126)
passed all three jobs, including one actual owner Web case with zero
unexpected, flaky or skipped cases. The [safe CI summary](evidence/browser-sso-discovery-2026-11-03/ci-summary.json)
retains the SSO counts; these mechanical timings do not replace native latency.

## Retained failures and correction

The `8eb35aab` HTTPS profile caught an actual automatic provider callback before
consent, despite a failed inspection result. The route guard did not see every
HTTP redirect. Redirect handling now fetches one authorized response with
automatic redirects/retries disabled and checks each next destination before a
fresh browser navigation. Provider bodies are never rendered as RP content;
the native context's private cookies, verified TLS and checked proxy remain.
The same guard fences post-consent callback paths and denies POST replay.

The `d77d9729`/`da6bf45e`/`8e5e2590` candidate profiles retained positive-control
failures while this path was being built. Local diagnosis isolated a lost
initial-popup reservation across the redirect bridge. The corrected guard
borrows no new window allowance: it matches the original Request's actual Page
before using its already-spent reservation. All21 cases then passed, including
the negative callback and popup controls. Fixed private error categories aid
diagnosis; raw Playwright errors, URLs, bodies and credential material stay out
of logs and model results. No failed native POST was replayed for these checks.

## Native acceptance

Native06 used a new public route variant, field names, button ID
and button order. Its submitted goal contains only the task, RP/IdP origins and
saved-record title; no page ID, selector, form path, account ID or private fact
was supplied. The model obtained current observations, chose the actual login
button, discovered/selected a candidate scope, obtained one protected decision
and delivered the independently verified private RP reference through the
original Input. PG evidence shows the chosen button and protected-account link
were present in the actual RP observation and the consent scope exactly equals
a returned discovery candidate. The original submitted goal matches its
operator file and contains no fresh variant, credential canary or selectors.
Both ephemeral children remain closed in the ledger; the model subsequently
closed the context. No operator selected a page/form or supplied a scope.

The independent recipient retained exactly one accepted IdP login, one
accepted callback and two authenticated account reads (browser plus Go probe).
The separate anonymous Go control was denied; `browser_sso_login_v1` effect
evidence is confirmed. There was one request/use, one counted test-operator
decision and zero generic POST approvals. The operator used the owner UI under
the user's prior explicit authorization. Vault UI required PIN again after
completion. Input/Run completed; durable Task completion is not inferred.

| Inclusive native06 phase | Seconds |
| --- | ---: |
| Ingress → tools discovered/loaded | 102.650689 |
| Loaded tools → saved record found | 25.529616 |
| Record found → RP opened | 17.435277 |
| RP opened → form discovered | 19.097036 |
| Discovery → protected request | 20.922256 |
| Request → counted operator consent | 54.860728 |
| Consent → verified RP result | 35.082650 |
| Verified result → Web ready | 22.205703 |
| Web ready → Telegram accepted | 5.128836 |
| **Ingress → Telegram accepted** | **302.912791** |

Ingress 17:34:00.887924 UTC; Web ready17:38:58.671879; Telegram
accepted 17:39:03.800715; Input finalized 17:39:38.625507 (337.737583 s).
All model deliberation, scheduling, tool loading and operator waits remain
inside the phase intervals. They are not pure network/model timings. The
recorded SSO tool span alone was 2.498692 s and discovery 3.061412 s; neither is
the agent's total login speed. Its 15 observed model attempts used 192738 input /
2434 output tokens. Slow discovery remains visible: six catalog searches plus
one load took the first 102.65 s with model/scheduling overhead included.

The owner Web rendered one result card with the exact verification URL and
privately matched reference, checked 17:42:45.409 UTC. This later UI check is not
first-paint latency; server Web ready and actual Telegram acknowledgement are
the native endpoints above. Credential canaries were absent from scanned
tool arguments/results, messages and publication; provider wire/all system
logs were not audited by that scan. No raw private reference is exported in
the safe proof. Evidence:

- [Recipient/effect/delivery verification](evidence/browser-sso-discovery-2026-11-03/native-sso-06-verification.json)
- [Independent discovery and authority linkage](evidence/browser-sso-discovery-2026-11-03/native-sso-06-discovery.json)
- [All tool/model/state spans](evidence/browser-sso-discovery-2026-11-03/native-sso-06-final-timings.json)
- [Exact attempt metadata](evidence/browser-sso-discovery-2026-11-03/native-sso-06-final-metadata.json)
- [Actual owner UI check](evidence/browser-sso-discovery-2026-11-03/native-sso-06-ui-proof.json)

External Google04 is separately qualified in
[its bounded diagnostic report](browser-native-google-2026-11-02.md). It used
normal Chrome and the already signed-in dedicated account, not native-form
password/MFA automation or session import. The synthetic result does not replace
that external evidence, and neither result claims universal SSO compatibility.
