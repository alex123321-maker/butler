# Pinned local observation transport

This opt-in Go adapter uses the existing ModelProvider contract with a private
Ollama HTTP/NDJSON server. It is text-only: tools, continuations, provider-side
stateful recovery, arbitrary endpoints/models and model-supplied options refuse.
No library dependency, credential or cloud fallback is added. The production
provider selector is unchanged; experiments compose NewProvider explicitly.

The fixed Qwen3:0.6b Q4_K_M manifest is
`7df6b6e09427a769808717c0a93cadc4ae99ed4eb8bf5ca557c90846becea435`.
Every inference first checks that this is the sole installed model. The lab
runner pins Ollama0.40.2 image
`sha256:b86366bb528bbf7f1424435d165028497a5b69bf6ddb4fa5a87102e2b79f44fb`,
uses a read-only model store, internal network, UID10001, two CPUs, 2GiB and no
cloud. One loaded runner and one inference slot serve distinct Butler contexts;
the adapter additionally serializes each instance and bounds its queue to four.

Input normalization retains typed instruction/data origins and rejects secret
content. Conservative UTF-8 byte plus per-message template admission bounds this
pinned byte-level tokenizer's4096-token context, including256 output tokens.
Complete serialized bytes are checked before HTTP. Generation is deterministic
temperature0/seed123 with thinking disabled; unexpected thinking or functions
refuse. Whole responses, lines, text and time are bounded. Duplicate JSON envelope
keys, incomplete streams, model drift and declared-token overrun refuse finals.
HTTP errors expose only fixed diagnostic codes, with no upstream body or URL.

Actual usage comes from explicit prompt_eval_count/eval_count. Missing/null counts
remain unknown, including when one count is missing; explicit zeros are valid.
Observed usage never refunds an execution allowance. Pricing local_no_billing
does not grant local_only authority: the existing generic admission retains its
privacy check until a separately verified private target is composed there.

Official protocol: [chat](https://docs.ollama.com/api/chat),
[model metadata](https://docs.ollama.com/api/tags),
[Qwen3 profile](https://ollama.com/library/qwen3:0.6b).

On laba only, after Sync into `/srv/storage/butler/agents/local-generative`:

```
bash /srv/storage/butler/agents/local-generative/current/scripts/dev/local-generative-lab.sh prepare
bash /srv/storage/butler/agents/local-generative/current/scripts/dev/local-generative-lab.sh check
```

Preparation downloads public weights, verifies the manifest, then replaces the
download process with the isolated runner. It sends no inference. The main
deployment, its model cache, its PostgreSQL state and the sealed native request
history are unchanged. Unit/race tests use synthetic servers and need no model.
Real generation is a separate bounded opt-in experiment; no quality or complete
delegation acceptance is implied by this component.

The first actual five-context experiment generated all finals with known usage
through one loaded CPU runner, but failed every source-ID/shape quality check.
The complete failure and fixed model/runtime versions are retained in
[the observation audit](../../../deploy/local-generative/README.md). No model
response from that experiment was accepted as a delegated result or executed.
