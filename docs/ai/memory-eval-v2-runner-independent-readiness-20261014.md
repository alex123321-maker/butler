# Independent bounded memory-eval v2 runner readiness, 2026-10-14

**Decision: PASS_BOUNDED_RUNNER_ENTRY_ONLY.** The independently reviewed
implementation, operational correction and zero-inference proof support one
new, finite public synthetic memory-eval comparison under the exact scope
below. This is readiness to collect original observations. It does not certify
actual inference, model quality, complete costs, external task effects,
promotion, merge, deployment or issue78 closure. Original post-run trace/ledger
correlation remains mandatory. Actual native and auxiliary model calls are zero
at this review.

Reviewer: `codex-independent-memory-review-20261014`, task
`/root/memory_independent_review`, separately appointed by the human user. The
reviewer authored neither dataset nor candidate implementation, made zero native
or auxiliary inference calls, and accessed no private credential or signing
material. Reviewer writes are limited to independent review declarations.
Independence is between roles on a shared host. The initial runner NO_GO, v1
NO_GO, rejected data proposals and exact approved golden/criteria declarations
remain preserved.

## Exact implementation and independently read proof

Reviewed clean implementation commit:
`bffd171388300d84077ee964fde59809d2cf3ea3`. Its only differences from the
final core-tested commit `d9886e72ffacbd4a0a6626761135f434596e3ab0` are the
operational script and its README. The Go implementation, schema and golden
data are unchanged; final core proof below applies to their exact bytes.

Exact tested snapshot:
`/srv/storage/butler/agents/memory-eval-78-20261014/snapshots/20261014-234329-928`.
The reviewer independently rehashed all 2,554 snapshot files using the
`TreeHash` path/NUL/file-SHA/newline algorithm and exclusion rules, reproducing
source SHA256
`eede8d73cca567129ebaefda46860396efa690a64bd1ad8a941409fc8a5943b3`.
The reviewed source was clean before this declaration was added. Source copies
with this declaration may have a new whole-tree digest; executable code,
fixtures, schema, criteria and scripts must retain the approved bytes below.
Final source digest, compiled collector/CLI, exact images and hardware must be
pinned independently during preparation, before credentials/native dispatch.
This declaration does not fabricate an as-yet-unbuilt binary hash.

| Reviewed object | SHA256 |
| --- | --- |
| v2 raw dataset | `5571d474ec54a8a29765e055cc0475e73653d679d5f250fa074dcea1d9d6b1a5` |
| v2 semantic suite | `7c74abe1234987883bd252126a8cade19b51ffb4e89d1b0df6bbc25447be413d` |
| v2 criteria | `9be3b4b51b5832bffc6349915cb61db4761e4b54ee95097b43677a28b9997e68` |
| `memory_eval_native_test.go` | `5bbc2fb933b90ce3c6aeb81ba5da6467642bf95bb2e0219c5db84ec982f4dc5b` |
| `memory_eval_collector_test.go` | `1c34f39db9a0da51a2b0badbfa90a6aaa12c83a748766a23f9ec699adb547a2e` |
| `memory_eval_integration_test.go` | `9011c0144785751321366e2ebb85613e1fb26d1f9760ee93f58657241be63aa0` |
| `memory_eval_event_test.go` | `590a950f001b6ea5cf9c24c21c2b4c60e9fe36a0cbfc797364e6e3efcd7bd4f1` |
| `memory_eval_export_test.go` | `ba3492262a373a22f7d7b25768fdc444dbe9d077a9fdaafd37c0158b1944b2f1` |
| `memory_runner_audit.go` | `ac6b5c0ab8b4bf3c9825822c836fc94b69182d97e16bc5b1a3f3d2a3b149516c` |
| `memory_runner_provenance.go` | `9dd9b4c1a26c002f42ea450e1599c5deaad4d0999101fa14eede04afb6c6072a` |
| `memory_runner_aux.go` | `472f64b4073c1b36f8283b72c531a181990398f445373b652fba6fa57a22f241` |
| `memory_runtime_pins.go` | `f980f7e102b9a031dd60c42c6b9294d057140196cdf65ce219366fcf26284be8` |
| `memory_v2.go` including exact native producer schema | `0b10616370ea1e132c0fab74af381fe6fa4565f3c3e8a1d856bba3baf9b69f97` |
| `memory_contract.go` | `2b5d5a217cf0286d7468585425b445c47c5e944786990297bacc4430ddd7ef3d` |
| `memory_score.go` | `0a4cb3988f98074b53af00274af1220fae20d24f52d6c072d78ad8f7998c249b` |
| `cmd/memory-eval/main.go` | `1d5af9634d6e5f6bad884e2233edb92a2f8a21ec2780d0b12ce87d2149c0caa0` |
| `inference_authority.go` | `848a459e169a8a9b498587810d3c79b85427d15d0676870d06483384cd8413a4` |
| `antigravity/protocol.go` | `6f59dc68ca1d677ac482fe921e7ecb9a9ae5de89f6cb1bf3949d97e868f046ec` |
| `antigravity/authority_test.go` | `54f4d6a840494e50fe015ae76abf646652e1e02690f423b36016c5f0ad297c99` |
| `memory-eval-model-lab.sh` | `4ad1c1e19708fadd1470fd16f446328a2693ab48eb85f7d688bbcb37c3207040` |
| `memory-eval-verify-lab.sh` | `9737e3971f7c175fc607f0552cf63a66410a9928d556fba51c9d3cc8549dc473` |
| `memory-eval-review/main.go` | `4cdf2b0a810b2241a2c33d101ab9b4244a30a4af5e3104cc650645299fb760e0` |

