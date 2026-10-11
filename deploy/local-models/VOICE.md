# Optional local Russian voice runtime

This runtime is disabled in the ordinary catalog. The evaluation catalog enables
the three fixed voice profiles for isolated acceptance only; it does not enable
microphone capture, auxiliary jobs, command ingestion or a main service pipeline.

Pinned upstreams are [Qwen3 ASR](https://huggingface.co/Qwen/Qwen3-ASR-0.6B/tree/5eb144179a02acc5e5ba31e748d22b0cf3e303b0),
[Qwen3 TTS CustomVoice](https://huggingface.co/Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice/tree/85e237c12c027371202489a0ec509ded67b5e4b5)
(Apache-2.0), and [Silero VAD](https://github.com/snakers4/silero-vad/tree/5cd7945676eb32225748052e2e6a0580e4686a08)
(MIT). These public checkpoints require no account token. Preparation checks the
listed Qwen tensor SHA-256 values and matches Silero's packaged tensor to the exact
primary Git blob. Metadata identity alone is not tensor integrity verification.

Run `prepare-voice.py DATA_ROOT voice-wheels.json` in the existing prepared Python
environment, with storage-only cache/temp and no private account/token mounts.
It uses public pinned wheel URLs, sizes and SHA-256 checks. The existing Torch
2.10.0+cu128/CUDA environment is reused. The isolated SDK overlays need official
Transformers 4.57.6 for ASR and 4.57.3 for TTS; NumPy 1.26 and compatible
SciPy/scikit-learn support the ASR SDK's imported DyNet helper. Torchaudio 2.10.0
matches existing Torch. No vLLM, flash-attention, demo web server, aligner weights
or voice-cloning inputs are added. The optional SoX module warns if its system
binary is absent; CustomVoice acceptance must prove its fixed path works without
relying on a missing external codec. Standard imports are not inference evidence.

The fixed TTS speaker is Ryan. Russian is an upstream supported language, but
Ryan is an English native preset, so pronunciation must be measured rather than
called a native Russian voice. CPU fallback remains subject to the same 6 GiB
RAM admission/reserve; the conservative TTS CPU peak can refuse instead of loading.
Only one GPU model is resident; the CPU VAD lane is separately admitted. Actual
Russian WER, names/numbers/negation, elapsed time and memory proof are pending.
