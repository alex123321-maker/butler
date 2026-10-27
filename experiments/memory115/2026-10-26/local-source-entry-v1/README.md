# Source entry denied before auth or native admission; actual layout fixed

The first execute-entry identity native-20261026-v1 was denied by the source-idle
fence. The operator had assumed public lease/Input tables; actual native fixture
tables are retained in their own schemas, while only journal/settings are public.
The guard failed closed. No key or encrypted source value was read, no native
child/runner/output/execution-consumed marker was created, and the target native
database remains absent. The original outer entry marker and failed log remain
unchanged. This is a blocked pre-admission operator entry, not a model attempt or
native cohort that can be resumed or replayed.

The source fence now discovers bounded catalog table identities, requires both
lease and Input relations in every discovered schema, quotes names with pgx,
checks every live claim/lease and rejects other credential-database clients.
Failure, missing pairs or unavailable observations still deny entry. Default
zero-auth preparation now exercises this exact source fence against the real
local source, in addition to artifact/preseal and actual kernel resource checks.

Race tests and vet pass, including18 real isolated PostgreSQL cases and six
strict cipher cases. Added namespace cases cover idle success, live lease,
live claim and incomplete pairs. The final exact ELF passed actual preparation
and its empty cgroup was removed. No auth/model/domain claim occurred in these
checks. Original preseal, five collector binaries,33/full57 cohort and deadline
remain unchanged. A future entry must use a new identity and rerun every guard;
no marker or source state is reset and no native result is replaced.