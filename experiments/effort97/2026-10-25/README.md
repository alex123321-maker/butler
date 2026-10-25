# Issue97 synthetic native evidence

See the [results and limitations](../../../docs/testing/effort-research-results-2026-10-25.md).
Five closed windows are exported exactly from the isolated local PostgreSQL
research store. `manifest.json` fixes source/binary/schema/cases/order/budgets;
`attempts.jsonl` retains actual synthetic inputs/outputs, failures and nullable
usage; `states.jsonl`, `outcomes.jsonl`, `window.jsonl` retain durable lifecycle.
There are no credentials, account identities, working mail or auth-table dumps.

The two quality directories also contain offline summaries, per-case CSVs,
profile-blinded review inputs, a separately stored profile mapping and the
Codex semantic-review record (zero independent human raters). The mapping was
not used for the semantic review. Review is separate from frozen fixed-facet
grading. Neither NULL thinking nor unavailable total means zero.

`expense-journal.json` includes every declared native preflight and quality
window. In preflight b the two original UNKNOWN records were local constructor
rejections before HTTP; they are preserved unchanged. `validation/` includes
local test/compile evidence; compile-only output is not executed-test evidence.
`sha256.json` inventories evidence bytes, excluding itself.

Recompute derived summaries without auth, inference or PostgreSQL:

```powershell
go run ./cmd/effort-eval -action summarize -out experiments/effort97/2026-10-25/quality-v2-20261025a
go run ./cmd/effort-eval -action summarize -out experiments/effort97/2026-10-25/controlled-evidence-20261025a
```

Do not rerun native windows to reproduce a favorable score. New research needs
new prospective IDs/protocols and retains these windows and costs.
