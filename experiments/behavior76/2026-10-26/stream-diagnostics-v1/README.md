# Local rejection diagnostics after closed day-v6

The closed day-v6 remains incomplete/NO_GO. Its last protocol error cannot be
attributed to a precise parser branch or to upstream versus the adapter. The
historical640 admissions,633 measured totals,4,900,143 known full tokens and7
NULL usage records remain unchanged. No native request or new window ran here.

The patch adds fixed reason codes to exact transport-owned rejection literals.
It changes no parser, response acceptance, limits, usage retention, retry,
authority or admission. Reasons identify local rejection, not upstream blame.
The combined EOF check still groups scanner/reader failure, total-byte limit,
incomplete frame and missing terminal as `stream_terminal_missing`.

Validation on this PC with Go1.27.2/Linux:

- `TestStream*`:17 malformed SSE cases plus the one-request failed-event test,
  race PASS. No tool/final/usage is released for the malformed stream.
- Both `internal/transport` and `internal/transport/antigravity`: race PASS,
  followed by vet exit0 (`stream-diagnostics-suite-v2.log`).
- The first full package run failed the existing catalog test because it checked
  canonical Broker bounds directly against the strict provider schema dialect.
  The test now exercises the actual `nativeToolSchema` conversion before its
  strict dialect check. Production schema conversion is unchanged. The failed
  log is retained, not replaced.
- An independent checker ran10 additional synthetic boundary cases with race:
  PASS, no concrete blocker. Its receipt pins exact reviewed files and records
  the combined-terminal limitation. Source-copy hashes matched the patch.

The independent test source is retained as text; it can be copied into an
isolated copy of the same transport package and run with
`go test -race -run '^TestIndependentStreamDiagnosticBoundaries$'`.
`artifact-pins.json` pins all included logs/review/test bytes. None are native
quality evidence or permission to reuse day-v6 stimuli/goldens.
