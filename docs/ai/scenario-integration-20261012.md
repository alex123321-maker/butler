# Integrated scenario component acceptance

[PR73](https://github.com/alex123321-maker/butler/pull/73) contains the combined
changes after main merge `3d2b44bb0a0626db58c60b26f388fdd53826c8d1`.
The independently tested source commit is
`ec5bdde96c86294cff5c03813266500720086855`, immutable laba snapshot
`20261012-164015-527`. Subsequent documentation does not change tested code,
scripts, schemas, criteria or fixtures.

All four root checks completed successfully against that same combined snapshot:
293 passing test/subtest names, 120 top-level tests, zero failures and zero test
or package skips. Race detection and affected vet passed. Builds compiled the
actual production Broker and separate network-none converter. All model calls,
real credential/import/profile reads and shared deployment changes were zero.

Private evidence root:
`/srv/storage/butler/agents/scenario-acceptance-root/reports`.

| Check | Proof directory | Passing names / top-level tests | Test event SHA256 |
| --- | --- | --- | --- |
| Entire affected action authorization suite | `scenario-fault-20261012T094055Z-939492` | 118 / 57 | `011c99a6f178efd986c585d6dbee82c34de706e56699a1d9fd1d1220bb4dc31a` |
| Actual converter workers, nine continuation regressions and three delivery faults | `scenario-fault-20261012T094055Z-939669` | 30 / 12 | `a07f20e27c45387df7074d9a5f3f0c23d779fa64538b8783b60ed917d09950a4` |
| Logical transport/native SSE and owning PG usage | `tool-turn-usage-20261012T094056Z-939803` | 98 / 34 | `7f1dba85fcd5bcd9087fcc64548a44f9af6bd582d6343c9ce0beb977255d59ef` |
| Existing signed corpus and supplementary command/CLI checks | `corpus-broad-20261012T094056Z-940054` | 47 / 17 | `5e36e2a18ac8e934930d89c6321808490955e852973c0dc9612ac28b54e57379` |

The converter proof preserves 14 scripted inference admissions and ten tool
effects. Exact original bytes are independently compared before authenticated
owner verification completes the task; the model final remains unverified.
Three distinct stored attempt notices retain their identities. A recipient
acceptance followed by failed ACK persistence and natural claim expiry produces
`delivery_unknown`, without another send or converter/model execution. The
channel is a bounded local HTTP simulator, not a human Telegram recipient.

The actual snapshot CLI fixture report has SHA256
`ef6be5b91c420acc2508200da98ac659e5611e4f0b1593dae1c8c483c68c8d54`.
It records `FIXTURE_VALIDATION`, model `none`, `observation_only`, twelve exact
function/argument matches, twelve exact outputs and zero witness errors. All
twelve input/output usage and memory measurements remain NULL. Model-quality
evidence and execution authorization remain false. The candidate raw corpus
hash still matches the [independent bounded review](ru-command-proposal-independent-review-20261012.md).

Independent source-proof review found no blockers. The private context uses only
the exact already locked current DB lease/run/grant, clears caller actor/token
metadata and has no general write authority. Current task/source checks, final
fresh-clock grant expiry, rollback and non-replay behavior remain enforced.
Independent usage review reproduced and fixed conflicting complete trailers and
Cloud Code wrapper aliases before release. Earlier streaming deltas cannot be
rewound by a later usage observation.

The separate fifteen-report [history preparation](scenario-current-evidence-20261012.md)
retains all 45 native attempts, 20 known/25 NULL pairs and the partial known
74698/2210 token subtotal. No old report is rewritten or window reopened.

These accepted components advance #33 and fix an actual #20 continuation
composition failure. They do not close #33, #20 or #17. The complete
mandate/initiative/delegated-development/allowed-install/verified-result/one
owner-notice chain and its restart/Stop/delivery variants, protected independent
semantic model holdout, actual model measurements and full fair single/multiple
agent costs remain separate gates. Browser UNKNOWN quarantine remains intact.
