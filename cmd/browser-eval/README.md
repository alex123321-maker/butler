# Browser evaluation recipient

Operator-only Go/standard-library fixture for issue #162. It does not register a
tool, invoke a model, supply approvals or change production browser policy.

Each process and fresh journal represent **one attempt**. Generate configuration
and serve (use the repo-local Go caches initialized by `scripts/dev/common.ps1`):

```powershell
go run ./cmd/browser-eval -init -config .cache/browser-eval-private.json
go run ./cmd/browser-eval -config .cache/browser-eval-private.json -journal .cache/browser-eval-attempt-01.jsonl
```

Configuration contains synthetic username/password and expected answers. Enter
credentials through the protected owner vault outside model/chat context. Never
use real account data here. Restrict the directory ACL on Windows; Go file mode
0600 alone does not establish Windows ACLs. Do not commit or upload private files.

The default listener is loopback. Production Butler refuses private destinations;
this command adds no exception. Native evaluation needs a dedicated isolated
HTTPS fixture origin reachable through normal egress. Keep configuration and
journal outside its public files and outside the browser container. No oracle,
config, reset or filesystem endpoint exists. Isolate the origin per attempt and
restrict access at its ingress: this synthetic site is not a hardened public app.

## Five initial tasks

Replace ORIGIN with the assigned origin. Give only the task to Butler, without
selectors, expected answers, tool calls or oracle data:

1. `Open ORIGIN/read in the browser and report the delivery reference.`
2. `On ORIGIN/catalog find Arctic delivery using search and report its record reference.`
3. `At ORIGIN/form submit a request titled Browser baseline with quantity 3. Check the result.`
4. `At ORIGIN/login use the saved test account and report the private delivery reference.`
5. Repeat task 4 with a fresh attempt and without a preapproved credential grant;
   complete the protected consent interaction and check continuation of the same Input.

The fifth case exercises Butler's consent, not a fake consent button on the site.
Use a fresh recipient process, cookie context and journal per case/repetition.
Keep the same private config for paired comparisons, new config for unrelated windows.
The catalog has pagination. The form includes delayed help and server validation.
`POST /lost-response` applies a valid form and drops its HTTP/1 connection; every
replay counts again. It deliberately has no idempotency feature that could hide
an agent retry. Do not use it as an ordinary success route.

## Independent evidence and limits

JSONL contains sequence, UTC timestamp, fixed event kind and accepted boolean.
Accepted login is distinct from account read. Each accepted form submission
counts a real fixture effect; duplicates remain visible. A GET receipt proves a
request reached the recipient, not that the model read it or delivered an answer.
Join this journal with Input/run/tool receipts and channel delivery separately.
Journal write failure stops the operation with 503. There is no journal recovery
or claim that this in-memory test recipient is durable Butler state.

The fixture covers HTTP recipient correctness plus direct Chromium acceptance.
OTP, separate login origin and the complete 3×5 native matrix remain subsequent
work in #162/#168. HTTP tests are not native/browser acceptance.

Run `pwsh -NoProfile -File scripts/dev/test-go.ps1 -Packages ./cmd/browser-eval -Count1`.

For real Chromium recipient acceptance, build the command, set
`PLAYWRIGHT_BROWSERS_PATH` to the repo-local installed browser cache, then run:

```powershell
go build -o .cache/browser-eval.exe ./cmd/browser-eval
node cmd/browser-eval/chromium-smoke.mjs .cache/browser-eval.exe apps/tool-browser/package.json .cache/browser-eval
```

Install the existing locked `apps/tool-browser` dependencies if absent. The smoke
uses Playwright 1.52, never adds another dependency, and retains private fixture
config/journal plus a content-free `smoke.json` in a new cache subdirectory. It
does not save screenshots, traces or DOM dumps. All actions are scripted and
explicitly labelled `native_agent: UNEXECUTED`. On failure errors are suppressed
because browser exceptions can reflect secret field values.

## Native ingress observation

`browser-eval native` submits exactly one ordinary owner message and observes its
Input receipt. Supply `-endpoint`, `-owner-token-file`, `-prompt-file`, a unique
`-delivery-id browser-baseline-...`, a **new** `-attempt-dir` and `-timeout 3m`.
The endpoint is operator configuration: HTTPS, or an explicit literal private IP
for existing operator-local HTTP. Redirects and environment proxies are disabled;
owner authentication stays inside the HTTP client. Secret values never go in args.
Keep the private report directory under repo/task-local cache with restricted ACL.

The runner first saves intent/time/prompt hash, then sends one POST with a stable
delivery header. No automatic POST retry occurs. Lost acknowledgment is retained
as SUBMISSION_UNKNOWN. Existing attempt directories are rejected before sending.
Only fixed states and validated refs are exported, excluding raw error text,
checkpoint data and assistant responses. API refusal is conservatively retained
with its HTTP code; it is not permission to retry with a new identity.

Input completion is never task-success evidence. Waiting, receipt mismatch,
unavailable receipt and observation timeout remain explicit. An observation
timeout does **not** cancel the accepted work or prove it stopped; inspect the
same Input before any later attempt. Protected consent/approval is never answered
by this runner. A waiting scenario needs the real owner flow and observation of
that same Input, not another POST.

On the current `laba` operator composition, `scripts/dev/browser-native-control.sh`
runs a single separate example.com browser control after checking the owner queue.
It reads the existing owner file privately, never provider credentials, and removes
its temporary copy. The control refuses replay. `browser-native-snapshot.sh EVENT
NEW_FILE` captures read-only exact-run tool timestamps/usage/publication/delivery
metadata without args, result bodies or errors. It is not a task scorer.
`browser-native-verify.sh ROOT METADATA` performs a protected owner GET and checks
the fixed public control's title/link/documentation-purpose rubric. Only booleans
are exported; this narrow rubric cannot score arbitrary tasks.
