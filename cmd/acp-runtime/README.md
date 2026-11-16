# Isolated Antigravity ACP runtime

This control-plane service executes a pinned official Google CLI, not Butler
tools. Core connects through a private Unix socket; authority and durable budget
admission stay in Core. Each turn negotiates the exact model, waits for Core's
dispatch acknowledgement, then returns one bounded untrusted decision only after
the process group and all remaining private PID-namespace children are killed
and reaped. Disconnect cancels the child. The server admits one turn at a time.

Run the inference service as non-root PID1 in its own Docker PID namespace,
without `init`, host PID mode, capabilities, privilege escalation or host mounts.
Its network mode is `none`. A local loopback proxy reaches a separate Google-only
egress service over a second Unix socket. The egress service reuses Butler's
checked-DNS/public-address proxy and allows only the fixed Google auth/inference
hosts. It has no CLI profile or Butler service credentials. By default it uses
a separate network; an operator can attach only the egress service to an existing
proxy network and set `-upstream-proxy http://host:port`. This route accepts no
credentials and tunnels only checked public IP:443 destinations, including fixed
Google DoH bootstrap; it never gives upstream DNS authority or bypasses TLS.
Private-IP/metadata and
unrelated public destinations are denied. Shared socket volumes are private and
contain no Docker socket, owner credentials or database files.

Build the Linux/amd64 image with `scripts/dev/build-acp-runtime.ps1`. The script
verifies the official 1.5.9 archive's SHA-512 before extraction; the image and
supervisor independently pin the ELF's SHA-256. Ubuntu supplies the required
standard glibc libraries; CA roots are copied from the pinned Go builder. No
Python, Node, browser, git or new Go library is installed.

`-check` negotiates a session/model but deliberately refuses the dispatch phase.
It is a protocol/auth readiness check, not inference or Butler acceptance.
`-login` runs only the official `agy acp --login` flow in the dedicated profile.
It emits only a Google login URL and final status, never raw CLI logs or codes.
The owner supplies the code through a private mode0600 one-use file selected by
`-login-code-file` (default `/tmp/login-code`); the file is removed after reading.
Stop inference before login and do not copy tokens from T3/native/desktop homes.

The Compose overlay is `deploy/docker-compose.acp.yml`. It requires the separate
`subscription_agent_no_overage` exact provider/model budget policy in both Core
and Broker. Token counts and internal Google model-call counts remain unknown;
the durable unit is one ACP prompt, not a claimed single native inference.
CLI settings disable credit overage. Google account eligibility/terms remain the
owner's responsibility; the integration never disguises Butler's identity.
