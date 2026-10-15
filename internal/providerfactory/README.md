# Provider construction

This package selects the existing native transport from trusted resolved operator
configuration. It never resolves credentials into orchestration/model context.

`AntigravityMaximumOutputTokens` is forwarded to the adapter's existing bounded
generation configuration. Zero retains the 8192 default. Values above 8192 up to
32768 require the selected Flash High model; other profiles fail construction.
The ceiling includes provider thinking and visible output. Model requests cannot
override it through transport options. An operator enabling the 32768 summary
profile must also set a finite execution policy with `output_tokens >= 32768` and
appropriate token/time allowances; provider construction does not grant budget.
