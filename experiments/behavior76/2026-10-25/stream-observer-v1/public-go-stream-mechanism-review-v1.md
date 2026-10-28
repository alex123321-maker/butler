# Independent Go stream mechanism review, public v1

Verdict: no mechanism-level blocker found in the reviewed source. This is not
collector wiring, prospective study freeze, native admission or quality proof.

Reviewed exact source SHA256:

- behavior76_authority_stream_test.go:
  `8db6b29580607b9bedc69c326f78e1ec9a05960b0eff420a5628fa28af46ff4e`
- behavior76_authority_trace_test.go:
  `5db699f29d721a38d351c01dfdb692485435b0baf8cc50a318732421a9939824`

The tracked tracer diff adds only the opt-in stream pointer, append branch and
snapshot/serialization dispatch. Existing SQL family/hash generation, arguments,
deadline sampling, pool reads, operation failure categories and original flat-v1
capacity behavior are unchanged when stream is nil. Legacy serialization uses
the same exported fields and original tags. The recorder and JSON callback are
research test source, not a production dependency or configuration change.

Recorder state, actual sequence allocation, buffering, flush and seal decisions
are protected by the existing observer mutex. The stream retains at most 4096
event records at a time, a bounded compressed batch, and at most 16384 segment
descriptors. Each descriptor follows successful CreateNew write, Sync and Close;
byte/hash data covers the exact compressed and LF-terminated raw buffers. Header,
line, active-span, segment and total compressed-byte limits fail globally. The
independent reader checks actual on-disk bytes and correlations separately.

Write/collision/cap/correlation/late-event failures set observer_failed and the
same shared GLOBAL flag; they cannot be interpreted as a classified local
synthetic UNKNOWN. New callbacks after failure do not resume recording as a
healthy stream. An attempted late start/end after sealing retains failure/drop
counters and marks GLOBAL, so even an earlier healthy header cannot excuse an
actual later fault. Non-ok SQL/acquire results retain operation_failures without
newly declaring every ordinary cancelled rollback an observer failure.

Seal flushes the final actual partial batch only when open=0, dropped=0 and the
observer is healthy. It marks a failed header unqualified; no synthetic end
callback is inserted. Publication and actual worker/runtime/lease quiescence
remain the collector's responsibility. Future wiring must use the agreed new
constructor/protocol only, propagate startup/publication errors globally,
publish actual segments before exact inventory sealing, and retain the shared
late-event/admission checks. These are existing contract obligations, not a new
test matrix or permission for a native call.

Actual independent cross-reader result on the Go-produced PUBLIC simulated
fixture: 65,570 events, 32,785 paired spans, 17 segments, 969,004 compressed bytes,
two retained cancelled-operation errors, zero open/drop/correlation failures.
Reader used unchanged pinned v2a source and independently expected identity.
No hidden controls, mapping, PG, model, credential or native access occurred.
These simulated callbacks prove representation compatibility, not production
authority timing or resolution of the retained historical 750 ms failure.

Crossproof: `public-go-stream-reader-crossproof-v1.json`, SHA256
`a451054a66a5f6604afe5f2a8fb46c4334bdaaa54b06ac11c8963bb10bb35a4a`.
Its inventory pins all 19 public fixture files, including the marker and actual
header SHA `5abb4269df9c06faf58c257440fe54172d52377bbc2faf08e1e23bba2de11d60`.
Original local-r2 failure, 405-row inclusive accounting and masked judgments are
unchanged. No fresh hidden controls or new paid study were authorized here.
