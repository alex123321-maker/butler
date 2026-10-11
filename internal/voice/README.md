# Optional local voice caller

This Go caller keeps voice as private catalog data. It does not own task lifecycle,
session leases, model transport or tool permissions. The optional Web/API adapter
requires existing owner authentication; speaker recognition supplies no authority.
All voice profiles and API composition are disabled by default. The frozen human
Russian quality gate failed; do not enable automatic command acceptance.

`Store.Upload` accepts canonical mono 16 kHz PCM16 WAV, at most 120 seconds, an
explicit owner-command/note/external role and a finite 1–168 hour access lifetime.
PostgreSQL catalog assets retain originals. Source-linked transcripts/reviews use
immutable derived DATA; clip records and queued inputs contain refs/digests only.
The independent deletion anchor, source graph and restore guard erase originals,
derivative text and affected queued/prepared projections. Access expiry denies
reading/new inference; physical deletion requires the explicit catalog deletion
operation. Neither expiry nor a browser close promises automatic file purging.

`Processor.ProcessOne` claims only `ingest/voice_asr` owned jobs. Every VAD/ASR/TTS
call independently reserves the shared owner/task/mandate/job budget, verifies
the fixed local target and exact installed profile and rechecks source/clip/claim
state after waits. VAD runs on CPU. Speech is split into 15-second ASR requests,
at most 16 per recording. Long continuous splits, empty results and unfinished
decoding remain partial. Worker failures preserve the original and expose fixed
error categories; no automatic tool execution or cloud fallback occurs.

Owner commands require an explicit review, critical-details acknowledgement and
current session/task revision. Notes/external/partial records cannot confirm.
Ordinary review uses `VoiceInputAdapter` and the existing accepted input queue in
the same transaction. Stop/restrict/pause/resume uses `VoiceControlAdapter` and
the common controller. Its same-transaction `ApplyWithGuard` validates current
review/source/clip before any mutation. Lost ACK retries retain the same controller
receipt. Cancellation of transcription is a separate operation from task Stop.

The post-lease source hook privately resolves the exact queued binding. Durable
preparation and approvals retain opaque refs instead of recognized text. Recovery
requires the same binding and current source access; deletion/restore cannot
release an old cached body. Generic selected DATA projection also supports trusted
delegation composition; selected IDs never grant access by themselves.

`Processor.Present` synthesizes an existing exact authorized catalog source using
one owner-created finite `ingest/voice_tts` job. `ClaimOwner` selects this exact
owner/job/kind/purpose rather than borrowing the next queued job. Output is
presentation only, never ingress. Request cancellation interrupts synthesis;
source text remains readable if synthesis fails. The fixed Ryan preset is used;
voice cloning, arbitrary speakers and execution options are unavailable.

Composition requirements:

- Full migrations, including 044 and 046–049; current catalog/deletion anchor,
  shared controller and fixed local target already used by owned memory jobs.
- A single private 600 token-file mount for `semantic.NewPinnedBackend`, fixed
  `local-models:9090` operator identity and explicit `local_no_billing` policy.
- Optional `VoiceServer{Enabled:false}` registered under `/api/v3/voice/` using
  the existing owner-control authentication; explicit operator activation is a
  separate choice from merely registering an HTTP handler.
- `VoiceInputAdapter` uses the existing input Store and current source hook.
  Chain other trusted source adapters by stored binding; never API-selected body.
  `VoiceControlAdapter` uses the current task controller with tree fences retained.
- A bounded background `Processor.Run` only when this optional caller is enabled.
  Configure all three pinned runtime profiles separately; a missing/disabled
  profile fails closed. No changes to ordinary model transport or legacy memory
  workers are needed.

See [voice contract](../../docs/architecture/russian-voice.md) for evidence,
remaining acceptance work and the default-off decision. Real-model tests require
explicit lab-only environment variables and a private token file. Normal tests
exercise actual PostgreSQL source/budget/control boundaries with a mocked model.
