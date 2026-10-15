# Isolated native memory read acceptance

This lab runs the actual Orchestrator and Tool Broker with optional memory reads,
the existing native High transport and the server's isolated proxy route. It uses
fresh synthetic PG/Redis, a dedicated owner and no Telegram delivery. The foreground
task must choose `memory.search`, open its exact returned source and answer with
the marker found beyond the compact search snippet. The prompt contains neither
the source ID nor the answer. Ordinary durable model attempts, tool effects,
shared budget reservations and source pins are the evidence.

Stage a reviewed snapshot under
`/srv/storage/butler/agents/issue86-native-20261016/current`, then run
`scripts/dev/memory-read-native-lab.sh ROOT prepare` on `ssh laba`. This is offline
and never imports provider credentials. After serializing the normal auth/refresh
slot with other native tests, execute with a fresh evidence label and the newest
private encrypted row file plus its matching private encryption env reference:
`scripts/dev/memory-read-native-lab.sh ROOT execute LABEL VALUE_REFERENCE ENV_REFERENCE`.
Only opaque paths appear in arguments; private process pipes import the encrypted
row. No account credential/token/key enters model inputs or evidence.

Each imported instance keeps its own refreshed state. The exit trap stops its
Orchestrator and writes `private/auth-return.value`; pass that newest ciphertext
and matching env to the next serialized consumer rather than recopying stale
main/snapshot auth. The script refuses existing labels, imported stores and spent
attempts. Product admission keeps its normal finite 64-turn task defaults and
five-minute native call bound; the one task has a twenty-minute wall-clock test
deadline. No provider fallback or budget refund is introduced.

All native internet uses the existing proxy configuration. Build/migration runs
use only cached modules/images on the private network. The main Compose stack is
untouched. JSONL-less full product evidence is retained in PG and `ledger.json`,
including every attempt's observed usage or honest SQL NULL. A failed window is
retained and cannot be relabeled successful. Exact code/evidence acceptance is
separate from general memory/model quality and original task verification.
