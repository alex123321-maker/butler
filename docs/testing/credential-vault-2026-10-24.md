# Credential vault: local implementation and validation, 2026-10-24

## Implemented behavior

Work was performed on the Windows PC in `D:\Repository\butler`. No production
database, remote host, real account, provider inference or Telegram recipient
was used. Existing user prototypes were not edited.

- Separate encrypted login/password fields, owner aliases and exact keyed login
  search. The same login may appear in multiple independent records. Optional
  site hints do not restrict sites; each use requires consent for the actual
  browser origin, purpose and fields.
- Global right-center bookmark, centered six-cell PIN setup/repeat/unlock,
  sidebar creation/edit/revocation/deletion, PIN change and automatic lock.
  Empty replacement fields preserve stored values. Lists return metadata only.
- Distinct access-existing and create-or-select cards. Creation alone does not
  grant access. Protected owner responses resume the same durable Input.
- Five canonical Broker controls: `credentials.search`, `request_access`,
  `request_create`, `request_otp`, `request_status`. Requests attest the current
  server-browser URL. The model receives request references, never secret values.
- Owner consent authorizes current local browser preparation only; it grants no
  form-submission permission and does not bypass unknown-effect handling.
- Chromium privately compares the actual input field with the resolved value.
  `credential_verified: true` contains no value, hash or length. It proves field
  equality at that moment, not remote login success.
- Protected manual OTP, encrypted two-minute material, reservation to an exact
  tool call, matching-answer idempotency and maintenance expiry.
- Telegram fixed protected cards and closure edits, using an HTTPS link to the
  owner-authenticated Web form. The fragment is a request locator, not a login
  capability. No secret input passes through Telegram messages or callbacks.
- Optional Android SMS login relay: separate phone opt-in, paired-device/app/
  exact-sender account route, fresh unambiguous challenge under approved password
  access, private in-memory extraction/retries and encrypted server delivery.
  Queue copies remain masked. Device/route revocation is rechecked at release.
  Payment/reset/ambiguous/hidden SMS falls back to protected manual input.
- No import or fallback for old `env://` records. Historical HTTP/extension
  token aliases fail closed; this account-vault release supports browser fields.
  Provider control-plane authentication remains separate.

## Executed evidence

| Area | Evidence | Scope |
| --- | --- | --- |
| Encryption/storage/PIN | `TestVaultLocalPostgresIntegration`, cipher/index tests | Actual PostgreSQL17; duplicate logins, separate values, ciphertext, owner isolation, PIN backoff/unlock, destination/run mismatch, rotation, OTP reservation/idempotency, SMS opt-in/sender/ambiguity/route revocation |
| Protected owner API | `TestCredentialVaultOwnerAPIPrivacyLocal` | Actual PostgreSQL; owner/unlock proof, no-store, strict JSON, fixed errors, no secret in metadata/error responses, PIN-change revocation |
| Model controls | `TestCredentialControlBrokerLocalPostgres` | Actual Broker pipeline + PostgreSQL; all five canonical controls, stale destination denial, pending does not authorize, safe status, direct executor denial. Owning action context is a fixture |
| Kernel | `TestCredentialVaultLocalWaitResumeAndExpiry` | Actual PostgreSQL; owning Input suspend/resume, early/late response, expiry, expired lease denial, closure after interaction replacement, no duplicate notice. This is not an OS process-kill test |
| Model-loop wait | `TestCredentialRequestsPreservePrivateWaitAcrossLostAcknowledgement` | All three request tools, normal/lost ACK and missing/read-failed checkpoint; one dispatch, no generic transcript result or old provider continuation |
| Action policy | `TestCredentialRequestDoesNotRequireApprovalToAsk`, `TestCredentialOwnerGrantIsLocalAndPinsInputClaim`, `TestCredentialRequestLocatorKeepsStandingMandateBoundaries` | Exact contract, no duplicate approval-to-ask, Input replacement/revocation denial, owner-goal future request references with unchanged site/tool/effect/deny scope; local preparation cannot authorize submit |
| Browser | `TestBrokerBrowserSequenceE2E` with local vault DSN | Actual encrypted PostgreSQL store, Broker resolver, gRPC runtime and Playwright Chromium; pending denial, owner consent under a bounded goal mandate without predeclared future request IDs, field equality, reflection masking, revocation and later ordinary form sequence. Session/action ownership is a fixture |
| Browser runtime | `TestBrowserSessionE2E` | Actual Chromium; credential comparison plus existing frozen preparation, guards and private observation checks |
| Web | `web/tests/e2e/credential-vault.spec.ts` | Chromium against built Nuxt; first/repeated six-cell PIN, independent fields, lock/storage checks, distinct cards, per-request OTP and explicit SMS route. API is mocked; screenshots/traces/video disabled |
| Telegram | `TestCredentialNoticeProtectedLinkAndClosedState` | Fake Bot API transport; verified bot/binding, protected HTTPS link, no secret callback, closed-card link removal. No actual Telegram send |
| Android | `scripts/dev/mobile-build.ps1 -Test` | Debug APK, unit tests and Android Lint. 18 unit cases, 0 failures/0 skips; includes login-code exclusion/freshness and wire serialization/redacted formatting |

Targeted Go packages passed, including credentials, actionauth, browsercontrol,
workspace security, mobile, Broker packages, owner API, orchestrator, durable
Input and Telegram adapters. The feature's local PostgreSQL tests were enabled
explicitly. Existing opt-in suites for other services can skip their unavailable
fixtures; those skips are not claimed as acceptance. `-race` passed for changed
business/policy/API/kernel packages; targeted `go vet` passed. Web typecheck,
ESLint and Stylelint passed. The final built-Web regression run passed all21
cases across credential vault, Mind and communication widgets (0 skips).

