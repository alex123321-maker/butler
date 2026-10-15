# Semantic scorer and collector validation

The historical72-row comparison was replayed against the independent58-answer
semantic review with exact original corpus/input/output/method hashes. The Go
scorer reproduced46 correct and12 partial;27 old strict rejections have correct
answer meaning. It retained6 failures and8 skips separately. See the
[review](memory-eval-v2-semantic-reassessment-20261015.md) and
[derived score](../../testdata/acceptance/results/ru-memory-v2-semantic-scored-20261015.json).
No model/auth/embedding call was made for this replay.

Current targeted Go/race and real PostgreSQL proof on ssh laba:

`/srv/storage/butler/agents/memory-eval-semantic-20261015/reports/memory-semantic-verify-20261015T094106Z-2068364`

Source snapshot `20261015-163929-581`: **41 test/subtest passes,0fail,0skip**;
vet and shell syntax checks pass, diagnostics empty, zero real inference.
The tests include independent-review original/witness tampering, separate safe
refusal and failed denominators, exclusive CLI replay, no rubric leakage,
free-answer native schema construction without authentication, reopened current
PostgreSQL retrieval and original retention, runtime/source/capability drift,
early embedding/native-construction failure retention, SQL-unavailable file
retention, and the full38-input schedule with exactly one retained simulated
provider timeout followed by the remaining distinct inputs. Fake responses are
code evidence only; they do not enter the model-quality report.

The prior real-PG/race run at092228Z passed47 tests/subtests and intentionally
skipped the explicit opt-in native entry once. It additionally covered the
unchanged legacy V2/collector regression. Subsequent intermediate failures
(unbounded optional citation schema, lost test-only search_path on pool reconnect,
and a test assuming a HybridReader fallback must raise an error) remain in their
own proof directories. They were fixed before prospective native execution and
were never model failures.

Pinned native helper SHA256:
`79f6256b6d11fd229540d6461d21d5ef845362bd97aba980940d76d56d979d80`.
Current test helper SHA256:
`c0b7c1679bf6a28d0c9a8c9f09ce11f871271f28f37fe856649e4d148321ab28`.
After this proof the launcher changed only executable permission for the offline
CLI alongside the collector; its shell syntax and actual prepared runtime receive
separate independent checks before native execution.

Independent calibration judged ten previously fixed examples without access to
the author's key. All40 grounding/completeness/context/corpus dimensions matched;
the [key](../../testdata/acceptance/reviews/ru-memory-semantic-calibration-key-v1.json)
was published after review. This small calibration establishes agreement on these
examples, not a statistical guarantee of reviewer accuracy.
