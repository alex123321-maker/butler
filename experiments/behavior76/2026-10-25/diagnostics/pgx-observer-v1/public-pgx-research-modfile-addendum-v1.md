# Build mechanism addendum to the pgx observer review

The previously reviewed callback-only correction is unchanged. The original
5.8.0 conn.go SHA is
`076f1d636cd6e2a1b8cadaea24ac60375cd98f1d63034ff796427f49a4408684`;
the exact derived SHA remains
`9fdbb58e77cebad5ff489cec5481b1aac2d5e111e44157605b796f7a7d8078fc`.
Refer to public-pgx-observer-overlay-review-v1.md, SHA
`c7eebc0d2d020150e8823acf24731c2fb7739433ba53c95f3e23c4839bcca313`,
for causal evidence and the limits of the repair claim.

Go1.27 rejected replacements inside GOMODCACHE via -overlay. Preserve that
failed build as failed; it cannot be described as a successful overlay build.
The proposed equivalent research-only build mechanism is acceptable:

* Copy the complete existing5.8.0 module into a dedicated research directory;
  exact file inventory differs only at the reviewed three-line conn.go site.
* Copy root go.mod/go.sum and add only one local replace to the research
  modfile. Invoke only the research runner with explicit -modfile/-mod=readonly.
* Preserve root dependency files, shared module cache and production/Broker
  builds. Use the same derived dependency for both experimental arms.
* Pin the actual source inventories, original/derived file, research mod/sum,
  compiler/source identity and ELF. Record the actual modfile/local-replace
  mechanism in prospective evidence, not the rejected overlay mechanism.

SQL execution, arguments, statement-cache configuration, contexts, returned
errors, connection lifecycle and all authority/timeouts remain unchanged. The
public before/after regression must continue observing two cancellation errors,
open connection after the query and closed connection after rollback, while
the corrected trace records both returns and no phantom open span. Existing
safety/composition checks remain the agreed gates; no new behavioral threshold
or hidden case is introduced by this addendum.

This assessment does not admit a native window or another hidden replay. Old
failure evidence is immutable. The historical750ms authority latency remains
unresolved and must not be described as fixed by callback accounting.
