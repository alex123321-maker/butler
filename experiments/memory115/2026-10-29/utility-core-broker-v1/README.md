# Actual Core/Broker utility trace verification — 2026-10-29

**PASS for the measuring tool; native quality remains unqualified.** Seven
controlled integration cases passed on laba, no failures/skips; 39 corrupted
trace controls were rejected. The three deliberate wrong answers remain failures
of their tasks, and the generic rejection remains incomplete UNKNOWN. They are
passing tests of the evaluator, not seven successfully solved tasks.

| Case | Actual Broker calls | Evaluated task success | Evidence |
|---|---:|---|---|
| Stored historical fact | 2 | yes | actual search, pinned full open, exact citation/value |
| Same read, wrong answer | 2 | no | valid JSON, rejected `answer_mismatch` |
| Inspect current project | 1 | yes | current failed check accurately reported |
| Same inspect, wrong answer | 1 | no | valid JSON, rejected `answer_mismatch` |
| Prepare and check | 3 | yes | current artifact ID, passing check, original unchanged |
| Same preparation, wrong answer | 3 | no | valid JSON, rejected `answer_mismatch` |
| Generic mutation rejection | 2 | no | actual `effect_unknown`, blocked input, `run_incomplete` |

## What was connected

The actual Input/Core/session/task path executed compiled Broker and extension/HTTP
runtimes against isolated PostgreSQL and a synthetic recipient. The source was
stored before constructing a new Catalog reader. This proves durable storage and
reader reconstruction for that fixture; it is not a process-wide restart or
semantic retrieval benchmark. The controlled provider chose tools and generated
typed finals from actual tool responses; it made no native model requests.

`utilitytrace.Capture` collected owning task/run/input, calls, intents, effects,
dispatches, normalized result evidence, source pins/heads, final and model-attempt
rows. `Project` checked those against the binding frozen before execution and
recipient file state before calling the utility scorer. Actual Catalog citation
identities were mapped to the corresponding synthetic source identities; raw
finals were retained without repair. Dynamic artifact IDs use the prospective v2
current-receipt marker. Old v1 fixed-ID suites retain their semantics.

The 39 corruption controls cover omitted calls/intents/effects/dispatches/evidence/
finals/inputs, foreign run IDs, replaced results/arguments, changed original files,
missing source pins/heads and ambiguous source mappings. The collector's seal is
trusted input, not an attacker-resistant signature. Model-provided booleans are
never an acceptable replacement for the collector.

## Execution and preserved failures

Final source commit: `2739e992` (code); subsequent documentation-only commit
`14aa594b` describes the same tested behavior. All 6,731 tracked source blobs were
compared against Git; only the declared CRLF-to-LF normalization of
`scripts/dev/verify-changed.ps1` was needed. No source mismatches remained.

Root: `/srv/gitea/butler-acceptance/issue115-utility-trace-20261029-r3`.
`reports/cases/*` retain private suites, frozen bindings, raw snapshots,
observations, results and their seals. Private raw files/oracles are not committed.
Public [verification summary](verification-summary.json) records their hashes,
counts, actual tool names, effect states and scoring outcomes. [Build log](build.log)
records Go 1.27.2, race-tested utility tests and executable hashes. Vet passed for
the adapter and app; the actual app integration binary was built/run with `-race`.
The seven cases took 17.17 seconds (package 18.242 seconds).

Earlier fresh roots remain intact:

- `...-r1`: build/race/vet passed, then Docker default address pool exhausted;
  no integration cases ran. A checked unused internal subnet was used subsequently.
- `...-r2`: six cases passed; the seventh caught a test expectation error. Core
  correctly returned `blocked/effect_unknown` without a Go error. The test was
  corrected to inspect the actual receipt and persisted UNKNOWN. The observer
  now retains explicitly blocked UNKNOWN as incomplete rather than discarding
  the trace because the run has no completion timestamp. No production behavior
  or earlier result was changed.

Final PostgreSQL exited cleanly and is stopped with its owned data retained;
the private internal network was removed. No host ports were published. Source
was mounted read-only, build networking disabled, and the execution network had
no external egress. No running Butler deployment was updated.

## Cost and limits

**Zero new native/Gemini requests in all three attempts.** Final traces retain 20
scripted model-attempt rows and 14 actual tool calls. Scripted usage is synthetic
fixture data, excluded from the historical native spending ledger. All old native
results, failed runs, unknown costs and full-stage accounting remain unchanged.

This verifies the observer/scorer plumbing for memory.search/open and the reviewed
project recipient. It does not demonstrate a quality improvement from memory,
cover all 24 corpus cases, or complete A/B/C. Broker skills.load evidence mapping,
the typed verified-negative fixture, lifecycle controls, native ledger correlation
and frozen full native comparison remain separate work. Unsupported tools fail
closed. Issue #115 remains unqualified for closure by this result alone.
