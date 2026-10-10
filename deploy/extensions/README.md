# Install a reviewed local extension

Apply migration050 on the authoritative PostgreSQL schema before enabling the
optional overlay. Back up durable state first; its down migration refuses to
discard installed history. Nothing installs a remote package or starts a service
from a manifest. Deployment and package review are operator actions.

1. Build the reviewed `docker/tool-extension/Dockerfile`, inspect its source and
   measure the resulting `/usr/local/bin/tool-extension` SHA-256. Pin your image
   digest in your deployment. A profile descriptor records the measured artifact
   and reviewed source; a descriptor alone is not image-integrity attestation.
2. Create a separate random runtime token file under `.secrets`, readable only by
   the container UID10001. Use it only for Broker-to-extension RPCs; never reuse
   owner, action-authority or provider tokens. It is not a model argument.
3. Configure `BUTLER_EXTENSION_PROFILES_JSON` with a fixed target and dependency:
   `{"reference-http":{"target":"tool-extension:9090","dependency":{"id":"reviewed-http-runtime","version":"1.0.0","source":"https://github.com/alex123321-maker/butler/tree/REVIEWED_REVISION/apps/tool-extension","sha256":"MEASURED_EXECUTOR_SHA256"}}}`.
   Replace both placeholders. The installed manifest dependency must match exactly.
4. Use `deploy/docker-compose.yml` plus `deploy/docker-compose.extensions.yml`
   (and your ordinary persistence/private-auth overlays). The executor is non-root,
   read-only, capability-free, has no host filesystem/Docker mount and receives
   only its narrow token/public profile. It is outside the backend PG network.
5. Generate a disposable fixture manifest with
   `go run ./scripts/dev/extension-manifest -runtime-sha256 MEASURED_EXECUTOR_SHA256 -source-revision REVIEWED_REVISION`.
   Review the exact JSON and dependencies, then install/review it on `/extensions`
   using the owner token. No source URL is fetched.
6. Save only the desired capability subset and public origin. Missing origin is a
   visible configuration error and produces no readiness request. The configured
   recipient must have a public allowed address: production never accepts a local
   loopback/internal container URL. The lab acceptance harness supplies a test-only
   trusted public resolver/fixed dial without changing production rules.
7. Validate fixed GET readiness and enable. Then grant an exact task attachment
   with current task/constraint revisions, manifest/generation, bounded resources,
   finite expiry and permission revision. Goal tasks instead use the exact owner
   mandate `extensions` entry plus its normal tool/effect/purpose/resource bounds.
   Enablement alone never grants a task access. Ordinary reads and commits use
   the same approvals/autonomy, journal, credential and budget gates as built-ins.
8. Review/stage an updated version, select it and save configuration. This disables
   the capability and invalidates old proposals/permissions. Rollback chooses a
   retained version and likewise needs fresh readiness/permission. Disabling or
   rollback preserves goals, tasks, artifacts and effect history.

The reference recipient `scripts/dev/extension-fixture` needs an operator-created
private data directory (`BUTLER_EXTENSION_FIXTURE_DATA_DIR`), not a project path
from the model. It serves only one synthetic README; it never invokes a process,
applies changes or accesses a real repository. GET `/project` reports the failed
ready flag and revision. POST `/project/patches` accepts the exact README
before/after pair, atomically persists an immutable patch and returns the existing
`external_receipt_v1` recipient proof. GET `/project/patches/<artifact_id>/checks`
passes on that prepared artifact with `applied=false`. GET
`/project/effects/<system-key>` returns the persisted receipt after a restart.
Prepared artifacts remain commits; receipt evidence, not the operation name or
generic HTTP status, permits confirmation. These fixture observations remain DATA
and never authorize a task or establish real repository quality.
