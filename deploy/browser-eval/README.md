# Isolated public browser recipient

This is an operator-only disposable test recipient for #162, not another Butler
service or a replacement for its Docker Compose deployment. The user selected
the existing `devops` Ubuntu/Nginx host. A bounded systemd template keeps this
small Go binary loopback-only behind the existing Nginx TLS terminator; no Docker
or persistent database is needed on that host.

Install the compiled Linux binary at `/opt/butler-browser-eval/browser-eval` and
the unit at `/etc/systemd/system/butler-browser-eval@.service`. Use the dedicated
system user `butler-browser-eval` without a login shell. Each new attempt gets
its own mode-0700 directory under `/var/lib/butler-browser-eval`, a generated
private synthetic config and an exclusively created journal. Serve only one
attempt at a time on port 18097. Preserve previous journals; do not reset them.

Configure a **new** Nginx virtual host for the assigned test subdomain, a valid
public certificate and HTTP→HTTPS redirect. Proxy only to `127.0.0.1:18097`.
Never expose a filesystem root, the private config/journal or an operator API.
Disable access logging (especially query/body data), use finite proxy timeouts,
bounded request bodies and noindex headers. Keep other virtual hosts unchanged.

`prepare-http.sh butler-eval.ZONE` installs only a fresh, ACME-only port-80
virtual host and refuses existing owned paths. `enable-https.sh` waits at most
15 minutes for both REG.RU authoritative nameservers, then requests one
certificate and renders the prepared HTTPS template. It does not retry failed
issuance. Verify public DNS, valid HTTPS and the recipient before native tasks.
Upload `nginx-https.conf.template` and `renew-nginx.sh` to their
`/var/tmp/butler-browser-eval-*` paths before enabling. The deploy hook reloads
Nginx only when this test certificate renews and its configuration validates.
The issued endpoint is `butler-eval.report-template.ru` on `ssh devops`.

The service forces Secure/HttpOnly/SameSite cookies behind TLS without trusting
client-controlled forwarded headers. It has no outbound network access. A crash
does not restart it, since a new process cannot resume its in-memory auth state
or append a second attempt to the old journal.

After each attempt: stop that unit, retain evidence, prepare a **new** directory,
start the new instance and verify HTTPS before sending the next task. Stopping
the recipient does not cancel Butler's Input or reconcile unknown effects;
check the previous Input separately first. Native observation timeout alone is
not sufficient permission to reset a still-running scenario.

`prepare-attempt.sh NEW_ID PREVIOUS_ID` refuses existing attempt directories,
retains their journals, records the binary hash and waits for HTTP readiness.
The initial systemd-active/HTTP-ready race was observed and fixed before sending
the second native Input. The matrix driver has an explicit guarded recovery for
that one unused prepared instance, not a generic replay switch.

Use only synthetic accounts here. Actual product credentials continue to pass
through the protected vault and Tool Broker. This recipient grants no runtime,
task, account or effect authority.
