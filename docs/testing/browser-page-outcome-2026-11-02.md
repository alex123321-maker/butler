# Exact managed-page live outcome review — 2026-11-02

Continuation of #164, stacked on draft PR181. Implementation retains mandatory
POST/unsafe approval and adds a bounded live owner factual review for locally
acknowledged page click/fill/type. It does not implement autonomous receipts,
federated credentials, Google login, historical/lost-ACK page reconciliation or
native delivery. No shared lab deployment or merge.

## Timing

Operator window starts **2026-11-02 02:59:46 UTC / 09:59:46 Tomsk** and includes
model deliberation, design/spec lookup, implementation, failed attempts,
builds/tests and CI waits until the final report. It is separate from browser
tool latency. Previous native slots and their metrics were not reconstructed.
All acceptance fixtures here are mechanical (`model_calls=0`).

## Local evidence

- `page-outcome-pg-final.jsonl`: sixteen new real PostgreSQL/race cases cover
  confirmed/absent/unknown, acknowledged navigation to a new document on the
  same page, foreign page/document, ordinary snapshot substitution, changed
  runtime body, owner epoch/mode, closure, expiry, constraints and post-request
  document/epoch changes. A historical managed request is denied. Grants remain
  consumed and the original tool transcript stays UNKNOWN. Final package21.017s.
- `page-outcome-pg.jsonl`: new and existing legacy owner outcome regressions
  passed against isolated PG/race (68.902s package), without selected skips.
- `page-outcome-core.log`: the deterministic Core probe uses the saved
  post-effect page ref once, rejects another prepared page or permission,
  rejects another returned document and does not copy original input. Legacy
  one-probe/no-model-resume tests remain passing (1.649s package).
- `page-outcome-runtime.jsonl`: eight actual Windows Chromium cases passed,
  including freezing one page while another stays frozen. Existing equal-title,
  popup/cap, owner fence, close and runtime-loss cases remain passing; package
  9.337s, runtime parent7.99s. The final required CI additionally tests denying
  page freeze after runtime loss.
- `page-outcome-broker.jsonl`: actual Broker/PG/resource kernel/Chromium passed
  in **3536.772ms fixture time** (4.771s package). One recipient POST, a genuinely
  captured frozen read of the wrong page rejected, exact page read admitted,
  other-page write blocked while waiting, owner attestation then canonical
  confirmed state, original UNKNOWN transcript and recipient count unchanged.
  This report is owner-assisted fixture evidence, not an independent machine
  receipt or native owner interaction.

Reports remain in repo-local `.cache/` without overwriting prior phase evidence.
Targeted vet and the final CI profiles qualify the committed source separately.
The Windows PG fixture lacks pgvector; full ordered migrations and actual Core
same-Input recovery are mandatory on the Linux PG17/pgvector CI service.

## Required committed-source checks

`browser-page-execution-tests.sh` now requires the exact page PG review matrix,
Core one-probe regressions, seven same-Input recovery outcomes, eight actual
Chromium cases and the actual owner UI/Core API/Broker/PG/resource profile.
Selected skips fail. `credential-vault-tests.sh` also requires both legacy and
managed same-Input recovery. Existing protected owner API/UI contract remains
unchanged; its resource hint identifies safe URL and opaque page/document IDs.

Known owner attestation only makes the existing waiting Input eligible. Core
must recheck its claim/lease, current Task/mandate and source guards before the
next model boundary. Unknown, expiry, changed/cancelled Task and source denial
must not infer or replay an action. Historical, unacknowledged and closed-page
outcomes remain restrictive. #164 stays open for protected RP/IdP consent,
cross-origin HTTPS/native model/delivery and external Google acceptance.
