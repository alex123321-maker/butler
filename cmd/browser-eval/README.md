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

This first slice covers HTTP recipient correctness. OTP, separate login origin,
native ingress driver, actual Chromium acceptance and latency aggregation remain
subsequent work in #162/#168. HTTP tests are not native/browser acceptance.

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