Paths in the collector group are under
`apps/orchestrator/internal/app`; acceptance helpers are under
`internal/acceptance`; transport paths are under `internal/transport`; scripts
are under `scripts/dev`. These file hashes were read independently. The reviewer
also compared the final core amendment to its predecessor: only provenance
binding and its four negative PG subcases changed, so broad regression evidence
was not silently substituted for the final targeted proof. The subsequent
operational amendment is separately reviewed below.

Final targeted PG/race proof was read directly on laba:
`reports/memory78-fixture-20261014T164342Z-2941833`. Its `narrow.jsonl` SHA256 is
`d16259a8482fe79a201c1c2ef649bbe5158899648709e9b673fac44552f629c2`:
16 named passes, zero failures/skips; both packages completed and runner/
diagnostic files were empty after successful execution. The implementing root
reports vet exit0. The reviewer inspected the exact zero-inference script and
its test commands; no private auth or live model flag is mounted there.

The post-run read-only source manifest SHA256 is
`79a5661e1accb5988db6c9907d7b022c0d9a29a2d412df0caef9fe661c370d3b`.
It explicitly records hashing the stated immutable snapshot after tests. The
reviewer reproduced that hash independently. It is not claimed as pre-run
instrumentation or a retrospectively observed native event.

Earlier broad regression proof at
`reports/memory78-fixture-20261014T155844Z-2805829` was independently read:
`regression.jsonl` SHA256
`9ad078cc5670b8504dfcf0b658d747968b1d248857442818d40bcd7e6e5a9347`,
646 named passes and zero failures. Three opt-in live tests skipped:
`TestLiveSharedObservationContexts`, `TestOpenAIIntegrationStartRun`,
`TestOpenAIIntegrationWebSocketSession`. Those skips do not establish live
authentication, provider acceptance or a model result. Intermediate targeted
proof at `memory78-fixture-20261014T161839Z-2865795` had12 passes/0 failures/
0 skips; its hash is
`bd0ed3a2edaeb2f7015e43056a48ed07c31f6fdec7accd837fdd4f7238610878`.
The first additional provenance test found a nested JSON serialization-order
mismatch; the final implementation uses the compacted original bundle bytes,
and the successful final16-case proof supersedes that failure without erasing it.

The reviewer initially withheld the unsigned readiness draft when the author
found an operational prerequisite failure: SSH UID1000 has no sudo and cannot
directly chown runtime outputs or stat the private import ancestor. The
withheld draft SHA was
`d187987373ac593cd0327e17b3f4d8c61999c9c1171c17028356be270af17fb8`;
it granted no entry. The corrected, pinned script now uses network-none,
read-only-root Docker helpers. Initial CHOWN/FOWNER/DAC_OVERRIDE applies only
to four named fresh experiment-output mounts. Metadata directory remains
UID1000/mode711; private runtime files remain UID10001/mode600. Later
read/hash functions accept only enumerated output leaves mounted individually
read-only; permission updates accept only pins/execution metadata. No helper
has a credential mount except the separate UID10001 metadata-only preflight,
whose only command is `test -f`/`test -r` on the two exact read-only private
file mounts. It runs before the one-use marker, performs no content read or
authentication, and `--mount` rejects missing sources rather than creating them.

The reviewer read the retained operational proof directly:
`reports/permission-preflight-20261014T165147Z/proof.json`, SHA256
`ce09cb5b3d8f94f3cea2dd0342900e4fa71ce83f6a278589bf3748bd83c2c526`.
It records PASS_METADATA_ONLY_PREFLIGHT, operator UID1000, runtime UID10001,
the exact script SHA above, credential_contents_read=false, inference_calls=0
and image
`sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61`.
The author reports fresh known-output permission/read/hash checks passed.
The reviewer independently checked the script hash and ran `bash -n` (exit0,
empty diagnostics) on exact snapshot
`/srv/storage/butler/agents/memory-eval-78-20261014/snapshots/20261014-235138-298`.
This operational proof establishes neither credential contents nor actual
provider authentication. It is not substituted for post-run native proof.

## Resolution of the first-pass findings

The reviewer reread the stable final code and relevant catalog/budget/transport
composition. The initial16 findings are resolved for this bounded experiment:

- Source/task/lease checks run before private authentication and exact HTTP
  dispatch, with a100ms cancellation watch and bounded current checks. Immutable
  SQL receipts bind the admitted operation, exact current source refs, protected
  task/constraint revision, lease and dispatch/final time. Empty-source no_ltm
  still receives the catalog's owning authority checks. Ordinary unhooked
  cancellation is preserved; stale/check-timeout diagnostics expose fixed text.
