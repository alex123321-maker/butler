# Issue29: local Intel Arc feasibility probe

Prepared only on2026-10-26. No SDK installation, model download, tensor inference,
microphone capture or service modification has occurred. Run only after the
current issue76 measurement window closes, with other heavy jobs stopped.

This PC reports Core Ultra9 185H / Intel Arc, driver32.0.101.8132. WMI's AdapterRAM
is not a measured usable accelerator budget. The existing pinned voice worker
requires CUDA for its GPU lane and remains unchanged/default disabled. Its
failed name-quality gate, original eight cases and thresholds remain unchanged.

PyTorch2.10 documents Windows11 support for the Meteor Lake-H family and BF16
XPU operations. This makes a separate hardware probe reasonable; it does not
establish actual availability, Qwen compatibility or Butler resource admission.
Use the official Windows CPython3.12 wheel for torch2.10.0+xpu, SHA256
`3ad605be4728b6d3a28a44d07dd794b1a9e45551b0057815bf25eb2a6d6a56a7`,
in a fresh repo-local virtual environment. Keep the resolved dependency report
and package hashes before execution; use binary packages, no global installs,
nightly substitution, driver changes or automatic version fallback.

The fixed Python diagnostic imports only the official tensor SDK, explicitly
requires XPU, and checks one2x2 BF16 multiplication against an exact result. It
loads no weights/audio and accepts no model, device or generation request.
Python is justified by that SDK; Go services and admission remain unchanged.
The PowerShell parent has a120-second deadline, two CPU-library threads, a
sampled2GiB RSS stop and requests child-tree termination on failure, then waits
for the parent process. This is not measured process isolation or a hard memory
boundary; a tensor PASS is not a resource qualification.
The environment is an explicit allowlist with repo-local caches; no account or
provider configuration is inherited. The script makes no network call, without
claiming OS-enforced egress isolation. Every run has a fresh output identity;
failure/unknown and raw diagnostic logs remain retained.

Only a successful probe can justify a separately frozen ASR experiment. A
possible larger model is Qwen3-ASR-1.7B, public revision
`7278e1e70fe206f11671096ffdd38061171dd6e5`, Apache2.0. No inference plan for that
candidate is admitted here. Its complete file hashes, fresh human speech corpus,
unchanged critical-name/number/negation rubric, real resource containment and
memory-workload composition must be fixed before inference. Existing failed
controls cannot tune decoding or be replaced by self-generated speech.

Sources checked2026-10-26: [PyTorch2.10 XPU support](https://docs.pytorch.org/docs/2.10/notes/get_start_xpu.html),
[official wheel index](https://download.pytorch.org/whl/xpu/torch/),
[Qwen official model card](https://huggingface.co/Qwen/Qwen3-ASR-1.7B).