## Reproduce on this PC

All data below is synthetic. The disposable database name must end in `_test`.
Portable PostgreSQL is checksum-pinned under the repository `.cache` and binds
only `127.0.0.1:55439`. The test fixtures create/drop their own schemas.

```powershell
pwsh -NoProfile -File scripts/dev/credential-vault-lab.ps1 -Action Setup
pwsh -NoProfile -File scripts/dev/credential-vault-lab.ps1 -Action Start
$env:BUTLER_TEST_VAULT_POSTGRES_URL = 'postgres://butler_vault_lab@127.0.0.1:55439/credential_vault_test?sslmode=disable'
. ./scripts/dev/common.ps1
Initialize-ButlerAgentEnvironment | Out-Null
go test -count=1 ./internal/credentials ./internal/actionauth ./apps/tool-broker/... ./apps/orchestrator/internal/api
go test -count=1 -run '^TestCredential' ./apps/orchestrator/internal/app

New-Item -ItemType Directory -Force .cache/vault-browser | Out-Null
go test -c -o .cache/vault-browser/runtime.test.exe ./apps/tool-browser/internal/runtime
$env:BUTLER_BROWSER_E2E = '1'
$env:BUTLER_BROWSER_E2E_BINARY = Join-Path $PWD '.cache/vault-browser/runtime.test.exe'
$env:BUTLER_BROWSER_E2E_SCRIPT = Join-Path $PWD 'apps/tool-browser/scripts/browser_runtime.mjs'
go test -v -count=1 -run '^TestBrokerBrowserSequenceE2E$' ./apps/tool-broker/internal/broker
go test -count=1 -run '^TestBrowserSessionE2E$' ./apps/tool-browser/internal/runtime

Push-Location web
npm run typecheck
npm run lint
npm run test:e2e -- credential-vault.spec.ts --workers=1
Pop-Location

$env:ANDROID_HOME = 'D:\ProgramFiles\AndroidSDK'
pwsh -NoProfile -File scripts/dev/mobile-build.ps1 -Test
pwsh -NoProfile -File scripts/dev/credential-vault-lab.ps1 -Action Stop
```

Browser dependencies must already be installed through each existing npm lockfile
and Playwright installation, with caches initialized by `common.ps1`.

## Activation and remaining acceptance

This work changes local sources; it does not enable the feature in production.
Apply migration096 through the normal migrator. Mount the same private random
key file (32+ random characters, outside Git) into Core and Broker, setting
`BUTLER_CREDENTIAL_KEY_FILE`. Use the existing server-browser composition and
private action authentication, plus `deploy/docker-compose.credentials.yml`.
Set `BUTLER_CREDENTIAL_KEY_SOURCE` to that private file and
`BUTLER_PUBLIC_WEB_URL` to the actual owner-facing HTTPS Web address for Telegram.
The owner sets the PIN in the UI; no PIN or production encryption key was chosen
by these tests. Missing key configuration leaves the vault unavailable.

For SMS, install the built `mobile/android/app/build/outputs/apk/debug/app-debug.apk`
using the deployment's normal signing/update rules. Enable notification access,
text transmission, the SMS relay switch and the account route. The SMS app must
be the phone's default SMS package and included by its app filters.

Final debug APK SHA256:
`3e63c7c1fc3bd45e2680e7780c091ab693059de2e5f09d88e3c2a797d13d90e0`.

**Not yet verified:** physical-phone SMS delivery (ADB lists no connected device),
real owner Telegram delivery, Telegram Mini App authentication, real external-site
login/MFA, and an ordinary full Core process restart/provider continuation.
Telegram input currently opens the Web owner-authenticated form.

Docker's local daemon was unavailable. PostgreSQL tests used real portable PG
and migration096 over isolated subsystem schemas. The complete ordered migration
chain with pgvector and the Linux-only native-release fixtures were not accepted
on this host. An earlier broad app-package run hit existing native/Linux fixture
prerequisites; it is not reported as a passing full project suite. The updated
legacy native browser/extension fixture sources compile, but their dedicated
full-schema native run remains separate.

Android may hide OTP content. The relay never bypasses that protection and does
not read old SMS from the message database. Arbitrary website login success and
external side effects still require the existing separate evidence/authority.


## Review before merging the local implementation

The review reproduced and fixed three defects: an OTP could inherit password
consent across changed task constraints; SMS route revocation left metadata
usable by a final dispatch check; cross-origin Web requests could not send the
vault unlock header. Regression cases now cover all three and migration 096
rollback/reapply. The new Credential vault acceptance workflow requires the
complete ordered migration chain on pgvector/PostgreSQL, vault/policy/API/kernel
cases, and Broker/runtime Chromium E2E without skipped required tests.

Fresh local review checks passed PostgreSQL vault/policy/Broker/API and kernel
wait/resume tests, Orchestrator/Input/Telegram/mobile/workspace/browser-control
regressions and Go vet. Web typecheck/lint and all three credential-vault browser
scenarios passed. Physical SMS and native Telegram acceptance remain the separate
limitations described above. Prototype HTML files are excluded from this merge.


The review also ran both actual Chromium suites successfully:
`TestBrokerBrowserSequenceE2E` and `TestBrowserSessionE2E`. Android debug APK,
all 18 unit tests and lint passed on this PC. All `apps/...`, `internal/...` and
`cmd/...` Go packages compiled. The first new CI run passed the full ordered
PostgreSQL migration test and vault cases, then rejected Go's package-level
skip for the contract-only package (which has no tests). That package remains
covered by vet; the required test list now contains test-bearing packages only.
The no-skipped-tests guard is unchanged.
