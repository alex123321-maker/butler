# Independent review of the first native experience window

**NO_GO.** The prospectively shortened33-stage window stopped at original01.
I reviewed both complete visible model requests, both proposed calls, the actual
inspect result, owning task/input/effect rows, all57 immutable outcome rows and
the reported cost/handback projection. No lesson, skill or A/B/C control ran;
there is no evidence of experience benefit or native experience qualification.

The frozen source was `813dda3efad1539eed0a48fe96de8a6ecdf2b50e` and preseal
`e9b0db87ce874ff89ad5f6e90049715daf60160130e199546be8556fcbc0043b`.
The unchanged [originals](fixtures/issue90/originals-v1.json),
[controls](fixtures/issue90/controls-v1.json) and
[protocol](fixtures/issue90/protocol-v1.md) were read before the output.
The shortened admission retains the original57 denominator:33 stages admitted
for this one-pass smoke,24 second-repetition controls intentionally unexecuted.

## Evidence and exact failure

The closed-window safe projection is
`/srv/storage/butler/agents/issue90-experience-20261016/reports/native-experience-preseal-v1/safe-review-v1.json`,
SHA256 `d9d70387b1aa64d7837bf2908ef78e9874a645a03b9727712016df439cb62fd4`.
Its read-only producer is `project-native-v77-safe.py` in that agent base,
SHA256 `1d80986b323f4314c5741ab886f343611e8c0293157c454e9c98d96ba5b65c45`.
Both downloaded files were independently hashed. The producer verifies the
closed leaf references/private inventory and projects declared visible text,
tool events and owning rows; it excludes reasoning, sessions, raw errors and
auth material. It explicitly refuses a later populated cohort. The referenced
closed leaf is `8d9329e13da9e51e620c5fa66d6625ae11c938006b18369d2e09f068f1517b3f`,
private manifest `b37867791e9843763827cb80cf5e59f02c8f48ac0ab680553d18f83d58efd6ab`.

1. The first response proposes `extension.project.inspect` at
   `http://external.test/project/alpha`. The actual result returns status200,
   the current README bytes, current revision, `accepted_ready_value=true`
   and a failed current readiness check. This is an observation, not success
   of the requested artifact task.
2. The continuation proposes `extension.project.prepare-patch` at the same
   `/project/alpha` URL. Its exact before bytes and revision match that read;
   its after text changes only `ready=false` to `ready=true` and preserves the
   canary. The recipient accepts preparation at `/project/alpha/patches`, so
   the proposed route is wrong. No check or final response follows.

The actual advertised schemas expose `url:string`; the package descriptions
describe inspect/prepare/check but give no operation route. The inspect result
also supplies no preparation/check endpoint. The concrete wrong proposal and
missing discoverable route information are both visible in the actual packet.
This supports an interface-discoverability diagnosis, not a provider refusal or
a claim that the model could never infer a valid route. Source pointers at the
frozen commit are `internal/extensions/projectfixture/manifest.go:17` and
`apps/orchestrator/internal/app/memory_experience_recipient_test.go:251`.
Any generic fixture-description repair is a new source delta; it cannot repair
this original outcome retrospectively.

The owning input is blocked at `tool_result_recorded`, `error_code=effect_unknown`,
attempts1/max1. The preparation tool and effect retain UNKNOWN; the effect
evidence has typed validation error and dispatch outcome unknown. The task has
no completion result and `run_final_results` is empty. The run row still says
`tool_running` without a finished timestamp, so this review does not invent a
terminal run-state transition. The closed collector failed its required source
success gate. No second preparation, false success or automatic retry is visible.

The safe packet has no published original witness or independent recipient
HTTP/status audit after that gate failed. Therefore it does **not** prove an
actual recipient404, physical artifact absence or unchanged recipient bytes.
The durable UNKNOWN remains UNKNOWN; a wrong route alone is not typed ABSENT.

## Population and cost

All57 distinct outcome IDs and ordered sequences are retained. Original01 is
failed (`required_source_success_not_verified`);32 remaining admitted stages are
`not_executed/window_closed_before_execution`;24 second-repetition controls are
`not_executed/predeclared_short_smoke_omission`. There are no accepted original
witnesses, notes, skills or allocated control database exports. Notes/skill and
all24 planned first-repetition A/B/C comparisons are unmeasured. Deletion,
scope isolation, environment drift and experience-mediated UNKNOWN behavior
were consequently not exercised natively by this window.

The two provider entries have distinct operation/reservation identities in the
same owning run. Both report known full totals:4326 and5587, total9913.
Input8519 plus output243 is8762 and is not substituted for provider full total.
Captured provider durations sum to15040ms. Both new product attempts have
reported usage; there is no new NULL. Inclusive history carries213 native/product
attempts once,212 known and one old NULL, known full-token lower bound1605455.
The old NULL is not removed by the two known entries. The conservative213-turn
carry also retains tokens230326272, external calls144, active milliseconds81065000,
egress600834048 bytes and provider input1786773504 bytes. These are quoted
resource charges, not measured provider token usage or a monetary invoice.
`cost_micros=0` does not establish free service. No A/B/C quality/cost ratio,
amortized saving or successful preparation cost can be inferred from this stop.

The safe handback reports collector status1 and handback status0, exact opaque
main CAS, active-work check, resolved/nonexpired/safe auth, paid discovery200
and a fresh opaque main snapshot. I checked that projection, without reading
raw credentials or querying the main deployment. It does not qualify task
quality, provider availability beyond these two entries or the full experience
cohort. This review adds no native call, retry, qualification flip or code change.

The machine-readable [independent review](fixtures/issue90/native-v1-independent-mixed91-review.json)
keeps every original outcome and the same limits. Experience qualification must
remain false; native92's experience prerequisite must stay closed. Existing
controlled mechanics and closed-state bootstrap checks remain separate evidence.
