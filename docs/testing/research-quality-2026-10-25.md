# Research quality and checked follow-ups, 2026-10-25

Scope: general current-fact research, concise owner replies, optional monitoring,
and recognition of reusable procedures. No title, publisher, release date or
anime-specific route was added. Earlier delivery/search recovery remains in
[the previous acceptance report](research-recovery-2026-10-24.md).

## Implemented behavior

* Current-run read receipts support exact short evidence quotes and direct
  citations. Snippets, previous runs, failed requests, inline extraction input
  and frozen local browser preparation cannot supply current page evidence.
  Primary/secondary attribution is a model judgement, not an authority certificate.
* Reader revision 3 bounds model text to 12000 Unicode code points by default,
  with explicit continuation offsets and available-text hashes. Batch reads share
  24000 text characters. HTML and discovery links have independent context bounds;
  existing MIME/UTF-8/DNS/body/wire limits remain.
* Ordinary researched messages default to four nonempty lines and 1800 characters.
  Up to four durable correlated draft corrections precede dispatch. Detailed
  requests retain their scope; honest unavailable-evidence responses remain valid.
  Ordinary form/HTTP actions do not acquire unrelated citation requirements.
* One useful optional question follows an already useful answer. A question
  creates no schedule. Consent leads to an ordinary bounded schedule proposal,
  retaining policy-required exact approval, timezone, expiry and occurrence limits.
* `monitor=true` agent tasks record confirmed/postponed/pending/unknown observations.
  Evidence and an actual check are required; planned time passing is not proof.
  Pending/repeated observations stay quiet, including across restarts. Binding,
  source, job revision and cancellation checks remain current. Known provider
  outages can allow a later slot without replaying an old run or resetting usage.
* Repeated completed exchanges supply a compact value-free workflow index: actual
  publication references and completed operation names, with duplicate requests
  collapsed and an explicit published_unverified label. This supports comparing
  current steps with past work and proposing a parameterized procedure.
  This change does not add an automatic skill builder or a new draft-saving API.
  Existing independent task evidence, testing, owner review and activation still
  govern actual skills. A taskless published answer is not sufficient evidence.

See [the architecture contract](../architecture/research-followups.md).

## Component and boundary evidence

Windows targeted Go checks passed through `scripts/dev/test-go.ps1`, using
repo-local caches. Linux checks used Go 1.27.2, race detection, vet and isolated
pgvector PostgreSQL 17 with all migrations through 100. PostgreSQL and executable
Go temporary files used bounded tmpfs, avoiding the nearly full host filesystem.

Coverage includes unrelated release/event/timetable fixtures; stale, unread,
secondary and invented citations; URL-only extraction and HTTP/browser readers;
concise drafts; correlated non-dispatch recovery; actual ordinary schedule
Task/Input admission into the explicit communication profile; source/binding
revocation; missing observations; unavailable evidence; deduplication after
restart; provider outages; and quiet lifecycle/error delivery.

The actual Broker/Core dispatch boundary regression suite passed with current
claim, grant, destination, expiry and source guards. The full authorization suite
also passed after updating its pre-existing hand-written credential fixture to
include the current Input budget columns; production credential logic was unchanged.

Reader tests independently retain the 2 MiB upstream and 3 MiB serialized-wire
guards while checking smaller model windows and source-version continuation.
The frozen optional local `read_webpage` experiment correctly refuses the new
reader contract; no local model was retrained or automatically requalified.

Raw JSONL reports are retained under the task-owned directory
`/srv/storage/butler/agents/research-recovery-20261024/reports/`:
`research-quality-unit.jsonl`, `checked-monitors-final.jsonl`,
`research-communication-boundaries.jsonl`, `research-quiet-notices.jsonl`,
`research-text-windows.jsonl`, `research-concise-preflight.jsonl`, and vet logs.
The optional runtime-harness entry is a subprocess fixture, excluded from the
ordinary runtime unit invocation; native acceptance uses the deployed runtime.

## Native diagnostic before bounding page text

Event `research-quality-go-20261025`, Input
`input-6fac0b5685f1415d4f03745ebf820ef3`, run
`run-1792865266793877140-037aa421f3c0573c` completed without approvals. It read
`https://go.dev/dl/` and `https://go.dev/doc/devel/release`, supplied actual quoted
primary evidence, published through the ordinary communication tool, and offered
regular update checking. Web was result_ready and Telegram acknowledged message
151. No schedule was created.

The result still included unrequested supported-version background. Two archive
pages also expanded cumulative observed model input to 1664871 tokens over six
attempts (680 observed output tokens, zero unknown usage pairs). These are
transport usage measurements, not invoice/cached-token accounting. That history
and spending remain intact. This diagnostic motivated bounded reader windows and
the stricter concise default; it is not counted as final quality acceptance.

## Bounded-reader native results

