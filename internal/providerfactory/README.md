# Provider construction

This package selects the existing native transport from trusted resolved operator
configuration. It never resolves credentials into orchestration/model context.

The distinct experimental `antigravity-acp` route requires an environment-only
opt-in and a private Unix socket to the isolated Linux runtime. It does not use the native
auth manager or fall back to another provider. It is omitted from managed UI
provider choices. The kernel requires the owner-approved distinct opaque
subscription accounting policy; no native token limit is invented. See the
[ACP adapter](../transport/antigravityacp/README.md).

`AntigravityMaximumOutputTokens` is forwarded to the adapter's existing bounded
generation configuration. Zero retains the 8192 default. Values above 8192 up to
32768 require the selected Flash High model; other profiles fail construction.
The ceiling includes provider thinking and visible output. Model requests cannot
override it through transport options. An operator enabling the 32768 summary
profile must also set a finite execution policy with `output_tokens >= 32768` and
appropriate token/time allowances; provider construction does not grant budget.
