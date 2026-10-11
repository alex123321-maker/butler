# Butler Configuration Layering

Sprint 8 adds a layered configuration flow for the orchestrator settings surface.

Resolution order:
- environment variables override everything else
- database overrides from `system_settings` win over code defaults
- field defaults remain the fallback for unset keys

Operational notes:
- secret overrides can be encrypted at rest with `BUTLER_SETTINGS_ENCRYPTION_KEY`
- when the encryption key is unset, Butler falls back to plaintext storage and emits a startup warning
- provider OAuth uses a separate strict encrypted store; private Antigravity import
  and refresh persistence require the encryption key and have no plaintext fallback
- hot settings update the in-process hot config container immediately
- cold settings are persisted and surfaced as `requires_restart=true` until the service restarts
- prompt-management settings follow the same env > database > default layering, and the operator base prompt uses hot updates so the next run sees the new value without restart

Prompt-management keys:
- `BUTLER_BASE_SYSTEM_PROMPT` - operator-authored base prompt text stored in `system_settings`
- `BUTLER_BASE_SYSTEM_PROMPT_ENABLED` - toggles whether Butler uses the stored operator prompt or falls back to the built-in safe default base prompt

Prompt revision metadata:
- the baseline reuses `system_settings.updated_at` and `system_settings.updated_by`
- missing or empty prompt values fall back to the built-in safe default base prompt

Display masking:
- passwords and connection strings are rendered as `••••••••`
- API keys are rendered as `...XXXX`
- Telegram bot tokens are rendered as `XXXXXX:...XXX`
# Context admission

`BUTLER_CONTEXT_BUDGET_TOKENS` is a positive integer, defaults to `65536`, is
available through layered environment/database settings and requires a restart.
It caps the conservative initial request estimate described in the memory model;
it does not report native provider usage or guarantee output-token headroom.
Operators should set it below their chosen model's effective context capacity.

`BUTLER_ANTIGRAVITY_MODEL` defaults to `gemini-3.8-flash-tiered`. Native Go transport
has no configurable CLI executable/profile or caller-controlled endpoint. OAuth
material is a hidden provider-auth record, outside the editable settings catalog.
Selecting the provider requires an operator-verified real text/tool continuation;
stored credentials alone do not establish subscription model availability.

Owner decision authority is operator-owned rather than a database setting.
`BUTLER_OWNER_CONTROL_OWNER_ID` and exactly one of `BUTLER_OWNER_CONTROL_TOKEN`
or `BUTLER_OWNER_CONTROL_TOKEN_FILE` are read only from the environment/file.
Database overrides cannot assign these fields. Missing authority denies Web
approval decisions; the token is scoped to owner decisions and separate from
extension/private-service authentication. See the
[durable execution contract](durable-execution-spec.md) for provisioning.

`BUTLER_EXECUTION_BUDGET_POLICY_JSON` is validated deployment operator policy,
outside the editable managed settings catalog. Environment overrides any legacy
database row; Orchestrator and Broker must resolve identical policy. Finite defaults
remain for omitted fields, but exact model price basis defaults to unknown and
denies inference. Changed defaults cannot reset or replace immutable PostgreSQL
scope policy. See [execution budgets](execution-budget-contract.md).

Reviewed converter child composition has a separate operator-only
`BUTLER_CONVERTER_DELEGATION_ENABLED` flag, default false in core and Broker.
Stored settings cannot enable it or select the core's
`BUTLER_CONVERTER_PROFILE_JSON`. Enabling ordinary converters and context-only
delegation does not enable this distinct developer role. Broker's operator-only
`BUTLER_CONVERTER_DELEGATION_CONTROL_URL` must name exactly
`/internal/converter-delegation`, with no URL credentials, query or fragment.
These settings supply startup composition only; current source/task/owner,
accepted role, exact dispatch and finite budgets still supply authority.
Core additionally requires ordinary delegation, the guarded converter control
endpoint, exact runtime profile and immutable053/054/055 readiness; Broker requires
enabled converters and ordinary delegation. The core's private projection sets
`converter_delegation_ready` only after both role lifecycle and all current
converter guards are composed. Disabling composition does not revoke or undo
already dispatched effects; see the generated converter operational contract.

The existing developer opt-in also composes its two bounded automatic parent
continuations; there is no separate model-visible scheduling flag. Core requires
declared finite provider request limits with output no larger than the current
execution policy. It uses that policy's conservative quote without claiming to
lower a provider request cap. The opaque continuation reader is absent when the
flag is off. Broker/final source guards still recognize retained055 inputs,
which cannot acquire ordinary owner authority by disabling new admission.
