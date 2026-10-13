# Independent V5 release-chain witness audit

This Go command reads an already completed, sealed V5 chain. It cannot call a
provider, authenticate, execute a tool, mutate PostgreSQL or reopen a native
window. A passing witness report always keeps `issue33_ready=false`: final
composition tests and issue-wide independent acceptance remain separate gates.

The quality dependency is the exact 144579-byte, LF-terminated independently
reproduced wrapper `f8248d19ec2a0c222b39d46fbc46c0d0cf4ccc3c1361c89965b597eedbb21cab`.
The exact LF prerequisite is
`9480184b2f5586d8f9826b8db328c52a845ee812bd48afbf1432ca7f30bfd5d3`,
carrying 535 prior attempts and chain-history fold
`2539239ea8905a677cb14b0256a9744c3354e365754cae127bc517c54c35d5af`.
The historical known total is a subtotal of 563997 tokens; 46 total-token
dimensions remain unknown. Attempt 266's unknown cause is preserved.

## Build and offline tests

From the repository root, after integration:

```sh
CGO_ENABLED=0 go build -trimpath -o /proof/bin/chain-final-audit ./scripts/dev/release-chain-v5-final-audit
go test -race -count=1 -json ./scripts/dev/release-chain-v5-final-audit
go vet ./scripts/dev/release-chain-v5-final-audit
```

Tests use existing exact public quality/prerequisite artifacts and synthetic product
records. The positive synthetic witness uses 70 entries solely as a fixture;
the auditor accepts a variable actual request count within the frozen cap of
80. Coherent tamper tests recompute payload hashes and change both collector and
PG projections where appropriate. They test paid failure retention, unknown
dimensions, excess admissions, image/profile binding, full source inventory,
budget scope sums, authority and action ownership, stopped-child denial,
original patch bytes, and the single notification's delivery state.

## Exact read-only inputs

The operator independently retains the source commit, complete source manifest,
TreeHash, prepared executable hashes, immutable image IDs and hashes of the
completed run's safe exported leaves. Never infer these pins from a collector's
`passed` flag. Stage only the following leaves as UID/GID 10001, regular
non-symlink files with mode 0600, in a mode-0700 directory. Their names in the
audit container are fixed:

| Container leaf | Exact origin |
| --- | --- |
| `/scope.json` | Completed private `scope.json` content-free export |
| `/pg-ledger.json` | Owning `pg-ledger-witness.json` export |
| `/pg-initial.json` | Owning `pg-initial-witness.json` export |
| `/pg-product.json` | Owning `pg-product-witness.json` export |
| `/runtime-state.json` | Completed actual Docker state and three actual image IDs |
| `/hardware.json` | This process run's declared host/fixture/image manifest |
| `/runtime-profile.json` | Exact prepared converter profile |
| `/source-manifest.sha256` | This process run's complete GNU sha256sum manifest |
| `/source-commit.txt` | Exactly the prepared 40-character commit plus LF |
| `/quality.json` | Exact public `cloud-shadow-v5-final-audit-20261013.json` |
| `/prerequisite.json` | Exact public `release-chain-v5-high-prerequisite-20261013.json` |

Mount the frozen source snapshot at `/src:ro`, the prepared measured executables
at `/app.test`, `/tool-broker`, `/tool-converter`, and this auditor at
`/chain-final-audit`, each executable mode 0555. Hashes stream over full binary
bytes; there is no small executable-size limit. Do not mount credential import
files, authentication material, provider originals, a Docker socket or whole
private/runtime directories. No environment credentials are needed.

Run in Docker with `--pull never --network none --read-only --user 10001:10001
--cap-drop ALL --security-opt no-new-privileges:true`, a bounded process/memory
limit, the retained immutable Go image, and only the mounts listed above.
The Go image is fixed to
`sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61`.
Use these flags, with each value supplied from independently retained evidence:

```sh
/chain-final-audit \
  -commit "$commit" -source-sha256 "$tree_sha" \
  -app-sha256 "$app_sha" -broker-sha256 "$broker_sha" \
  -converter-sha256 "$converter_sha" \
  -postgres-image "$pg_image_id" -runtime-image "$converter_image_id" \
  -scope-sha256 "$scope_sha" -pg-ledger-sha256 "$ledger_sha" \
  -pg-initial-sha256 "$initial_sha" -pg-product-sha256 "$product_sha" \
  -runtime-state-sha256 "$state_sha" -hardware-sha256 "$hardware_sha" \
  -runtime-profile-sha256 "$profile_sha" \
  -source-manifest-sha256 "$manifest_sha"
```

The operator saves stdout as a **new** exclusive mode-0600 artifact, preserving
the exact emitted JSON plus LF. Retain its SHA256, size, stderr error code and
exit status. Do not overwrite earlier failed or empty proof files. No output
path, model argument, database URL or native-run switch exists in this command.

## Checked semantics and evidence limits

The command independently recomputes every source manifest entry and complete
inventory, TreeHash, runtime executable hashes and production profile digest.
Actual Docker image IDs must match the declared hardware and prepared operator
pins. Strict bounded duplicate-free UTF-8 JSON rejects ambiguous frames, extra
product fields and unbounded values. Safe error codes never print input bodies.

The collector and independently exported PG window/requests must match exactly,
including nullable usage, terminal observations and deadlines. Ordinals start
at 536; five scenarios follow the frozen order `normal_sent`, `restart_sent`,
`explicit_absence`, `lost_ack`, `owner_stop`. The actual window lasts at most
60 minutes, each initial-to-final scenario at most 20 minutes, each native entry
at most 120 seconds. Every new input/output/total dimension is measured for PASS.
All owning-PG admissions are counted before semantic success or cap checks;
failure and cap denial preserve their known subtotal and unknown dimensions.

Initial/final witness hashes are recomputed with numeric-preserving canonical
JSON. Product records bind current original source revisions, native ordinary
Core runs, root/child ownership, mandates, budget reservations and all eight
scope charge dimensions, unique authorized Broker effects, independent parent
and child checks, activated catalog, original conversion hash and actual
prepare/check receipts. The exact synthetic README patch is prepared, checked
and unapplied. Ordinary model finals remain unverified; one owner result
completes the promoted task and creates one stable semantic notice. Lost ACK
stays `delivery_unknown` without resend; explicit absence permits the one
specified retry; owner stop permits neither child effects nor successful result.

Evidence is limited to a shared administrator-controlled host. Owner setup and
cheap reads are model-free; the production finite-IR builder authors the missing
adapter selected by the native child. Project and Telegram endpoints are
loopback fixtures with no human recipient or live deployment. Kernel budget
reservations are conservative quotes, separately correlated with the native
ledger's actual measured costs. Cloud weights, server effort/hardware/RAM,
subscription invoice and historical unknown expenses remain unavailable. This
checker cannot supply missing independent measurement or broader release tests.
