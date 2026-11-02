# SSO browser cookie policy correction

The final #164 audit found that `route.fetch` uses the browser-associated
APIRequestContext. When Chromium intentionally omits Cookie, that API client
can fill the missing header from an origin-only jar, ignoring the browser's
SameSite decision. Its response addCookies path also must not replace native
Set-Cookie processing. This was discovered after PR185 merged; issue164 remained
open while the correction was implemented.

Each SSO HTTP request now uses a fresh, initially empty, isolated one-response
API context through the same checked proxy with certificate verification.
It copies the actual browser headers, including absence of Cookie. No API jar
is reused or imported. Native Chromium processes response Set-Cookie, including
on the bounded redirect bridge. Private response values join secret redaction;
the response/context are disposed after use. Approval, one-POST, path/epoch and
independent RP proof contracts are unchanged.

The regression was reproduced in actual Linux HTTPS/Chromium with genuinely
different registrable sites (`rp.sso.test` and `idp.provider.test`). With all
RP cookies Strict, the original code leaked the withheld cookie to callback;
both direct and post-inspection negative controls failed at the independent
recipient. With the fix, the cookie is absent, the RP denies its missing binding,
there is one accepted IdP POST but no accepted callback/account read/proof.
Positive cross-site Lax-binding controls still require successful RP proof.

All 25 HTTPS/Chromium cases passed locally in 60.13s after isolating response
handling, including all prior redirect, popup, discovery and loss controls.
This portable local run had no race instrumentation and zero model calls;
required CI runs the same 25 cases with race instrumentation and rejects skips.
The [safe local report](evidence/browser-sso-cookie-policy-2026-11-03/local.json)
retains per-case spans. The main #164 SSO profile grows from 289 to 293 named
tests/subtests solely through four meaningful cookie controls.

No native model attempt was replayed for this correction or to reconstruct
metrics. [Native06](browser-sso-discovery-2026-11-03.md) and
[Google04](browser-native-google-2026-11-02.md) retain their original source and
timing boundaries. This report is cookie/transport evidence, not another native
login, general MFA automation or Chrome-session import qualification. Final
PR/main CI results are linked from issue164.
