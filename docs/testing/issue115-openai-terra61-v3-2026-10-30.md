# GPT-6.1 Terra on the identical V3 A/B/C corpus

The requested `gpt-6.1-terra` is tested through the existing OpenAI Responses
provider, real Core, PostgreSQL, Tool Broker, memory and reviewed skills on laba.
The native source is `e4835ce1cfced57454f064b67992ef25f5effcc9`; the new private root is
`/srv/gitea/butler-acceptance/issue115-utility-native-openai-terra-20261030-r1`.
Results and receipts are published in
[the evidence directory](../../experiments/memory115/2026-10-30/openai-terra61-v3-v1/).

The exact 24 cases and 72 Latin-order positions reuse the earlier Gemini/Luna V3
suite bytes, SHA256 `8fef1a32d521882cc751c595261dc9dc6ae1ff75e48ef239c77bb86ad4036919`.
Both closed comparisons and historical unknown spending are pinned unchanged.
Instructions, source states, scorer, read-stop policy, tool contracts and serial
execution are not tuned for Terra. Tasks, sessions, schemas and journal state are
fresh. A has no past experience; B has notes; C has the same notes and the reviewed
skill. Preparation is scripted with zero native creation calls; manual authoring
and autonomous learning costs remain unmeasured.

High, Standard tier, 65536 input/8192 output tokens, 90 seconds per request,
13 methods per variant, 420-second authority, 480-second case and four-hour global
deadlines remain fixed. The only profile differences are the requested model and
conservative monetary quotes: input/output 3/5 USD micros per token and five USD
per isolated task/mandate/conversation scope. These rounded admission ceilings
do not add actions, tokens or time and are not measured API prices.

The fresh controlled root
`/srv/gitea/butler-acceptance/issue115-utility-trace-openai-terra-20261030-r1`
passed all 72 controlled positions, real Broker skill approval/load and the new
explicit-model/conservative-price allowlist test, with no required skips.
All 6797 Git blobs were verified, accounting for declared Windows CRLF exports.
Unchanged production provider/config/Core/usage checks retain their explicit
provenance: 92 provider/config race tests and vet from c5009153, and the earlier
22 PG usage and 325 transport checks, with eight separate live/external skips.
The source, binary, plan and actual cgroup limits are bound before key input.

The previously authorized API key is reused through private SSH stdin, with no
persistent secret copy on laba or inherited runtime credential. Read-only model
access returned 200 before generation. No production deployment is included.

Read-only reporting correlates raw methods with the owning PostgreSQL and Core
operation/run/reservation records. Prices use the published Standard short-context
Terra rates: USD 1/M ordinary input, 0.05/M cached input, 1.25/M 30-minute writes,
2.50/M 12-hour writes and 5/M full output. Unknown cache lifetimes preserve a range;
unknown usage is not priced as zero. Reasoning is already in output; cache counts
are input subsets. These are tariff estimates, not invoices/account adjustments.
See [Terra](https://developers.openai.com/api/docs/models/gpt-6.1-terra),
[pricing](https://developers.openai.com/api/docs/pricing), and
[cache accounting](https://developers.openai.com/api/docs/guides/prompt-caching).

This is one public development corpus, not an independent holdout or autonomous
learning acceptance. Strict source/effect/field failures remain primary failures;
posthoc exact-value or artifact diagnostics never change the original score.

## Completed result

All 72 scheduled positions completed; Terra's strict A/B/C results are
**16/24, 22/24, 21/24**, totaling **59/72**. The unchanged earlier results are
Gemini **7/24, 20/24, 20/24** (47/72) and Luna **7/24, 8/24, 5/24** (20/72).
Within Terra, notes add six successes over A; the reviewed skill adds no primary
benefit over notes. All twenty factual cases pass in both B and C, compared with
fourteen in A. The remaining B/C failures concern action-result fields, including
`ready=true` where the frozen contract expects `ready`, despite physically
preparing and checking the correct unapplied artifact in every arm.

Terra makes 47 memory searches (12 `ok`, 35 `empty`), opens all twelve successful
results, and loads the reviewed skill 21 times through Broker. C costs 43.7%
more full tokens and 113.9% more execution time than B without improving the
primary score. Memory has demonstrated utility here; the current skill and
lexical-query interface still require improvement. These are descriptive corpus
observations, not generalized claims about every model, memory or skill.

The 221 dispatched methods total 1167881 full tokens, all reconciled with raw
PostgreSQL and Core records: zero unknown usage, transport failures, missing Core
attempts, token mismatches or rejected source projections. Execution sums to
18.83 minutes, or 21.72 including measured preparation; unmeasured authoring and
some setup/observer overhead remain excluded. Producer exit zero means the
window completed technically, not that all tasks or product acceptance passed.

The known-usage Standard tariff estimate is **USD 0.63199945–1.10074195**, versus
Luna's **0.083172960–0.12937046**. Cache-write lifetime uncertainty remains in
both ranges; neither is an invoice. Gemini's subscription allocation remains
unknown. All eleven historical receipt hashes were reverified without rewriting
previous results or adding overlapping spending. The isolated runner/network
were removed, PostgreSQL stopped cleanly, and its private data retained.
