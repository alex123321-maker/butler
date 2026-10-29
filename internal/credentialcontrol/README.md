# Credential controls

Fixed Broker-owned `credentials.search`, `request_access`, `request_create`,
`request_otp`, and `request_status` contracts. Search returns safe entry metadata;
request operations create protected owner interactions, not authorization to
release a secret. The Broker derives owner/run/task identity from current Core
authority and checks an attested browser session. Secrets never appear in these
contracts. See [credential management](../../docs/architecture/credential-management.md).

`StoppedAccessReceiptTx` is a kernel-only metadata reader for the pinned historical
revision1 access-request protocol. Under the caller's terminal Input/run fences,
an expired, unapproved, unnotified request, its single creation audit and expired
owning browser context can prove creation only. Exact original argument hashing
is checked by the Action journal. It reads no ciphertext/PIN/unlock material and
cannot authorize use, deliver a notice, revive a run or prove login.
Pruned contexts require a separate confirmed original open and matching saved
result digest/expiry. `StoppedAccessAbsentTx` instead proves only absence of the
exact call's atomic request while the terminal creator fences remain held.
