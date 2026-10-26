# Private physical identity observation

Component implementation for issue21, with actual separate-process validation.
Default disabled. This one-shot process reads one actual local Docker container
and signs only that measured physical identity with the private node key. It
cannot deploy, execute submitted code, sign arbitrary payloads, assert effect
settlement or clear UNKNOWN. A running process is not functional success.

One five-second context includes stdin and inspection. Input is read and checked
before private configuration/signing material is loaded. An open pipe without
EOF is bounded by the caller deadline; cancellation closes the owned input.
The independent original-source regression failed after5.25s with a100ms caller
deadline; the same regression now returns after0.10s without EOF or signing.

Invoke explicitly with `-enable-local-observation -config <private file>`.
Canonical private configuration contains `docker_socket` (local Unix socket),
`container` (one literal name), `profile_file` and `signing_seed_file` (32 raw
Ed25519 seed bytes matching the fixed profile's node public key). These files
and daemon access stay outside author/candidate environments and model context.

Standard input accepts one bounded canonical `ObservationChallenge`: exact
`profile_sha256`, positive `epoch`, and a SHA256-shaped `nonce`. It cannot select
an owner, target, daemon, image or key. Standard output contains only the signed
measurement envelope. Errors contain no request, configuration or key material.
The controller Store separately checks current profile/epoch/freshness; an old
challenge cannot authorize an update. Observation always leaves LastAttemptID
empty and EffectSettled false, preserving any unresolved physical action.

No Compose profile is enabled. Actual independent node effect execution,
functional trial, promotion, current-state compatibility and rollback remain
pending; this observer alone does not meet issue21 acceptance. The Linux/race
test forks a separate observer process and verifies its signature against the
actual stopped private Docker fixture. It does not infer functional success.
