# Provider authentication control plane

Provider credentials are resolved through typed sources rather than model-visible
arguments. Antigravity import accepts only a private operator stream and requires
the encrypted PostgreSQL SecretStore. The Manager serializes refresh, validates
the OAuth application, rejects redirects and masks bounded upstream errors.
Rotated credentials are saved before returning token/expiry to native transport.

AICode requires no subscription project. An optional legacy `project_id` can still
be imported and remains encrypted, but resolution never performs Cloud Code
discovery or exports that value to inference. The settings surface exposes only
safe connection/expiry state and disconnect, not credential material.

The private operator command `provider-auth -action audit-origin` compares a
bounded private reference import on stdin with the current encrypted PG record.
Only fixed stored/application/credential/refresh-match booleans are emitted.
It performs no refresh, write, OAuth or inference request, even for expired
access tokens. Tokens, hashes, account/project IDs and raw store errors stay
private. A refresh mismatch can reflect token rotation and does not prove a
different account. There is no model, channel or settings API route for this
operator capability.

See [the operator guide](../../deploy/antigravity/README.md) for import and smoke.
