# Independent V5 corpus seal

Run this operator tool only after a separate reviewer approves every exact corpus
and review-declaration byte. It verifies both supplied SHA256 identities, the
inventory and public original witnesses, then creates a new Ed25519 signature.
The key and files are exclusive 0600 writes in an existing UID10001-owned 0700
directory. Partial failure seals that directory; never remove/reuse it.

Build on isolated laba with network disabled. Mount candidate source read-only
at `/src` and only a new reviewed directory at `/review`. Supply independently
reviewed hashes with `-corpus-sha256`, `-review-sha256`, `-semantic-sha256`,
`-schema-sha256`, `-policy-sha256`, `-criteria-sha256` and `-resources-sha256`.
Compiled library identities must match before any signing key is generated.
The reviewer declaration
must already exist as `/review/independent-review.md`. The output contains only
public artifact hashes. Never mount the private signing key in model or app
containers. Shared administrator control is explicit; this is role separation
and an immutable review seal, not hardware isolation or a blind benchmark.

Signing grants no model inference, execution, benchmark PASS or release-chain
permission. Native preparation must still bind source, runtime, hardware, signed
corpus, criteria and a fresh finite window.
