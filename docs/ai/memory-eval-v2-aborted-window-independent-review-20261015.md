# Independent assessment of the aborted memory-eval v2 window

Decision: **SEALED_PRE_INFERENCE_FAILURE_NETWORK_COMPOSITION**. The first
one-use window failed during retrieval capture, before any auxiliary or native
inference. It provides no model comparison, quality result, complete cost or
issue acceptance. The spent entry must remain sealed even though inference
admissions were zero. A correction requires a different fresh root and a new
finite independent entry review; this assessment grants no second entry.

Reviewer: `codex-independent-memory-review-20261014`, separate task
`/root/memory_independent_review`. The reviewer used read-only SQL, Docker
metadata and named safe synthetic artifacts. No credential/import/signing
material was accessed, no Infer call was made, and no old source, container,
database or artifact was modified. Review date is 2026-10-15 Asia/Tomsk;
the original run timestamps below are UTC on 2026-10-14.

## Exact window and durable observations

Experiment root:
`/srv/storage/butler/agents/memory-eval-v2-native-20261014`.
Release: `1aa037fc91e41a7e3279596760bf76a3cfe5fee9`.
Source snapshot: `memory-eval-78-20261014/snapshots/20261014-235501-018`.
The reviewer independently rehashed its 2,555 files before entry:
`6a1e874165217c5982f7153a2be94aef5347a7effce2a5cd227ec1ab892e48da`.
Prepared collector binary:
`03cb70fdf511e254729c5bb454c38d4d9dcfa533e9b11dd5bdd058a786ae062c`.
Prepared audit CLI:
`422128822850f3f76599c57eb34b80f347984e89409da2f7ac5591d0d62b5e7f`.
Hardware hash:
`f99562d33da268f5dfddcb9475f2275a11939c55738acc2d85b96fb4d32e7b31`.
Golden/criteria remain the separately approved v2 digests
`7c74abe1234987883bd252126a8cade19b51ffb4e89d1b0df6bbc25447be413d`
and `9be3b4b51b5832bffc6349915cb61db4761e4b54ee95097b43677a28b9997e68`.

The reviewer read the isolated PostgreSQL state directly and cross-checked its
retained safe ledger. `fixture_memory_eval_window` has calls=0,
reserved_units=0, max_calls=280, closed=true. Its plan hash is
`2d78e95c25e8cea04c63ea305c7fdccb2c89647fe124a9c59dc9523a4d9c7664`.
There are zero model attempts, budget reservations, auxiliary IPC receipts,
generation-attempt receipts, guard receipts or model/task-result rows.

One owned index job exists:
`memory-index:memory:b19116fc74b63e9cae77cc8658d2cb87`, kind=index,
purpose=memory-eval-78, status=pending, attempts=1/max_attempts=4,
error_code=`local_model_unavailable`. It was created at
16:58:56.322730Z and finished its rejected attempt at16:58:56.351145Z.
Generation `ce763146e8e1319f351e29feac37d1bc` remains building with
verified_at=null, zero semantic entries and no active index. Three synthetic
sources, revisions, chunks and source/chunk bindings were retained. No
generation activation occurred. Job retryability is durable implementation
state, not permission to resume this sealed experiment.

The expense prefix is retained rather than erased:

| Stage | Durable outcome | Calls | Elapsed | Storage observation |
| --- | --- | --- | --- | --- |
| source write | measured/success | 1 storage operation | 22ms | 4,401 SQL row-column bytes |
| background failed prefix | unknown/failed | 0 inference calls | 64ms | null |

The write count is not an inference call. The storage number is the reviewed
logical row-size observation, not total physical PG/WAL/backup footprint.
Elapsed background time does not prove GPU inference, and unknown host/process
cost does not become zero complete expense.

## Diagnosis and its limits

