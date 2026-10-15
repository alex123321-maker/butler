# Local change reconciliation, 2026-10-15

Source checkout: `D:/Repository/butler`, HEAD
`d343485f550ce74cd1f46b3e02215169ba4cf295`, 55 tracked modifications and
15 untracked files. Destination starts at current GitHub main
`4dc7fa8bc9138b025550a0160c29ee0fb18fd4c2`. The original checkout is retained.
The destination's ignored `.cache/local-port/` contains the original binary
patch, all 70 source files and a raw SHA-256 preservation manifest.

## Implementation decisions

| Original change | Main implementation and resulting decision |
| --- | --- |
| In-process autonomy counters, mode config, risk-based approval shortcut | Main already has operator `BUTLER_AUTONOMY_MODE`, PostgreSQL root/mandate/day budgets, exact-action authorization, restart fencing and durable approvals. Retain those implementations. Do not import counters that reset or zero-as-unlimited semantics, or a second approval decision in Orchestrator. |
| Class/contract network allowlist intersection | Retain main's public egress policy and exact release authority. Add only an optional Broker startup destination cap. Check the class cap and original contract independently; disjoint lists and an explicitly empty class deny. Never replace the contract's hashed release declaration with the cap. Core and runtime authorization remain mandatory. |
| Tool contract metadata reads | Replace placeholder `{}` argument validation with the existing GetToolContract RPC. Missing metadata is an error. Exact-action authorization remains the execution decision. |
| Transport presets and automatic unsupported-option retries | Retain existing explicit options in the transport contract and budgeted provider admission. Do not import model-selected presets, default parallel calls (main rejects batches), hidden retries or removal of an admitted output limit. Native Antigravity rejects arbitrary options. |
| Instruction continuity | Codex already retains instructions. Add the missing OpenAI Responses/Realtime run continuity, preserve instructions across tool responses, clear after final/error/cancel/shutdown and clear a reused Realtime session for a new instruction-free run. No provider identity prompt or extra request is added. |
| Default prompt and section markers | Port the useful execution/completion guidance. Adapt failure guidance to current durable retry/effect rules and explicitly preserve negation. Retain main's typed context provenance and data separation rather than replacing it with text markers. No quality improvement is claimed without a model trial. |
| Keyword-driven screenshot intent | Exclude the rewrite completely. The exact structured tool proposal goes through current authorization unchanged. A negative or quoted screenshot mention cannot turn status into capture. This is covered through a controlled full Orchestrator tool loop. |
| Legacy bind/status recovery, force rebind, capture after bind, multimodal rereading | Main's guarded single-tab path owns resource/selection/dispatch authority. Do not import legacy automatic recovery/replay, action bundling or unguarded screenshot-to-model injection. Retain existing disabled-by-default guarded composition. |
| Relay heartbeat, browser instance and duplicate legacy approvals | Port the narrow relay grace/identity fixes. Reuse only pending unexpired bind approvals for the same run/session; reject foreign, resolved and expired records. No guarded feature is enabled. |
| Artifact UTF-8, base64 and capture constraints | Normalize human-readable payloads, retain identity fields unchanged, truncate summaries on rune boundaries. Add migration070 for existing browser_capture/image_data_url support; do not edit applied migration023 or reuse numeric029. Rollback preserves captures and fails if incompatible records remain. |
| Telegram draft and per-tool noise | Suppress transient drafts and chat messages for each tool; retain chat actions and the current durable outbox, acknowledgements, owner controls and final persistence ordering. |
| Settings descriptions and hints | Port additive metadata/UI rendering for existing settings only. Keep current keys, defaults, validation, secrecy and owner authorization. The Broker cap is operator-only and not a mutable UI setting. |
| Old eval-harness JSON/NDJSON averages | Main has scenario/fault/semantic evaluation, pinned sources, independent review and full cost evidence. Do not add a second score-only harness whose supplied booleans do not prove task success. |
| OpenCode package lock | Tool-specific generated dependency file; no Butler runtime change to transfer. |

## Verification

Validation results and remaining limits are recorded here after execution.
Tests with controlled providers/Telegram/HTTP responses do not establish native
model quality, a real user-tab workflow, Telegram account delivery or production
readiness. No deployment or model inference is part of this port.
