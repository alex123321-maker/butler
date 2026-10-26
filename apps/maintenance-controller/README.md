# Independent local owner control

Work in progress for issue21. Default disabled. This initial separate service
exposes only private owner status, stop/resume and bounded permission/revocation.
It starts no agent, inference, updater, node effect or Docker operation. Core is
not needed for its owner stop path. No Compose deployment/profile is enabled yet.

Enable explicitly with `-enable-local-owner-control -config <private file>`.
Configuration is canonical JSON with `listen` (loopback IP and port),
`database_url_file`, `signing_seed_file`, `owner_token_file` and `profile_file`.
The signing file holds exactly32 raw Ed25519 seed bytes; the token has32+ characters.
Keep all private files outside author/candidate mounts and model-visible stores.
The profile must already be installed and match the controller signing identity.
Startup never runs migrations, installs a profile or clears a stop.

Authenticated bounded POSTs to `/private/maintenance/owner` accept the typed
`OwnerControlRequest`. Owner and target come only from the operator profile.
There is no arbitrary signing endpoint or request-selected file/checker/target.
Private HTTP follows the existing explicit authenticated control-plane pattern;
it is separate from public channel ingress and normal model/tool execution.
Duplicate/unknown fields and scope from another operation are rejected. Errors
do not include request bodies, signing material, tokens or database URLs.

The actual separate owner-process test proves authenticated stop with Core
absent and retention across restart. Independent release checker, node effects,
trial and rollback acceptance remain pending. A listening endpoint is not
acceptance of the updater or proof of functional candidate quality.

The optional `node_token_file` enables a separate fixed
`/private/maintenance/node` POST boundary. Its private token must differ from the
owner token and remains outside Core/author mounts. The node can only present an
existing signed claim for one-use live dispatch or return a signed measured
outcome. PostgreSQL repeats the original permission, stop, version, review,
observation and expiry checks. The node credential cannot create owner permission,
reserve a claim, stop/resume the target or sign arbitrary data. Losing a dispatch
response never authorizes retry. The client checks that the returned record exactly
matches its original signed bytes. This boundary starts no node or Docker
operation; physical executor and release acceptance remain separate work.