- Model usage/attempt/expense persists before final/event/source-retention errors;
  admitted unknown prefixes remain charged. Auxiliary IPCs bind one exact owning
  reservation and original request/response. Failed source/index/read prefixes
  remain measured or unknown rather than becoming unused.
- Raw UTF-8 output has an explicit retention encoding. A genuine JSON string,
  malformed prose and valid object remain distinct original bytes. Original-byte
  bounds, STOP final, observed version, complete consistent usage, event IDs,
  run/provider identity and final-before-terminal ordering are checked.
- Original import, pre-native freeze, exact model projection/instructions/schema,
  current bundle digest/prompt/ordered IDs, source body/hash/alias/owner and
  auxiliary query/chunk provenance are correlated. Every native/auxiliary
  admission/reservation/result is used uniquely. Guard operation/source refs and
  capture digest/source IDs now have explicit negative corruption tests.
- Input reading, shared retrieval source/index/read preparation and artifact
  retention have explicit attribution. Unmeasured kernel/session/guard/ledger
  overhead remains unknown. Pipeline time adds one-time retrieval setup only to
  retrieval; per-sample latency has its separately documented60s gate. Stage
  overlap is not summed into fictitious elapsed or billable time.
- Native reservation bounds serialized JSON/schema/framing and output, using the
  same native HTTP cap; actual observed counts above admission fail closed.
  Owner operations keep experiment cancellation/deadline. Window closure and
  evidence failures prevent complete-collection claims.
- One-use entry, fresh isolated PG, exact Docker image IDs, real command/UID/
  CPU/RAM/PID/read-only-mount metadata,3h context, hard stop on attached-client
  timeout,4,096 events per entry and128MiB reports are enforced. Existing main
  deployment/database/credential paths are preserved. Containers/PG/artifacts
  remain recoverable after failure; a window is never reopened.

The shared actual admission policy and owner-authority resolver are used by the
fake PG tests as well as the actual entry. The complete fake matrix correlates
64 observations and64 unknown broader-task results, including a genuine JSON
string. Task amendment, lease expiry, source deletion and retention failure each
deny acceptance while keeping charged expense. The new corruption subcases
reject guard-operation, guard-source, capture-digest and capture-source forgery at
their specific bindings. All64 admitted native schemas pass provider construction
without authentication. The source's OpenAPI subset uses decimal-string
`maxItems=8`; confidence range remains application-enforced. Its exact bytes
are pinned by source/binary, while golden/criteria semantics remain unchanged.

## Approved one-window scope and remaining conditions

Scope is one fresh public synthetic v2 comparison of24 cases, three arms,
fixed case/arm order and pre-native captures, requested
`gemini-3.8-flash-high` with required observed `gemini-3.8-flash-n`, no fallback,
repair, resampling or retry. Planned secret/oversized skips are retained.
The script metadata allows at most66 native samples; the current frozen fixture
has64 eligible ones. Shared new inference admissions are at most280, with
4,000,000 conservative units,3 hours,2 CPUs,1536MiB,128PIDs,4,096 safe events
per entry and128MiB reports. Each actual serialized native input has a finite
cap no larger than32KiB; generated allowance is32,768 tokens, final bytes8KiB.
The numerical90%/critical0/false-positive0/60s/cost criteria are unchanged.

Preparation must pin the final source tree, compiled runner/CLI/helper, exact
images, hardware/config, independent golden public key/approval and permitted
private import references. The new review document may be included in that
source tree without changing the executable bytes approved above. Actual
container metadata and source/binary/data/criteria checks must pass before
private provider state. Unexpected code/config/fixture/schema changes require
review before inference. A failed or spent native window stays sealed; any
later experiment requires its own independent review and finite admission.

The configuration measures current owned user-scoped semantic chunks with
Gemma2/structured-go-v1/HybridReader/BuildBundle; profile, episodic, working and
summary stores are unconfigured, and extraction/summary/consolidation/reranking
are off. This scope does not establish full production memory coverage. Data is
public, synthetic, small and nonblind. The finite lowercase/whitespace rubric
can reject unseen equivalent phrasing. Output proposals do not prove actual
file, repair, catalogue or delivery effects; broader task success remains null.

Subscription invoice, local tokenizer usage, physical PG/WAL/backup footprint
and host energy remain unknown. Zero admission quote/local no-billing basis is
not measured complete expense. The final report must retain known subtotals,
nulls, failed prefixes, native versus pipeline latency and explicit baseline
limitations. These unknowns deny complete-cost and promotion claims even if QA
thresholds pass. A negative model result can support harness acceptance.

After the one-use run, separately read the original private directory and exact
compiled-runtime pins, correlate every original native/auxiliary event, output,
source and charged ledger record, and preserve negative outcomes. No real model
observation exists in this review. Native compatibility, actual quality, broad
effects and issue closure cannot be inferred from the16/646 code-test counts or
this readiness declaration. The reviewer grants no production execution,
deployment or promotion authority.
