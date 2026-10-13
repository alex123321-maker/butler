# Read-only V4 original-final audit

Run the frozen executable network-none/read-only as UID10001 with capabilities
dropped. Mount exact private report at `/report.json`, separate read-only SQL
attempt/diagnostic exports at `/pg.json` and `/diagnostics.json`, public review
leaves at `/metadata/public-key.hex` and `/metadata/approval.json`, and frozen
source read-only at `/src`. Mount only the explicitly retained `final-NNN.raw`
leaves at `/raw/final-NNN.raw`; never mount the whole runtime, import, encryption
key or signing directory. Supply the independently read exact hashes and prepared
commit/source/hardware identities through flags shown by `-help`.

The audit reuses frozen programmatic scoring, verifies every SQL measurement,
retained original byte/hash/mode/category and completed lifecycle, and checks
synthesis inputs against the actual children. Its stdout is a content-free
projection with costs and metrics. Correct witness counts can be less than the
inventory; SQL/raw equality does not imply quality PASS or release acceptance.
No model/source text, upstream provider error, credential or hidden reasoning is
printed. The audit performs no inference, authentication, network or mutation.
