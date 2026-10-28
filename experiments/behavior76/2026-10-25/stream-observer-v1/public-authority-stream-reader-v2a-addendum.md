# Public reader implementation correction v2a

This supersedes only the implementation pins in
`public-authority-stream-reader-receipt-v2.json`. Its original files/logs/receipt
remain unchanged. The public stream-v2 representation contract, original
criteria, thresholds, old scoring code, bounds, error/cost accounting and
admission requirements are unchanged. No hidden controls have been authored and
no native call was made for this correction.

Final source review found that delimiter-substring membership could accept a
compound string such as `deadline|cancelled` where exactly one fixed category
was required. No Go writer emits such a value, but accepting it violates the
declared schema. New `authority-stream-reader-v2a.cs` replaces only those four
enum membership expressions with exact-token membership. Four new negative
selftests cover compound outcome, context cause, family and SQLSTATE. The
original 41 reader tests and all four original composition tests are retained.

Use `authority-stream-reader-v2a.ps1` and
`authority-stream-scoring-v2a.ps1`, with the same public function entry names.
The wrapper pins the corrected reader source, then imports the exact unchanged
old composer, adapter and criterion kernel. It creates no substitute snapshots,
events or v1 evidence. Case/property aliases and duplicate properties are
rejected in C# before the header is parsed by PowerShell.

Result: 45 reader and 4 scoring-composition public selftests PASS, no provider,
PG, credential, mapping or hidden-case access. This is a representation-checker
receipt, not authorization for native collection or issue closure. The closed
local-v4-r2 GLOBAL/incomplete result and all original costs remain retained.