The reviewer independently inspected both actual Docker metadata and the
retained pre-start execution metadata. `butler-lab-local-models-1` is running
on **only** `butler-lab_local-inference`, with alias `local-models`. Its image is
`sha256:1f8334477430a5d885a9defcbe79ac21367591dbe85f8f9ede4d4b50e45ff502`.
The experiment runner was attached to `memory-eval-v2-native-20261014` and
`butler-lab_backend`; neither network supplies that worker alias. Isolated
PostgreSQL is correctly on the experiment network with alias
`memory-eval-postgres`. Therefore the pinned endpoint `local-models:9090`
cannot reach the worker through this reviewed runner network composition.

The exact original script connects only the backend network after creating
the runner on the experiment network. Its execution validator checks the
network count=2, without checking the required local-inference network. That
operational prerequisite was missed by the earlier readiness review. Resource
limits and metadata-only file readability did not test worker reachability.

The source path is capture -> indexing.Enqueue -> ProcessOne -> semantic.Rebuild
-> Adapter.Embeddings -> Adapter.Verify -> backend.Describe. Adapter.Verify
requires the pinned embedding profile and maps an unavailable Describe/catalog
to `semantic.ErrUnavailable`; ProcessOne persists `local_model_unavailable`.
Verification precedes owning embedding admission and traced Infer. The observed
error, building generation, missing inference receipts and network mismatch
are consistent with this pre-inference rejection. The exact underlying gRPC
status/DNS error was not retained, so this assessment does not invent its text
or claim a measured authentication/profile response. No catalogue response or
embedding output was observed. Source code initializes the native provider
only after all retrieval captures and the pre-native freeze; neither occurred.

Runner final state is exited/exit1/OOMKilled=false, started
16:58:55.690063896Z and finished16:58:56.367731026Z. The private originals
contain `captures.json=null`, an empty frozen hash, observations=null and
`collection-status.json={"Complete":false,"Failure":"collection_failed_no_retry"}`.
There is no frozen-before-native/frozen/measurements/report/native observation
artifact in the independently listed original directory. A model-quality
failure or success cannot be derived from this aborted collection.

## Original artifact pins

The reviewer hashed each named safe file through UID10001, a network-none,
read-only-root helper, and an exact individual read-only mount:

| Original artifact | SHA256 |
| --- | --- |
| private/plan.json | `fc78dfd9a75dda3bfc81b6fb5995a7290dcb2e5f08c0ca5c4f0bc6ae691efad7` |
| private/captures.json | `38e0b9de817f645c4bec37c0d4a3e58baecccb040f5718dc069a72c7385a0bed` |
| private/partial-measurements.json | `578fddf93350fb27eb75a1224e38fbf7bf12452361895cc591dd6fa973742d09` |
| private/collection-status.json | `ce0743a56515b9a9f25ef70265e2791c1e7f78bed8f3a36191224a659e20e07b` |
| private/ledger.json | `6cb3a3ae66e3c198cf32e862a3428fe1fa194ace391865b9d4931b7c68289540` |
| metadata/execution.json | `73da622012162087d7651065b529ae2f30d0d07c3e704eb61f550b160293b9d5` |
| entered/exit-status | `4355a46b19d348dc2f57c046f8ef63d4538ebb936000f3c9ee954a27460dd865` |
| entered/final-state.json | `2ebc4b39c7b415fff1252fd8e6292b412e051741a1ec3ea1099775081d11e016` |

## Requirements for a separately reviewed future window

Keep this root, entry marker, failed-prefix expenses, pending job, generation,
containers and originals unchanged. Do not process the old pending job or
replace its artifacts. Pin a different new window ID in the runtime metadata
and actual plan, derived from its new root rather than the old fixed date.
Validate the complete exact network set, including the isolated experiment
network, backend and local-inference network, and reject absent/extra/duplicate
network identities. Retain the existing finite resource/admission limits and
guard/binary/source/golden validation.

Before any future inference, independently review the corrected source and
fresh compiled pins, then establish zero-inference worker reachability and
pinned Describe-profile compatibility through the existing trusted runtime
composition, without exposing credential bytes. Merely counting three
networks is insufficient. A future successful capture must retain original
auxiliary responses and correlate them with owning reservations and charged
expense. No model observations from this window exist for tuning. Golden,
criteria, native schema and prompts must remain unchanged unless separately
reviewed. Actual model quality, full task effects and complete costs remain
unapproved.
