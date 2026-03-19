# Butler Configuration Layering

Sprint 8 adds a layered configuration flow for the orchestrator settings surface.

Resolution order:
- environment variables override everything else
- database overrides from `system_settings` win over code defaults
- field defaults remain the fallback for unset keys

Operational notes:
- secret overrides can be encrypted at rest with `BUTLER_SETTINGS_ENCRYPTION_KEY`
- when the encryption key is unset, Butler falls back to plaintext storage and emits a startup warning
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
