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

Display masking:
- passwords and connection strings are rendered as `••••••••`
- API keys are rendered as `...XXXX`
- Telegram bot tokens are rendered as `XXXXXX:...XXX`
