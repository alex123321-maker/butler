# Final delegation TLS prerequisite

This standard-library Go command opens exactly two concurrent, credential-free
lanes to `daily-cloudcode-pa.googleapis.com:443`, through the fixed lab proxy
`antigravity-proxy:18080`. Each lane resolves the target and proxy DNS, opens TCP,
sends an authless HTTP CONNECT containing only the fixed authority, and performs
a TLS1.2-or-newer handshake with normal certificate and hostname verification.
No application/provider HTTP follows TLS. No model call, auth import, environment
configuration, cookie, token or project ID is read or sent. No destination or
verification setting can be selected through command arguments.

The whole pair has one15-second deadline. Both successful handshakes are required;
one refusal, timeout, invalid certificate or incomplete lane denies the pair with
no retry/fallback. JSON evidence includes only fixed host, lane booleans and fixed
failure categories, never DNS addresses, headers, certificates or error strings.

`delegation-final-high-lab.sh prepare` runs synthetic race/vet checks and builds
the executable. The sole `execute` validates the tested source/release and probe
binary hash, seals its separate window, then runs this command in a read-only
UID10001 container with only the binary mounted. Authentication metadata or import
access starts only after both lanes pass. Proof and its SHA256 remain in the
private final report directory. A network denial spends0 native slots but keeps
the invocation sealed. Historical windows and reports remain immutable.

The tests use real local TLS over in-memory sockets and verify concurrent
connections, no application bytes, certificate/hostname verification, partial
failure, DNS/deadline failure and bounded proxy-response denial. They require no
provider, credentials, Docker external network or model inference.
