# Credential vault

Migration106 adds distinct one-use login consent, with exact browser/form/POST
scope and account revision. `browser.login` resolves both fields under a durable
same-call claim; another proposal cannot spend that consent. Legacy requests
remain preparation-only. Owner UI explicitly displays submit plus verification;
PIN/owner authentication and ordinary Broker/task policy remain unchanged.

The current production path is `Vault`, an encrypted PostgreSQL store with
independent username/password fields, owner-scoped aliases and keyed exact login
search. Site hints are organizational metadata, never permanent allowlists.
Broker resolves only an approved request reference for its exact run, browser
session, origin, fields and current account/task revisions. `env://` resolution
has been removed. The old metadata-only Store remains solely as historical test
infrastructure; production startup never installs it as a credential provider.

`BUTLER_CREDENTIAL_KEY_FILE` points to an operator-owned random encryption key
file shared by Core and Broker. The six-digit owner PIN protects the control UI;
its server-keyed verifier, durable rate limits and five-minute unlock tokens
do not derive the encryption key from the PIN. Missing keys disable the vault.
Provider OAuth/control-plane credentials remain separate.

Owner secret input uses `/api/v4/credentials` with owner authentication and a
short-lived `X-Butler-Vault` capability; no secret body is an Input or transcript
message. Password edits preserve a field when its replacement is empty. Changes
revoke outstanding access requests. OTP values are encrypted with a two-minute
expiry and reserved to one exact tool call, allowing Broker revalidation without
authorizing a second dispatch. Expired OTPs are physically removed by maintenance;
backups can retain encrypted bytes under their ordinary retention policy.

For optional SMS login codes, enable the Companion's separate relay permission
and configure the account's paired device, default SMS package and exact sender
in the unlocked sidebar. Only one matching active login challenge is accepted.
Candidates are fresh for 90 seconds; receipt IDs carry no code or code hash.
Route revisions and current device status are checked again at release. Hidden,
late or ambiguous SMS requires the protected manual OTP form. HTTP/extension
API-token aliases from the old env resolver are no longer supported.

Run `scripts/dev/credential-vault-lab.ps1 -Action Setup`, then `-Action Start`
on Windows for a checksum-pinned, loopback-only PostgreSQL test runtime in `.cache`.
Set `BUTLER_TEST_VAULT_POSTGRES_URL` to the printed local test database endpoint
(user `butler_vault_lab`, database `credential_vault_test`, port 55439), then run
`scripts/dev/test-go.ps1 -Packages ./internal/credentials -Run TestVault -Count1`.
Fixtures create and drop their own schemas and contain synthetic secrets only.
Stop with `credential-vault-lab.ps1 -Action Stop`.

Linux CI runs `scripts/dev/credential-vault-tests.sh` with pgvector/PostgreSQL
and Chromium. Required migration, vault, policy, API, wait/resume and browser
tests must execute without skips. Parent consent pins task/constraint versions;
SMS source revocation also invalidates metadata at the final dispatch check.