All requests entered the ordinary explicit owner Web/Input/Core/provider/Broker/
WebFetch/publication path. No expected answer was injected into Butler. Each
successful result had three nonempty lines, actual current-run primary-page
quotes, zero approvals, a protected owner GET returning 200, and Web result_ready
plus a real Telegram sent acknowledgement. Source choice was assessed from the
actual read URLs and evidence, separately from mechanical quote validation.

| Event suffix after `research-quality-` | Source supporting the result | Attempts | Observed input/output tokens | Telegram ACK |
| --- | --- | ---: | ---: | ---: |
| `go-bounded-20261025` | `https://go.dev/doc/devel/release` | 5 | 40609 / 393 | 153 |
| `python-20261025` | `https://www.python.org/downloads/release/python-3151/` | 7 | 60577 / 485 | 157 |
| `event-20261025` | `https://fosdem.org/2027/` | 6 | 34981 / 436 | 159 |
| `rust-retry-20261025` | `https://blog.rust-lang.org/2026/10/15/Rust-1.99.1/` | 7 | 58081 / 548 | 165 |

These four successful runs had zero unknown usage pairs. The Go diagnostic
decreased from 1664871 to 40609 observed cumulative input tokens after bounding
page text. This is a comparison of diagnostic runs, not a controlled benchmark:
their histories, page count and model trajectories differed. It is not a billing
or cached-token claim.

Go/Python/FOSDEM offered optional monitoring without creating a job. Python's
initial result repeated the per-subject monitoring offer rather than recognizing
reuse. The bounded workflow index and stronger generic comparison instruction
were then added. On a subsequent ordinary Rust request, the model offered to
save the shared version/date-checking procedure with “название языка или
технологии” as its input parameter. The request did not mention skills or ask
for that suggestion. No schedule or skill mutation tool was called without consent.

The independent event source checks that these changes are not confined to
software releases. Release/event/timetable component fixtures cover additional
source failure and postponement cases. No anime-specific rule was added.

### Retained failed probe and acceptance limits

The first Rust probe, `research-quality-rust-20261025`, Input
`input-a3e1753c8c258856b1372bab4b80c4b2`, run
`run-1792866606408686796-990bf928421b2e2c`, stopped before any tool execution.
The existing authority observer reported phase `lease`, classification `deadline`,
elapsed 751 ms. Its underlying cause was not established in this task. The guard
was not weakened. There was no publication or external action effect. The stopped
Input was closed through the protected owner CloseFailed API, preserving its error,
reservation and one unknown model-usage pair. A separately identified retry then
completed on the same deployed code. This was an operator-driven diagnostic
retry, not evidence of automatic recovery from authority-observation timeouts.

The probes establish bounded successful examples, not universal search accuracy
or general reliability. They do not establish that an active skill was built,
that a real future release was monitored after consent, or that a human opened
the Telegram client. Actual future-job creation/execution was exercised in the
isolated tests below; semantic model planning after future consent remains a
separate acceptance case. Existing reviewed-skill activation requirements remain.

## Actual scheduling boundary and deployment

`TestScheduleBrokerProcessAndOrdinaryExecutionIntegration` passed both ordinary
execution and lost-ACK cases with an actual race-instrumented Broker process,
PostgreSQL, exact approved dispatch, ordinary Task/Input execution and result
delivery. `TestCommunicationScheduledOccurrenceGetsQuietCurrentAdmissionIntegration`
passed the actual owning scheduled ingress and explicit quiet admission path,
including cancellation fencing. Observation/source/deduplication tests exercise
the checked mode independently of scripted model planning.

Final Linux race/vet reports contain no skipped required cases. The separate
subprocess runtime-harness entry is deliberately excluded from the unit command;
the deployed runtime was exercised by the native prompts. Required process
schedule cases were run, rather than counted as skipped.

Code commits: `8b95a2d1` (evidence and checked monitoring), `b679e9fa` (bounded
reader context and concise output), `484a7d33` (value-free workflow examples).
Final deployed release:
`/srv/storage/butler/main/releases/research-quality-20261025-484a7d33`.
Migration 100 was applied. Health returned 200; the live `/proc/1/exe` digests
matched the built binaries:

* Core: `d5a746376aa66b89db61658853e5bd35d0b3c754866e8c7fe2a29f487b425256`
* Broker: `2a34d0198c86e5db3ca2b5f7fbedfdc5446afda504de01f2516648c3b0736778`
* WebFetch: `627079d7b669ab7c8f52c272df5364bd935c1fbd7077ae4b5a69c0664f617eca`

Compose changes only replaced the relevant candidate/migration mounts; private
profile, proxy, credential, source-deletion and budget configuration were retained.
The prior Compose snapshot is stored privately with mode 0600. This is a scoped
`laba` deployment from the draft PR branch, not a merge into main.

Additional raw evidence: `research-workflow-context.jsonl`,
`research-schedule-broker.jsonl`, `research-quality-native.json` and associated
vet reports in the task-owned reports directory. No costs, unknown states or
failed attempts were reset to obtain acceptance.
