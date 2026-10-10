# Structured extraction of candidate memory

The local service computes tensors. The Go caller owns source/job admission,
normalization and atomic candidate publication; the catalog owns current access
and separate owner decisions. Extraction cannot change orchestration, accepted
profile memory or action authority.

The caller pins `fastino/gliner2.5-multi-v1` revision
`cf5593a5d45e3bbf204b9df621b13b1c0cf25ee3`, profile
`gliner2.5-multi-cpu-v1`, schema `butler-memory-v1`, normalizer
`butler-russian-time-cues-v1` and the existing
`torch2.10.0+cu128/transformers5.19.0/gliner2.1.0` runtime. The
[upstream model](https://huggingface.co/fastino/gliner2.5-multi-v1) supports
entities, relations, records and CPU execution. Scores are uncalibrated
extraction signals, never truth probabilities.

The reviewed schema covers project/component/task/resource/action/deadline,
prohibition, exception, condition and temporal qualifiers. Relations cover
action-resource/deadline/exception and project-component. A constraint needs a
literal action anchor; no generated field fills a gap. Optional/ambiguous fields
remain absent/unresolved. Every field is checked against original Unicode source
positions; scores must be finite and bounded. Python remains only the reviewed
upstream tensor runtime, not an agent or permission engine.

Each actual call requires an independent live `extract` job and complete source
manifest. Ancestor revisions, owner/task/goal/mandate, sensitivity and finite
shared budgets stay current across waits. Model labels, caller IDs and prices
cannot grant locality. The composed internal target proves locality separately
from explicit `local_no_billing` pricing; backend credentials never enter input.
Complete correlated output is validated and current job/source access is checked
again before atomic publication. Empty output completes with an empty array.
Partial/malformed windows publish nothing; exact retries preserve IDs/decisions.

Records retain quote/offsets, inherited data-only origin, full source hash,
author/locator and distinct event, ingestion and extraction times. A pinned
calendar grammar uses only known event time and trusted timezone. Missing
context remains unresolved. Literal Russian negation/exception/conditional/
historical cues are fragment-scoped, separate from model field scores. Neither
“можно” nor “теперь” becomes current owner authorization. Bounded quotes can be
explicitly clipped without changing source spans.

## Acceptance and measured limitations

The public `russian-extraction.json` has 6 development and 18 held-out cases with
threshold 0.5 selected before final testing. Original and corrected reports are
retained. Exact model fields are compared with the existing generic entity
adapter and a deterministic rule baseline. Literal protection evidence is
checked separately and excluded from precision/recall. The legacy cloud caller
is disabled; no authorized live cloud comparison is claimed.

Held-out structured precision/recall is 0.484/0.564, generic 0.319/0.273 and
rules 0.975/0.709. The model misses prohibition fields in five cases across both
splits. The predeclared suggestion gate (precision>=0.90, recall>=0.80, no runtime
errors or omitted prohibitions) fails. Fixing empty-array persistence repairs a
contract; it does not change threshold or improve predictions. Passing the test
process means the experiment and authority checks completed, not quality approval.

The explicit message “До конца месяца не обновляй рабочий сервер, тестовый можно”
must preserve a `constraint_fragment` candidate with model resource/action/deadline spans, the complete
original restriction and separate literal “не”/“можно” evidence. These cues are
not fabricated model fields or a test-server grant. Missing/ambiguous model
fields remain visible. Owner acceptance and mandate creation are separate.

The fallback groups existing literal entity annotations by their source quote;
it adds no model fields or predicted relationship. It explicitly leaves action
polarity and exception scope unresolved. Model spans and literal rules retain
different `evidence_method` values. This protects the source restriction without
pretending GLiNER recognized its semantic prohibition.

Production scheduling remains disabled: this is a measured no-go for automatic
suggestions. It does not promise reliable semantic negation or complete memory.
New versions/schema changes require fresh pinned evaluation, recorded development
changes and rerun quality/privacy/replay/current-source/job fault tests. A local
failure cannot authorize stronger cloud fallback.
