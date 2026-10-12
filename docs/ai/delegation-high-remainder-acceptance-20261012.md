# High delegation remainder — two calls within the original ceiling

The owner requested full issue16 acceptance, including repairs, with subagents.
The prior [full High report](../../deploy/delegation/native-acceptance-high-20261012.json)
remains FAILED and SEALED. It proves two real High parent receipt projections,
four correct native child observations after real parent worker loss, and
width1 independent source verification/owner accept and reject. It did not prove
width2 overlap or run the final controls. Its SHA256 is
`afa63de06b6991feb3a96b322d442d573d4d2834c51a70c07e5fd4419f751a91`.

The timing defect was in the fixture: independent claim expiries were staggered,
but the replacement started before the latest expired. The new fixture waits
for PostgreSQL clock to reach the latest actual expiry before starting the fresh
ordinary worker. No claim or lease timestamp is edited. Production input already
has two workers and is unchanged. [Actual PG proof](delegation-parallel-recovery-acceptance.md)
checks staggered expiry and two simultaneous current core contexts/fences.

This explicitly mixed-evidence experiment repeats ordinary Broker, source,
report and control paths using scripted parents and scripted width1 children.
Only the two width2 children enter the real Go High provider. Scripted activity
is labelled in each width's `execution_mode` and at report level; its eight
logical model-turn reservations are exported separately as `scripted_model_turns`.
It is not new native parent/width1 generation or native quality evidence.
Only owning rows whose run IDs occur in the native attempt journal supply
`new_usage`; neither scripted values nor their NULL rows enter native accounting.
The final acceptance audit combines original native evidence with the remainder.

The router accepts only actual owning runs in the two fixed fixture roots,
their exact current empty-permission child contracts and selected source pins.
The existing context observer and all production lease/source/Broker/budget/final
guards remain. Unknown contexts deny before native entry; there is no automatic
native fallback or inference retry. Only two actual width2 child starts are
allowed, with strict unchanged native overlap==2 and numeric final verification.
Width1's labelled component overlap uses owning model-attempt timestamps; prior
native width1 overlap==1 remains the independent real-generation evidence.

The first remainder invocation is SEALED and failed before any native entry.
Its [immutable preflight report](../../deploy/delegation/native-remainder-high-preflight-20261012.json)
has SHA256 `ce8835a6fefcd600c6b28b88fed73212453c5af54016bdd6d8c77c4e506dbeae`:
37 total/0 new native calls, six scripted turns, width1 component checks passed,
then width2 parent admission stopped. Scripted tool references were reused across
the two parents; the real dispatch committed before global tool-call persistence
failed and correctly became effect_unknown. No retry reconciles that old run.
Scripted references now include their actual run ID. The routing regression
uses both parents in one schema/provider/Broker and checks cumulative observer
rows; separate fixtures per width would not expose this collision.

The first remainder preparation also caught duplicate observer instrumentation,
now fixed by observing once in the selected lane. Its final zero-inference
preparation a473dbd/snapshot123243-483 passed268 names/94 top-level,0 fail/skip,
race/vet. Events SHA256
`b462b0bb31fb9e4b36e8c21286757f96eeb2351d32b9ef97ed586cb9724d0b3a`;
source-manifest SHA256
`90127e416882079826204f99512bd5cab0bb50288d62edb02e3e8a05564dd751`.
Its execute report is under the first isolated root,
`reports/delegation-remainder-high-execute-20261012T053627Z-111806`;
native events SHA256
`f1b15a10d96579c730bb69c13b87f494da66466e1ad2d63d7707ebb3efd4497c`.

New isolated root: `/srv/storage/butler/agents/delegation-remainder-high-v2-20261012`.
New one-use marker:
`/srv/storage/butler/native-acceptance-windows/high-delegation-remainder-v2-20261012-baseline37`.
The original remainder marker remains sealed. Twelve immutable history pins carry37 attempts,17 known/20 unknown usage pairs,
62760 input/1810 output known subtotal. The durable limiter admits only38–39,
at most2 new calls, at the original39 ceiling and a fixed18-minute expiry.
All history is revalidated before every native entry, including after process
reconstruction, and clock expiry repeats after row waits. No spending is refunded
or deadline extended. [Limiter proof](delegation-high-remainder-limiter.md).

The separate runner is `scripts/dev/delegation-remainder-high-lab.sh ROOT prepare`
then one explicitly opted-in `execute`, pinning the clean release and complete
tested source bytes. Worker/Broker builds, all targeted race/vet/private UID guards
and the actual zero-inference routing proof must pass before the execute seal.
It reads only the already approved exact private import reference; all earlier
windows, signed corpora, credentials, main checkout and deployment remain intact.

Preparation and actual remainder results are pending. A successful component
baseline cannot label the missing two native child calls or overlap as passed.
