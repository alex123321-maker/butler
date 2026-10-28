# Actual laba scripted acceptance

The full collector's57 stages, all31 required test names, new-profile lesson
read gate, corrected-loopback operator recovery/CAS and vet passed. No Gemini
request or real credential access occurred. See receipt.json and
docs/testing/issue115-laba-resumption.md. Raw hashes refer to private server and
local retained logs; raw proof, synthetic credential fixtures and private closed
history are excluded from this publication.

The first network allocation failed because Docker's automatic pools were
exhausted. An explicit non-overlapping internal subnet fixed the environment.
The first PostgreSQL container did not start because local log compression
requires more than one retained file; only the verified never-started created
container was removed, then bounded two-file logging was used. No existing
network, cache, deployment or database was removed.

The first operator invocation failed its mandatory dedicated-loopback check,
before the actual integration. Its original log and exit remain retained.
Operator-v2 uses a separate database/network namespace with the unchanged fixed
loopback endpoint and passes. The first gate chain is not relabelled successful;
the receipt distinguishes its failure and the separately corrected invocation.
The final source inventory check passed after all executions.

These results qualify the scripted server setup. A server-native host adapter,
current one-use recipe/preseal and original containment/accounting/auth-handback
guarantees are still required for a new model study. No release GO is claimed.
