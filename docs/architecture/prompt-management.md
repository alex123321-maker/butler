# Butler - Prompt Management Contract

## Status

- Type: Architecture Subspec / Prompt Management
- Status: Draft baseline

## Purpose

This document defines how Butler assembles the effective system prompt for a run.

## Ownership

- Orchestrator owns final prompt assembly during `preparing`.
- Memory returns structured bundle data only; it does not assemble the final prompt.
- Transport carries normalized instructions to the model; it does not read memory or config directly.
- Tool availability remains authoritative in structured tool definitions, not in prompt text.
- Credentials and raw secrets must never be injected into prompt content.

## Prompt layers

The effective system prompt is assembled from these layers in order:

1. Base prompt
   - operator-authored prompt stored in `system_settings`
   - if missing, empty, or disabled, Butler falls back to the built-in safe default base prompt
2. Runtime context sections
   - session summary
   - working memory
   - profile memory
   - episodic memory
   - document chunks
   - optional tool summary derived from stable tool-contract metadata
  - browser strategy guidance for browser-oriented runs, including short follow-up interpretation rules, single-tab approval continuity, and recovery guidance needed to continue browser tasks safely across turns
3. Final transport-facing instruction payload
   - emitted by orchestrator as a normalized system instruction item

## Placeholder policy

The base prompt may reference allowlisted placeholders only:

- `{{session_summary}}`
- `{{working_memory}}`
- `{{profile_memory}}`
- `{{episodic_memory}}`
- `{{document_chunks}}`
- `{{tool_summary}}`
- `{{browser_strategy}}`

Rules:

- placeholders inject labeled section text, not arbitrary values
- unknown placeholders are omitted safely and surfaced in preview results
- missing sections resolve to empty output without breaking the run
- no arbitrary template execution, scripting, or expression evaluation is supported
- tool summary is informational only and never replaces structured transport `ToolDefinitions`

## Section formatting and order

- section order is deterministic: session summary, working memory, profile memory, episodic memory, document chunks, tool summary, browser strategy
- empty sections are omitted
- if a section is not inserted through a placeholder, orchestrator appends it after the base prompt in the same deterministic order

## Safety rules

- operator prompt updates are rejected when they contain secret-like material such as tokens, passwords, cookies, or credential-bearing DSNs
- runtime sections must be sourced from already safe memory bundle outputs
- prompt assembly must never resolve `credential_ref` values or expose provider auth material

## Storage model

The current minimal storage path uses `system_settings` keys:

- `BUTLER_BASE_SYSTEM_PROMPT`
- `BUTLER_BASE_SYSTEM_PROMPT_ENABLED`

Revision metadata uses existing `updated_at` and `updated_by` fields on those settings rows.

## API baseline

The orchestrator exposes operator-facing REST endpoints for:

- reading the current prompt configuration
- updating the stored base prompt and enabled flag
- previewing the effective assembled prompt for a session-oriented context

## Budget policy

The baseline implementation applies deterministic character limits to:

- base prompt text
- each runtime section
- the final assembled prompt

Oversized content is truncated with an explicit marker rather than silently dropped.
