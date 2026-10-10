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

See [the operator guide](../../deploy/antigravity/README.md) for import and smoke.
