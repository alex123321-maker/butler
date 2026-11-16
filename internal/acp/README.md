# ACP protocol probe

Issue #200, protocol and Linux subprocess qualification. This package is not
an authorization/security boundary. The separate experimental
[`antigravity-acp` adapter](../transport/antigravityacp/README.md) now uses the
protocol with a managed custom agent, fixed arguments and dedicated CLI HOME.
Its environment-only opt-in defaults off. The isolated runtime supports the
owner-approved opaque request/time/data budget, with unknown native tokens.
Core dials its private Unix socket; it never launches an authenticated host CLI.
The existing `antigravity` HTTP/SSE adapter remains separate.

`NewProbe` pins a local owner/session/task/run/version tuple and finite protocol
limits. `Run` owns one supplied `net.Conn`, closes it on every return, and uses
ACP v1 newline-delimited JSON-RPC. Protocol tests use `net.Pipe`; Linux process
tests additionally launch the synthetic test executable over actual stdin/stdout.
There is no dialing, CLI installation, credential lookup, login or external model
call. `/acp-fixture` in connection-only tests is a synthetic protocol field, not
a sandbox. Process tests use a freshly created directory instead.

Implemented fixture flow:

1. Initialize with client filesystem and terminal capabilities disabled.
2. Create a fresh session with no supplied MCP servers.
3. Select an exact offered model through session config options and require its
   acknowledgement. No default-model fallback; grouped/legacy model listings
   are not supported by this narrow prototype.
4. Admit one prompt, accumulate only bounded text chunks for the same session,
   and release them only after `end_turn`. Thoughts and usage payloads are not
   returned. Usage remains `unknown`, never zero/free or a billing guarantee.
5. Cancel every permission request, reject other client RPCs, and abort on tool
   activity or unexpected protocol/configuration changes. Partial output is
   discarded on any failure.
6. Apply the earlier parent deadline or child protocol wall limit to reads and
   writes. On failure after session creation, attempt `session/cancel` for at
   most 50 ms, then close. This cleanup allowance is additional to the wall and
   wire budgets (one bounded notification). No remote cancellation ACK or process
   termination is claimed by the connection-only API. No retries, continuation,
   load or resume.

## Linux subprocess qualification

The generic `RunProcess` API is a developer-only harness, not a provider or
registered tool. It checks the owning
binding before launch, consumes the single-use probe, and requires an explicit
absolute executable path, SHA-256 and bounded argument list. The opened executable
must be a regular executable ELF file, not a final-component symlink, FIFO or
script. It is hashed under the wall deadline and executed through its inherited
descriptor (`/proc/self/fd/3`), avoiding path replacement between hashing and exec.
This is a local identity check, not a signature/provenance check or protection
against a same-user attacker modifying the inode or its dynamic libraries.

The fixture child receives no ambient environment. HOME, XDG paths, temporary directory
and working directory point into a fresh private tree removed on exit. No
caller-supplied environment, MCP servers, host workspace or Google credentials
are injected. This does not hide the host filesystem or block network access.
Arguments are operator-owned test configuration, never model-proposed input.
`RunAntigravity` instead supplies fixed managed arguments and a validated private
persistent HOME; XDG/cwd/temp stay disposable. It does not perform login. The
dedicated HOME can contain private CLI auth/runtime data and is not erased by
per-turn cleanup. `PrepareAntigravityProfile` refuses an existing ordinary home
or unsafe customizations; managed files/settings must match exact templates.

Stdio bridges preserve protocol deadlines and frame/byte limits. Stderr is
discarded without retaining or returning any content; more than 64 KiB aborts
the probe. Partial output and nonzero exit failures are not accepted. Cleanup
closes pipes, SIGKILLs the dedicated Linux process group and the root separately, waits for the root
process (250 ms bound), and joins the pipe pumps. The root is not reaped until
the group signal, so its process-group identity is retained during cleanup.
Tests cover descendants holding inherited pipes after root exit, wall timeout
and parent cancellation, plus root reaping after the root changes its group.
Other operating systems fail closed before launching.

**A process group is not a process sandbox.** A process can detach with `setsid`,
read host files, use network access or spawn work elsewhere; Pdeathsig alone does
not contain descendants after a supervisor crash. No full process-tree or
effect-containment guarantee is made. A separate enforced container/cgroup and
Broker mediation remain mandatory before any real authenticated agent can run.

Limits count both outgoing and incoming frames/bytes. A frame is capped at
256 KiB; duplicate JSON keys, invalid UTF-8 and excessive nesting are rejected
using the existing `jsonstrict` implementation. Errors never include raw peer
errors, params or transcripts. Completed text is still untrusted agent data,
not independent evidence of an effect, task completion or delivery.

The tuple check only rejects mismatches to the local pinned tuple. It does not
reload PostgreSQL authority, observe revocation, reserve money/tokens or renew
a kernel lease. The protocol limits do not bound the agent's internal model
calls or background children. Denying client RPCs does **not** stop an agent
from using its own tools, reading its own files, or performing network access.
Do not connect the generic host process harness to an authenticated real agent.
Authenticated operation uses the dedicated non-root private PID1 container,
Google-only egress and two-phase dispatch in
[`acp-runtime`](../../cmd/acp-runtime/README.md). Protocol tests alone do not
establish that containment or actual Butler acceptance.

See [the ACP decision](../../docs/architecture/antigravity-acp-adr.md) for the
remaining release gates and source evidence.

## Validation

From an isolated checkout in PowerShell:

```powershell
. ./scripts/dev/common.ps1
$state = Initialize-ButlerAgentEnvironment
go test ./internal/acp ./internal/jsonstrict -count=1 -timeout=30s
go vet ./internal/acp
```

Run `go test -race ./internal/acp` where the Go race toolchain is available.
On Windows this checks the unsupported-platform refusal, not Linux process
execution. To run the Linux fixtures from Windows with an already installed
`golang:1.27.2` image:

```powershell
pwsh -NoProfile -File scripts/dev/test-acp-process.ps1
```

The runner uses an unprivileged, read-only, capability-dropped container with no
network or Docker socket; source is read-only and caches are checkout-local.
`--pull=never`, `GOPROXY=off` and `GOTOOLCHAIN=local` prevent downloads. `/tmp` is
explicitly executable to run the Go fixture binaries; it is not a secret store.
No live CLI is mounted or launched. Fixture passes prove local protocol and
owned-process-group behavior only, not Google compatibility, account eligibility,
native-tool containment, billing bounds or mediation of actual effects.
