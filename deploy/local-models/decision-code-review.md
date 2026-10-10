# Pinned decision-model loading review

This evaluation is text-only and default-off. It adds no dependency to the
existing Torch2.10.0+cu128/Transformers5.19.0/GLiNER2.1.0 runtime and grants no
tool, task, owner or verification authority.

`fastino/GLiNER2.5-multi-Decide` is pinned to
`e173bca1f0c4217d7a55b3b0c705d5ece8a0218d`, uses the existing reviewed GLiNER
loader with no custom model code, and has Apache-2.0 metadata in the
[official card](https://huggingface.co/fastino/GLiNER2.5-multi-Decide/tree/e173bca1f0c4217d7a55b3b0c705d5ece8a0218d).
Its published English suite is not Russian acceptance evidence.

`LiquidAI/d1-omni-600M` is pinned to
`02b55d7076f15129e59ab3f94783f32c4b088674`. Its
[LFM Open License1.0](https://huggingface.co/LiquidAI/d1-omni-600M/blob/02b55d7076f15129e59ab3f94783f32c4b088674/LICENSE)
includes a commercial revenue threshold of$10M, unlike Apache-2.0. The current
personal/research evaluation is separate from future redistribution/commercial
deployment decisions. The license stays with the downloaded checkpoint.

The five upstream modules were read before execution:

| File | Reviewed behavior |
|---|---|
| modeling_d1.py | local model configuration, tensor API, fixed choice distribution, cached tokenizer |
| encoder.py | neural trunk/head, attention/convolution and tensor operations |
| prompt.py | bounded option/state serialization, reserved-token escaping and decoding |
| vision.py | image tensor preprocessing and local SigLIP construction; not exposed |
| audio.py | waveform/mel/conformer computation; not exposed |

No module uses subprocesses, arbitrary file IO, network requests or credential
lookup. Upstream tokenizer loading uses `name_or_path`; Butler explicitly
installs a locally loaded `trust_remote_code=False` tokenizer in that cache.
The dynamic model loader only sees a read-only local directory, offline
environment and exact reviewed code/config SHA-256 values. Go first checks all
seven files through contained bounded `workspace.ReadRegular`; Python repeats
regular-file/hash validation before loading. Missing/changed/symlinked code is
denied. This is one explicit reviewed exception, not permission to execute any
HF repository or future revision. Dependencies retain their separate supply-chain
risk; hash pinning is not a general OS sandbox.

Upstream `prompt.encode` silently truncates state/options. Butler independently
builds the complete exact token sequence and requires equality with upstream
encoding and the operator profile cap. Oversized input is refused without
truncation. GLiNER classification also checks actual text+schema encoding.
CPU-only FP32 profiles are selected explicitly; d1's public card recommends
avoiding BF16 for answer stability. Images/audio are not accepted by these
profiles, and no Russian voice or image decision quality is claimed.

`download-decisions.py` downloads only immutable public revisions with
`token=False`, explicit allowed files, no model-code imports and verified
safetensors SHA-256 values from the upstream LFS catalog. Everything stays in
the existing storage model root, with no dependency/image layer installation.
`models.json` keeps both profiles disabled. A separate
`models.decisions-evaluation.json` enables them only for an operator-controlled
lab evaluation; ordinary runtime/task execution must not use that catalog.
