# Antigravity native transport

This Go adapter uses production AICode v1 SSE with the subscription experience
`gemini-3.8-flash-tiered`. The flat request uses `aicode.experience`, ordered
contents, optional system instructions/Butler function declarations and an
8192-token output allowance. It omits the mutually exclusive `model`, Cloud Code
project/request wrappers and v1main-only session/entitlement fields. Flat SSE
responses provide candidates and usage metadata.

Only `providerauth.AntigravityTokenSource` supplies token and expiry. Transport
does not read credentials or run the CLI. Native calls must match supplied aliases
and schemas; Tool Broker remains the sole executor. Exact function parts and opaque
thought signatures remain private and run-local. Durable recovery starts fresh
with Butler observations as data, while live continuation correlates the original
native call and function response. Cancellation closes HTTP and discards context.

HTTP 429 with the bounded, recognized `HIGH_DEMAND` reason uses the fixed
`model_capacity_unavailable` subtype and reports temporary model
capacity loss. Other 429 responses remain generic; neither proves exhausted
subscription quota. Raw Google diagnostics stay private and no implicit retry runs.

See [the operator guide](../../../deploy/antigravity/README.md) for private import,
the unsupported third-party integration policy, proxy configuration and real
validation status. Fixtures verify wire shape, safe failure handling, signatures,
schema guards and cancellation; they do not prove Google model access.

`InferenceLimits` declares 1,048,576 input plus 8,192 output tokens for conservative
kernel admission. Private `transport.WithInputByteLimit` bounds the exact complete
serialized request, including private retained history, schemas and tool results,
before HTTP write; unset callers use a fixed 8MiB cap. Operator subscription cost
basis is separately required by production execution budgets. Neither this quote
nor model availability proves a successful authenticated request.
