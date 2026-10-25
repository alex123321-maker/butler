# Window A protected infrastructure follow-up

The original boundary audit and raw A files are unchanged. This is later
infrastructure evidence, not a retroactive change to that failed window.

The implementation author reported a protected, inference-free diagnosis:
OAuth refresh returned HTTP200, but persistence of refreshed encrypted settings
failed because the table-filtered restore omitted the system_settings key
constraint. No secret values or hidden case content were disclosed to this
reviewer. The reviewer did not independently execute credential resolution.

The public owning code supports that causal mechanism:
`internal/config/store.go:104` writes with `ON CONFLICT (key)`, and
`migrations/017_system_settings.up.sql:2` defines key as PRIMARY KEY.
The reviewer independently ran only a metadata/count query on isolated local
behavior76_test after the repair: rows1, distinct keys1, primary-key count1,
definition `PRIMARY KEY (key)`. It selected no key/value/credential bytes.
The pre-repair observations/refresh diagnostic remain attributed to the author.

A fresh infrastructure-only window is methodologically admissible because A
reached no model dispatch/output/tool proposal. Preconditions remain those in
the original audit: retain A's closed admission and NULL expense, no refunds or
old-task allowance expansion, unchanged candidate/controls/goldens/thresholds,
new explicit producer/window/output and cumulative accounting. Prefer the same
private arm mapping and exact schedule; the reviewer neither reads nor requests
that mapping before masked judgments are sealed. Fresh owned fixture IDs are
expected and must be correlated through aliases.

With a new768-entry ceiling, cumulative wrapper admissions are at most769.
A's one public wrapper admission and one unobserved kernel attempt describe the
same original event; they are not two independent inference calls. Authentication
refresh/discovery is separate control-plane activity, not model inference and
not a measured zero invoice. No behavioral quality or issue-closure verdict has
been issued.
