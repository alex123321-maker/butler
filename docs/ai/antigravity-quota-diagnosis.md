# Antigravity account and quota diagnosis — 2026-10-11

The owner reports desktop version2.22 and Flash remaining100% for the same
account used in the lab CLI. Installed official CLI is1.3.3. Codex reasoning and
critics use Codex, while the Butler native experiments use Antigravity OAuth
from that initial console login. No second Google login was performed.

The earlier private backend audit compared the saved CLI refresh authorization
with the prepared operator import and returned match booleans only. The new
`provider-auth -action audit-origin` read the current live encrypted PostgreSQL
record and that same private import. Its actual result was:

```json
{"stored_credentials":true,"oauth_application_matches":true,"oauth_credential_matches":true,"refresh_credential_matches":true}
```

Together these checks establish the same retained console authorization in the
live Butler store at observation time. The audit did not refresh, write, request
OAuth or infer. It returned no token/hash/account identifier. A future mismatch
could be rotation; equality does not establish paid inference quota accounting.

Read-only metadata queries using the saved CLI access token returned:

- AICode production `GET /v1:fetchConfig`:200; requested
  `gemini-3.8-flash-tiered` exists, base `gemini-3.8-flash`, configuration version
  `3.8-flash-08-2026`, input1048576/output65536. Butler retains its8192 bound.
- Cloud Code `loadCodeAssist`:200; base tier `free-tier` named Antigravity and
  paid tier `g1-pro-tier` named Google AI Pro coexist. A base-tier field alone
  does not prove that a generation used a free quota.
- Cloud Code `fetchAvailableModels`:200; the requested Flash entry reports
  remainingFraction1. This agrees with the desktop100% and is a server response,
  not merely a stale UI display. Account/project details remained private.
- Correct GET `v1:fetchQuotaStatus`:500 UNKNOWN; production discovery:404.
  Neither is usable evidence about exhausted quota or request-schema support.
- Separate version comparison: `v1:fetchConfig`200 with the requested experience;
  `v1main:fetchConfig`404 and `v1main/remote:getUserQuotaSummary`404. The CLI
  protobuf namespace `google.gca.aicode.v1main` is not evidence that `v1main`
  is a supported production HTTP version. Earlier actual production requests
  rejected its session/entitlement fields, so they were not blindly added.

Official CLI `models` is confirmed by offline `--help` as a metadata subcommand;
it was stopped after its bounded metadata deadline and is not a completed
same-prompt inference comparison. These diagnosis probes added zero actual model
requests. No automatic fallback, extra credits, API key or account rotation was
enabled. The sealed fifth native generation window remains immutable.

Prior actual Go and Node requests yielded429 RESOURCE_EXHAUSTED/HIGH_DEMAND,
identifying capacity for those particular failures. Old generic `rate_limited`
records, including the fifth window, cannot be reclassified without their
original evidence. The transport now preserves recognized HIGH_DEMAND as fixed
`model_capacity_unavailable`, discarding raw provider details. Google documents
capacity-dependent rate limits in [Plans](https://www.antigravity.google/docs/plans)
and backend refresh of quota in [CLI usage](https://www.antigravity.google/docs/cli/commands/usage).

Unresolved: whether the official client and Butler receive the same response to
an identical generation, and how these direct AICode generations enter the
desktop quota meter. A full meter alone proves neither a free route, a broken
generation contract nor freedom from capacity429s in the official client.

Actual audit validation used immutable source snapshot20261011-155541-905,
disposable encrypted PostgreSQL, race checks and vet on laba. The narrow report
`/srv/storage/butler/agents/provider-origin-audit/reports/provider-origin-20261011T085545Z-335155/tests.jsonl`
has SHA256 `0b68dcedbaa196037dea09f25112e990b5f1e15b6659b1d28a524f57a0afafbc`.
The expanded report
`/srv/storage/butler/agents/provider-origin-audit/reports/provider-origin-20261011T085605Z-336598/tests.jsonl`
has SHA256 `dffd92922bfbecc2169451c7609e55cc6f9583094081da0429b9a7103103514b`:
61 passed test/subtest names, three packages and no skips. It also built the
private static operator used for the live audit above. All checks added zero
actual model calls. The first failing duplicate-case fixture was preserved;
the importer now rejects case aliases rather than silently accepting them.
