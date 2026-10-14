# Native metadata preflight

One-shot Go helper uses the existing private `providerauth.Manager`, an ephemeral
process-only SecretStore and the mounted operator import reference. It does not
open any native/production database or construct a model provider. There is no
generation, tool execution, admission, model-selected URL or command flag.

It requires the standard laba `antigravity-proxy:18080` route, resolves paid Cloud
Code discovery and reads the fixed daily `fetchAvailableModels` endpoint. Only
booleans, numeric HTTP status and `model_calls:0` are emitted; tokens, project,
account identity, catalog IDs/values and raw responses are never exported.
45-second total,30-second catalog request and2MiB response caps apply. Failure
denies native entry; metadata availability does not prove generation capability.
The frozen High profile must be present. No new library or durable state is added.
