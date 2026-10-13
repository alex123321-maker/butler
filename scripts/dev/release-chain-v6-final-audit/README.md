# V8 independent final checker overlay

This ignored overlay extends `scripts/dev/release-chain-v6-final-audit` with the explicit `-protocol=v8` policy. V6 remains the default, and V7 remains unchanged. The measured source/runtime and the independently built auditor source/runtime have separate pinned identities.

The V8 policy binds the sealed actual baseline of 541 requests, 74 remaining admissions and the absolute ceiling of 615. It binds prerequisite `effa65a6e39fcecce2510c76554685d047ee5e8c7ed1b9e6d27e633133d402ec`, human amendment `3560bb090a6b2c8b8bc78a987b2331eaf2eb55550b5e62572fbaf0bb7646973d`, and the exact retained V7b history and NO_GO artifacts. Old missing measurements and the unknown specific cause of request 541 remain unchanged.

In addition to the existing named safe input files, stage one directory at `/proposals` containing only the exact original own-tool proposal files named `tool-proposal-%06d.json` for the ordinals in `scope.proposal_observations`. Mount it read-only. Each leaf must be a regular non-symlink file with mode `0600`, match its pinned digest and size, and be at most 262144 bytes. Do not mount metadata files, the containing native-private directory, provider SSE, credentials, or profiles. Public proposal metadata is already in the safe scope file.

The checker compares the retained byte copy with `ToolCall.ArgsJSON`, its unchanged authorization hash and the actual Core action row identified by run, tool call ID and tool name. Every completed native tool packet and every recorded product action must match exactly once. The legacy lexical integer diagnostic is only a boolean diagnostic; it is not an acceptance predicate.

Validation used the Go overlay in `overlay.json`: the 18 exact V8 names in `required-tests.txt` and two V6/V7 compatibility tests passed, with zero failures or skips. `go vet` passed. The seven Go files are pinned by `handoff-sha256.json`. No provider calls, credentials or tracked source edits were used. A recovered unknown-cost timeout followed by a tampered proposal must report NO_GO with zero accepted subset counters and unchanged factual unknown cost.

Directory or named-file identity failures before the audit may produce empty stdout and a nonzero process exit. They are NO_GO. Such failures must never be interpreted as a passing report. Proposal tampering inside the audit produces a failed report with factual cost accounting.
