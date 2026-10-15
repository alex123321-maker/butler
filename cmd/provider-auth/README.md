# Private provider auth operator utility

Provider secrets stay in the strict encrypted PostgreSQL SecretStore. The CLI
uses the runtime's environment/private references; never pass tokens, passwords,
provider JSON or key material as arguments or publish its environment.

- `-action inspect`: read-only connection/expiry projection of existing State.
  No refresh, inference, account identifier or pending-login details.
- `-action diagnose-auth`: inspect before/after the normal automatic Resolve,
  with fixed OAuth refresh/persistence diagnostics. It can renew an expired token
  and writes rotation to encrypted PG before returning. It makes no inference;
  `unsafe_refresh=true` means stop and reconcile before any further auth/model call.
- `-action diagnose-cloudcode`: the same safe projection plus current paid
  subscription discovery. Use the service's exact network environment; it makes
  no generation request and is not proof of inference access.
- `-action quota`: fixed Cloud Code subscription discovery/model metadata read,
  using the same account and current Antigravity model. Prints only model,
  advertised flag, numeric HTTP status, optional remaining fraction/reset time.
  Unknown is null, never zero; this meter does not attribute individual AICode
  requests or establish inference access. No generation, onboarding, credits or
  model/endpoint fallback occurs. Routine Resolve can refresh first.

Only one refresh-capable operator process may use a given stored credential at
a time. Stop the isolated worker before these maintenance actions; Manager's
mutex coordinates threads, not other processes. A connected or renewed token
does not prove model entitlement. The older `smoke` action makes actual model
requests outside execution admission and must not be used for memory acceptance.
