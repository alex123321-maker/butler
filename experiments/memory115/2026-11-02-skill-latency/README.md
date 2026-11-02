# Model-free investigation of the additional 137 seconds

Verified on laba on 2026-11-02. **Zero new native model requests.** All historical
outputs, scores, usage and costs remain unchanged. Production services and code
were not modified. Diagnostic sources are private overlays of native source
`66be595bc122ea5a6e02def4689b92b70945fc3a`.

## Historical attribution: Terra C minus B

The old totals reproduce exactly: 158.429s more execution, of which 21.114s is
additional provider wait and **137.315s is outside the provider boundary**.
There are 11 additional provider methods: eight follow `skills.load` and three
follow `skills.search`; the counts of project and HTTP tools are unchanged.

| Location of the additional outside-provider time | Seconds |
|---|---:|
| Last provider response to durable run finish | 115.943 |
| Between provider methods | 20.622 |
| Before first method | 0.242 |
| Local work inside method before pre-serialization marker | 0.725 |
| Difference in execution/run envelope overhead | -0.217 |
| Total | 137.315 |

These wall-clock phase differences are approximate (old `elapsed_ms` rounding
and PostgreSQL/Go timestamps); the overall 137.315s uses original monotonic
timers. The 2.376s additional tool RPC is a **subset** of these intervals, not an
extra term. Historical raw-write lag increases by only 1.041s; file mtime marks
write time and cannot measure completion of `fsync`. The first C case also had
14.119s between methods versus roughly 1.6–1.7s in later migration C cases;
the original spans do not identify every component of that outlier.

[terra-historical.json](terra-historical.json) contains all 36 cases and full
arithmetic. Luna shows a similar additional final tail (115.735s), whereas
Gemini's is 5.992s. Supplemental historical analyses of unfinished runs use
`updated_at` when no `finished_at` exists; Terra has actual finish timestamps.

## Cause confirmed by actual Core/Broker control

The frozen [skill guard](../../../apps/orchestrator/internal/orchestrator/skill_provider.go)
calls `catalog.Pinned` **before every transport event**, including every text
delta, and also on a 250ms ticker. This is a private HTTP RPC. The
[catalog implementation](../../../internal/skills/run.go) calls `Load` for each
pinned skill: authority, contract/dependency, source and lifecycle checks,
database locks and a transaction are repeated. This is considerably more than a
local boolean check.

Same task, tool effects, checked migration and final answer; only stream
fragmentation changes. All six cases passed through real Core, Broker, runtimes
and isolated PostgreSQL, using scripted transport and no provider connection:

| Arm | Text deltas before final | Scripted provider methods | Pinned RPCs | Pinned RPC time, s | Execution, s |
|---|---:|---:|---:|---:|---:|
| B | 0 | 4 | 0 | 0 | 0.908 |
| C | 0 | 5 | 9 | 0.338 | 1.386 |
| B | 180 | 4 | 0 | 0 | 1.056 |
| C | 180 | 5 | 223 | 9.210 | 10.251 |
| C | 360 | 5 | 441 | 18.521 | 19.593 |
| C repeat | 180 | 5 | 225 | 9.414 | 10.405 |

Thus fragmentation adds about 8.87–9.02s at identical provider-method count in
the C controls; nearly all is measured in Pinned RPCs. Doubling fragmentation
roughly doubles cost. Ticker checks amplify the per-event calls while draining
the stream. The original 137s cannot be assigned RPC-by-RPC because those spans
were not recorded historically. The observed final-tail location, code path and
controlled reproduction establish the dominant mechanism, rather than claiming
an exact reconstruction of every old internal operation.

## Research buffering changes the timing interpretation

The native [capture wrapper](../../../apps/orchestrator/internal/app/memory_utility_native_test.go)
drains a whole provider turn, writes its journal, then returns a buffered channel
of saved events. The outer skill guard therefore checks those events **after**
the measured provider wait. Many OpenAI delta events incur a long sequential
tail; Gemini emits many fewer events. Normal live streaming may overlap these
checks with remote generation. This experiment confirms repeated RPC overhead,
but does not establish a 137s latency penalty for an ordinary production build.
Race instrumentation, host load and the changed measurement date also preclude
equating new replay wall times with old per-RPC times.

## Journal-only control

Replayed all 128 original Terra payloads (4,461,216 serialized bytes), including
all arms, into a **new** journal table and private files, under the same pinned
Go race image/1 CPU composition. Each entry performs INSERT, dispatch UPDATE,
JSON serialization, body UPDATE, file write, `Sync`, close. No model or tool is
invoked by this test.

| Operation | Seconds |
|---|---:|
| PostgreSQL operations | 0.4154 |
| JSON serialization | 0.2615 |
| File write + fsync + close | 0.1578 |
| Total measured operations for all 128 methods | 0.8347 |

The earlier failed diagnostic's independent journal replay measured 0.8406s.
These are current warm-environment measurements, not exact historical disk
latencies. Together with the old write timestamps they exclude durable trace
writing as the dominant explanation for the 137s.

## Evidence and safe reproduction

- [summary.json](summary.json): six controls, actual provider-method counts, RPC
  durations, successful task scores and journal-only measurements.
- [validation.json](validation.json), [exit.json](exit.json): successful race
  binary/test log hashes, no native requests, owned PG stopped and data retained,
  private internal Docker network removed. No provider credentials are mounted.
- `historical.py ROOT` reads only original receipts/snapshots/timings; it checks
  their pinned hashes before reporting. Redirect to a new file.
- `run_lab.py` creates an exclusive private source copy and network, builds the
  race test binary offline, and runs exactly the two diagnostic tests. Upload
  this script and `latency_diagnostic_test.go.txt` together to the input path in
  the script; choose a new root for a fresh run. The `.go.txt` is copied into the
  frozen app test package, not compiled as an experiments package.

The first attempt exhausted Docker's predefined address pool. The script now
checks overlap and explicitly allocates an internal subnet. Its first actual
control run (r1) failed because the measuring provider wrapper hid the optional
inference-limit methods; Core correctly denied it. r2 embeds the concrete
scripted provider to retain those methods. Both roots and zero-spend failures
are retained; all r2 controls and journal replay passed without a race report.

No runtime optimization was applied. A subsequent change would need to preserve
revocation, authority and source fences while avoiding a full catalog reload for
every text fragment, and validate on live streaming independently of this
collector's buffering.
