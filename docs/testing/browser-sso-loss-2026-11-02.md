# SSO loss and hostile-popup validation

This extends #164 / draft PR #185 after the successful native
[delivery slot](browser-native-sso-2026-11-02.md). Old native attempts and their
metrics are unchanged. These new cases are mechanical, with no model calls.

## Required HTTPS/Chromium cases

`scripts/dev/browser-sso-execution-tests.sh` now requires nine named cases with
zero skips: verified, delayed popup, wrong password, denied IdP POST, two
ambiguous popups, popup storm, stale RP document, actual helper loss before SSO,
and actual helper loss after an accepted IdP login but before its callback.

The last case uses the independent synthetic recipient's durable login receipt
to select the kill point. Its operator-only `--login-response-delay` defaults
to zero and is bounded to five seconds. It delays releasing the callback code,
not recording the accepted login. No public fault-control endpoint, runtime
test switch or real account is involved. Cancellation retains the consumed form
and login receipt, without an RP callback. Unit coverage rejects replay of that
form and verifies anonymous RP access remains denied.

Both loss cases must return no authentication proof, reject the old-context SSO
invocation and successfully open a distinct fresh read context. The fresh context
cannot restore authentication or disclose the private reference. Neither a stale
invocation nor the fresh read may change the recipient journal. Delayed popup
is a positive control; popup storm must produce zero login POSTs.

This runtime profile kills a real helper process; it is not a full Core/Broker
native recovery test. Existing PostgreSQL one-use, UNKNOWN and protected
historical-review guards remain separate required stages. No automatic outcome
reconciliation, grant refund or restored browser session is introduced.

Local fixture unit/race checks, runtime compilation and vet passed. Real Linux
Chromium/race results belong to the required CI run for this commit; no PASS is
inferred from compilation. Per-case logs retain wall time and `model_calls=0`.
Engineering/CI time is separate from native ingress-to-delivery latency.

## Google preflight and remaining implementation

At the owner's explicit request, a dedicated Google account was saved using
the protected Butler owner form as `Google SSO test account`, alias
`browser-google-sso-test`. No credential decision/use was granted by saving it.
The vault was locked afterwards. Password and login values are absent from
source, reports, native prompts and the synthetic recipient configuration.

The [Google OAuth policies](https://developers.google.com/identity/protocols/oauth2/policies#use-secure-browsers)
require a suitable registered OAuth client and restrict developer-controlled
embedded user agents. Users must be able to inspect the Google connection.
Butler's current bounded executor controls a helper, request routing and private
browser state. Based on that implementation and the policy, compatibility needs
an explicit supported full-browser authentication path. This is a preflight
inference, not an observed Google rejection or a passed Google login.

The current SSO contract supports one native form with both fields and one
exact POST. It does not implement a general multistep JavaScript identity flow.
The synthetic one-POST proof must not be reused as Google acceptance.

Required next work, still target design:

1. Define an external/full-browser auth handoff under the execution kernel:
   exact owner/Task/mandate, RP/IdP, OAuth client/redirect, expiry and one attempt.
   Preserve private state/PKCE, current page/control fencing and explicit owner
   consent. No User-Agent impersonation or reduced TLS/egress checks.
2. Compose that handoff with an existing supported desktop browser/owner control
   path. Credential release remains Broker-mediated to the current provider;
   saving a record, opening Chrome or a child result supplies no authority.
3. Bind the returned authentication result to the original RP attempt. Confirm
   RP access independently, retain unknown/lost outcomes without replay, then
   resume the same canonical Input and deliver its answer.
4. Run the dedicated Google smoke, retaining provider refusal, MFA or manual
   handoff as explicit outcomes and timing components. Do not create an account,
   accept new legal terms or enlarge Google scopes as a hidden test step.

Full hostile/origin/opener composition, independent model page selection and
verified durable Task completion also remain separate acceptance. #164 stays
open; this stage does not claim universal SSO support.
