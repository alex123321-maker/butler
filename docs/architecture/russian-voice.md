# Russian voice experiment

The optional voice caller is default-off. Its core safety contracts have real
PostgreSQL/race and browser evidence, and the three pinned local models execute
on the lab. Independent Russian speech testing fails the frozen quality gate.
Issue #29 remains active; this experiment is not production acceptance.

A separate [local Intel Arc feasibility probe](../testing/issue29-local-arc-probe.md)
passed on this PC on2026-10-26 using the pinned Windows XPU SDK. This fixed
tensor diagnostic changes no service backend and does not qualify resource limits,
Russian speech quality, the memory workload or the failed original gate.

## Boundaries and privacy

Voice belongs to ingress/presentation plus catalog-owned memory. Go controls
owner proof, source roles, finite jobs/budgets, current-source checks and review.
The existing auxiliary service controls tensor loads, deadlines and 6 GiB GPU/RAM
admission. Its documented Python exception uses the pinned upstream inference
SDKs, without remote code, online model resolution or new cloud dependencies.

The browser asks for microphone permission only after the owner clicks the
button, displays recording state and releases tracks on Stop/unmount. It creates
mono 16 kHz PCM16 WAV with a 120-second limit and sends it only on explicit upload
to Butler. Browser permission and voice characteristics are not authentication.
Uploads default to note; external files are marked external. Only the separately
authenticated owner can explicitly review a complete owner-command recording.
Names, numbers, recipients and negations must be reviewed; decoder EOS and model
confidence cannot prove correct intent.

Original WAV bytes, ASR previews and owner corrections are versioned catalog
assets/sources with full parent lineage. Clip/input/job metadata retain hashes
and refs. Voice-endpoint access lifetime 1–168 hours is separate from physical deletion. Owner
deletion removes audio/derived text and affected queued/prepared copies under the
persistent deletion anchor; restore reconciliation denies old snapshots. There
is no automatic audio upload to a cloud provider. Owner catalog administration
can retain/export original sources until explicit deletion. Voice TTS also checks
the lifetime of every audio ancestor and bounds its job/request by that expiry.
Explicitly confirmed ordinary
text input subsequently follows the existing cloud/local privacy policy.

Ordinary confirmation atomically creates one opaque accepted input. After the
new live lease is acquired, a trusted stored-binding hook resolves current text
and pins its complete source graph. Checkpoints/approvals store refs, not copies
of ASR prose. Recovery denies missing/replaced bindings and reloads current data.
Trusted selected DATA refs are similarly projected without modifying instructions
or actual tool/effect observations; refs do not supply their own authority.

Reviewed voice Stop/restriction uses the common owner task controller, explicit
task revision and a same-transaction source guard after authority locks. A lost
ACK preserves its stable delivery receipt. Raw/partial/external speech cannot
call it. The ordinary text/UI Stop remains available when voice is unavailable.
Transcription cancellation never claims to stop a task or undo an external effect.

TTS consumes an existing authorized source, independently admits one finite job
and returns bounded audio. It neither creates a task nor feeds output into ASR.
The owner can stop the request/playback. Original text survives synthesis failure.
The API accepts a typed current source ref; the page demonstrates reviewed text
presentation. Automatic spoken-answer delivery is not installed by this caller.

## Runtime and frozen evidence

