# Tool Workspace

Private Go service for bounded task code artifacts. It plans no run and executes
no package/shell code. Tool Broker remains the only admission path; the runtime
requires a committed one-use dispatch, original input/source/worker/task proof,
shared budget and exact resource/file CAS. Actual Store/Observer confirmation
commits file metadata, immutable source/effect evidence and lease consumption.
Unknown outcomes retain quarantine and cannot replay.

The operator provisions a separate private absolute directory and service token,
plus the current independent Catalog deletion manifest. The runtime never creates
or resets that manifest, runs migrations, imports host credentials or mounts a
repository implicitly. Source delete/revoke starts the bounded durable privacy
job; actual erasure completes the queue only after fixed-root removal/sync. This
job cannot reconcile, renew a worker or resume a stopped task.

Required environment:

| Variable | Meaning |
| --- | --- |
| `BUTLER_POSTGRES_URL` | Same durable database as Broker and owning Catalog |
| `BUTLER_WORKSPACE_PROFILE_ID` | Stable exact operator profile, max128 bytes |
| `BUTLER_WORKSPACE_ROOT` | Separate private absolute artifact directory |
| `BUTLER_WORKSPACE_RUNTIME_TOKEN_FILE` | Private regular service-token file |
| `BUTLER_MEMORY_DELETION_MANIFEST_FILE` | Existing independent deletion anchor |
| `BUTLER_GRPC_ADDR` | Literal loopback or exact `unix:///run/butler-workspace/runtime.sock` |
| `BUTLER_HTTP_ADDR` | Literal loopback liveness address; default127.0.0.1:8086 |
| `BUTLER_POSTGRES_MAX_CONNS` | Default2; no nested pool borrowing under locks |

The default gRPC address is127.0.0.1:9096. Authenticated gRPC readiness binds
profile, target and all canonical contract hashes, checks actual root identity/
private permissions, current schema, immutable-history/deletion triggers and
deletion anchor. Anonymous/mismatched probes are denied. Readiness supplies no
execution permission or content. A fixed Unix socket requires its own directory
with mode0700, and the socket is0600. Startup refuses active/non-socket/unknown
endpoints; only a verified unchanged stale refused socket may be replaced.

The Linux runtime holds an exclusive advisory lock on its artifact directory
inode for its lifetime. A competing runtime sharing that root cannot start,
including with another profile or listening address. Independent artifact roots
stay concurrent. This requires directory `flock` support; other operating systems
fail closed. No lock/PID file or shared temporary payload is created. Root path
replacement or shared permissions deny readiness and new execution. Shutdown
drains active execution/readiness/privacy work before releasing the root. A
crash releases this process lock without clearing any PG resource/effect state;
restart is not evidence that an uncertain operation was absent or permission to
reconcile/replay it. See [root ownership acceptance](../../docs/ai/workspace-root-owner-acceptance.md).

For Compose use the explicit optional
[workspace overlay](../../deploy/docker-compose.workspaces.yml). Prepare separate
data and socket directories owned by UID10001, mode0700, and a readable private
token owned by that UID, mode0600. Broker shares only the read-only socket mount
and token; the artifact root belongs only to this runtime. Keep the existing
independent deletion-anchor volume initialized outside database backups. Provide
the four `BUTLER_WORKSPACE_*_SOURCE`/profile variables required by the overlay.
It publishes no runtime port or host Docker socket and runs with a read-only
image, dropped capabilities, no privilege escalation, and bounded CPU/process/RAM.
`/health` is process liveness; Broker independently requires authenticated exact
runtime readiness. The overlay is not applied by tests or package installation.

Broker and Orchestrator `BUTLER_WORKSPACE_ENABLED` are operator-only and default
false. Stored settings cannot enable this topology. The application supports
bounded artifact preparation with honest quarantine; separately authenticated
owner reconciliation and the remaining integrated issue17/native16 gates are
pending. See [bootstrap acceptance](../../docs/ai/workspace-bootstrap-acceptance.md).
