# Skill stream guard fix: model-free before/after on laba

The product change removes catalog reloads on each **plain text delta**. Every
provider request and every non-delta event, including tool calls, batches and
final answers, still requires a fresh proof. Text deltas can reuse a proof for
less than 250ms; both the idle ticker and the elapsed-time branch on a busy
stream refresh it. An envelope containing final/tool fields is never treated as
a plain delta. The existing 2s proof timeout and parent cancellation remain.
Source/privacy, Broker dispatch, and atomic final-result checks are unchanged.

## Controlled comparison

Fresh, sequential before/after compositions on laba, 2026-11-02. Same original
workflow task, actual Core, Broker, extension/HTTP runtimes and PostgreSQL; same
scripted provider, event fragmentation, oracle and five scripted provider
methods in C. No native provider is called. Both use pinned images, 1 CPU and
3GiB for the race-instrumented app; each has its own fresh internal Docker
network and PostgreSQL. The AFTER product source differs only in
`skill_provider.go`; it also includes the new unit-test file.

| Arm / final text fragments | Before, s | After, s | Pinned RPC before → after |
|---|---:|---:|---:|
| B / 0 | 0.905 | 0.924 | 0 → 0 |
| B / 180 | 1.024 | 1.024 | 0 → 0 |
| C / 0 | 1.436 | 1.383 | 9 → 9 |
| C / 180 | 10.360 | 1.473 | 223 → 9 |
| C / 360 | 19.638 | 1.576 | 439 → 9 |
| C / 180 repeat | 10.365 | 1.484 | 224 → 9 |

180-fragment execution is about 7x faster; 360 fragments about 12.5x. All 12
scenario results passed their original artifact/evidence scorer. No provider
methods, final content, task conditions or verification steps were removed.
The text-fragment penalty now stays small instead of growing with private
catalog RPCs. Journal-only controls also stayed similar (128 saved payloads,
three SQL operations plus JSON and file/fsync each): about 0.845s before and
0.864s after, rather than an explanation for the old multi-second stream tail.

These are isolated scripted measurements under Go race, not new model-quality
scores or production latency guarantees. The collector's original buffering
effect and the prior [137s attribution](../2026-11-02-skill-latency/README.md)
remain explicitly separate. Historical paid runs and their expenses are intact.

## Verification

Both compositions passed the existing skill/recovery unit checks with `-race`,
`go vet` for the orchestrator package, 6 workflow controls, journal replay,
12 actual Core/Broker trace scenarios, the reviewed-skill real Broker durable
approval/recovery fixture, and the complete PostgreSQL-backed `internal/skills`
test suite. Required fixtures did not skip; no race report occurred.

New AFTER guard tests cover:

- 600 text fragments without per-fragment catalog reload or lost events;
- revocation before final, single-tool, batch-tool and completed events;
- revocation during a continuously ready text stream (no ticker starvation);
- parent cancellation while a catalog proof is pending;
- a mixed final/delta envelope cannot reuse a cached proof.

The existing idle-stream revocation and catalog-failure-before-provider tests
also pass. PostgreSQL checks include lease/task constraint revocation, source
deletion/restore, required actual read authority and concurrent pinning. The
real Broker fixture retains ordinary owner approval and durable recovery.

[before.json](before.json) and [after.json](after.json) bind all timing and RPC
measurements to the tested source, race app binary and full private log hashes.
[verification.json](verification.json) records successful test names, source
hash of the new unit tests, and stopped private PG / removed private networks.
Local candidate guard and test bytes were compared with those actually tested
on laba. Both completed exits are zero. **New native model requests: 0.**

## Reproduction and scope

`run_comparison.py` reuses the previous archived
[`run_lab.py`](../2026-11-02-skill-latency/run_lab.py) and
[`latency_diagnostic_test.go.txt`](../2026-11-02-skill-latency/latency_diagnostic_test.go.txt).
Upload those helpers, the current product `skill_provider.go`, the new
`skill_stream_test.go`, and the comparison script into the private input path
specified by the script. Select new exclusive root/shell-script suffixes before
another run. It builds without network access, then runs on internal-only
networks with no provider auth mounts or native flags.

Core/HTTP fixtures run from their package directory; the existing reviewed-skill
approval fixture runs separately from the repository root to resolve migrations.
Earlier roots r1–r3 were kept: r1 omitted the action fixture DB setting; r2 had
the approval fixture in the wrong working directory; r3 put the HTTP fixtures
in the wrong working directory. These setup failures did not spend model usage
or change the product patch. The complete r4 gates pass on both variants.

The fix is present and tested in isolated laba source copies and the research
branch. This task does not merge main or restart/deploy the running Butler.
