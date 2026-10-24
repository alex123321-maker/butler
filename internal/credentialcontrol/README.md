# Credential controls

Fixed Broker-owned `credentials.search`, `request_access`, `request_create`,
`request_otp`, and `request_status` contracts. Search returns safe entry metadata;
request operations create protected owner interactions, not authorization to
release a secret. The Broker derives owner/run/task identity from current Core
authority and checks an attested browser session. Secrets never appear in these
contracts. See [credential management](../../docs/architecture/credential-management.md).