Qwen3-ASR-0.6B revision 5eb144179a02acc5e5ba31e748d22b0cf3e303b0,
Qwen3-TTS-12Hz-0.6B-CustomVoice revision 85e237c12c027371202489a0ec509ded67b5e4b5
and Silero VAD v6.2.3 revision 5cd7945676eb32225748052e2e6a0580e4686a08 are
operator-pinned. ASR accepts≤15 seconds per call; VAD≤120 seconds; TTS≤200 Unicode
characters/1024 bytes and≤20 seconds of canonical 24 kHz PCM16. Russian and the
fixed Ryan preset are selected internally. No compressed codecs, arbitrary
sample rates, speaker cloning, language/model URLs or execution settings are args.
Official sources: [ASR](https://huggingface.co/Qwen/Qwen3-ASR-0.6B),
[TTS](https://huggingface.co/Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice),
[VAD](https://github.com/snakers4/silero-vad/tree/v6.2.3).

The public Google FLEURS CC-BY4 corpus is pinned to
70bb2e84b976b7e960aa89f1c648e09c59f894dd. Eight human recordings were selected
before inference: first four bounded records plus four additional negation/number
records. Repeated sentences across speakers make this a small engineering probe,
not a population accuracy estimate. The fixed gate is WER≤0.20 and zero critical
misses. All eight cases are retained in
[human-quality report](../../deploy/local-models/voice-human-quality.json):
16/126 word edits, WER 0.126984, zero call failures, four critical misses. Three
proper-name recordings substitute Карпенко for Карпендо. The fourth flag concerns
the unchanged literal-number checker for 117 versus inflected “сто семнадцати”.
No held-out threshold/checker tuning was performed; name failures alone fail the
gate. Measured warm VAD+ASR latency was 864–2012ms.

The [synthetic report](../../deploy/local-models/voice-synthetic.json) records
TTS 13.876s plus VAD/ASR 12.191s, three actual durable reservations and zero retained
original bytes after deletion. The self-roundtrip preserves “Не отправляй Ивану
семнадцать файлов.” It is not independent human speech or a listening-quality
rating. Observed whole-device GPU peak 2373 MiB and process RSS peak 3101 MiB were
below policy; no OOM/cloud fallback occurred in that measured window. Models
load/switch under the shared service rather than remaining resident together.

Earlier raw-cgroup RAM refusals remain in
[refusal report](../../deploy/local-models/voice-human-resource-refusal.json).
The separately tested RAM accounting fix credits only bounded clean inactive file
cache, conservatively excludes dirty/writeback pages and retains hard 8 GiB and
RSS/reservation limits. It does not flush host caches or change quality results.

[Actual bounded failure matrix](../../deploy/local-models/voice-failures.json)
retains all five cases: silence/seeded background noise become empty partial
previews; a 21-second repeated public human note completes as note DATA in 13.140s
without an accepted input; real CPU-only ASR refuses admission with
`ResourceExhausted`; a closed caller connection marks its job/clip failed while
retaining the original. The
[initial matrix](../../deploy/local-models/voice-failures-initial.json) also remains:
its direct helper left a cancelled long case pending, and its stale pre-enqueue
CPU clip revision failed before RPC. A subsequent run lost its report to a file
ownership error; it is not claimed as acceptance. The final full ProcessOne run
uses current clip state and preflight/atomic report persistence. Neither that fix
nor the repeat retunes the human corpus or replaces its failed gate.

## Acceptance matrix

| Issue #29 criterion | Evidence and status |
| --- | --- |
| Explicit mic with visible state | Chromium fake-device capture; no page-load permission, fixed WAV, track release; PASS. Human hardware/browser permissions remain operator-specific. |
| Audio role; no automatic owner authority | Typed role/current owner review; external/partial refusal in actual PG and UI; PASS. |
| Voice not sole authentication | Existing owner proof before request body; foreign/missing proof refused; PASS. |
| Partial/ambiguous content checked first | Partial cannot confirm; critical-details checkbox/current revision required; PG/UI PASS. Automatic name/number disambiguation is unavailable; frozen quality gate FAILED. |
| Common Stop plus text fallback | Actual controller/source guard/lost-ACK receipt/foreign-source denial; text Stop while voice disabled in browser; PASS. Parent release must retain tree ownership fences. |
| Duration/silence/noise/errors/restart original | Fixed WAV/segment/output limits, real failure report and PostgreSQL original restart/delete proof; bounded safe refusals retained, not proof that every long note succeeds. |
| Explicit retention/deletion/transmission | Access TTL versus explicit physical delete; catalog lineage/pre-message purge/restore denial; PG PASS. No automatic TTL purger or cloud audio path. |
| Russian accuracy/critical fields/latency/memory with #24 | Actual independent 8-case report and GPU/RSS measured; names fail. Mixed Russian technical speech and independent human TTS listening remain unproven; simultaneous production memory-workload acceptance remains open. |
| Interruptible TTS, text retained, no new task | Current source/job admission; browser cancellation/failure preserves text, bounded actual synthetic output; PASS for implemented presentation adapter. Automatic channel delivery remains absent. |

Real local models are restored to the normal catalog with voice profiles disabled
after each bounded lab window. No core/native/main PostgreSQL rollout is part of
the optional runtime staging. Issue #29 stays active pending unmet quality/workload
and production-composition criteria; recording/ASR never supplies a mandate.
