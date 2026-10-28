# Browser GET evidence in the deployed Core

Continuation of PR #173 / #170. The three successful revision2 native controls
06/07/08 each incurred a publication rejection and an extra web.fetch, because
Core's evidence collector did not recognize the two new GET readers. This is a
text-provenance issue; it does not supply receipts for external mutations.

## Owning source and minimal patch

The current Core executable SHA256
10aecd518d3ca819c737ccfb154138cd0817a8174274bfde31ca08b144f8316e matches
docs/evidence/issue147-current-request-20261025/review/deployment.json. That
owning record identifies commit47f9429cc3a712060eaf01d329e371552cd58bcf.
Its predecessor9974b301 (reported by the dirty build metadata) differs only in
tests and a CI selector, with no production-source difference. This resolves
the provenance uncertainty without replacing Core with the whole current HEAD.

Prepare the exact old base plus the already tested three-file delta from
cb7c937e (one production file and two tests):

```powershell
git archive --format=tar.gz --output=.cache/core-evidence/base-47f9429c.tar.gz 47f9429cc3a712060eaf01d329e371552cd58bcf
git diff --binary --output=.cache/core-evidence/guarded-get-evidence.patch 47f9429c cb7c937e -- internal/research/evidence.go internal/research/evidence_test.go apps/orchestrator/internal/orchestrator/research_test.go
```

Upload these files and scripts/dev/browser-core-evidence-build.sh to the owned
server-browser-get-form-20261028 root. The script verifies both pinned SHA256s,
refuses an existing output bundle, applies the patch after --check and builds
under cached Linux Go1.27.2. Isolated pgvector17/Go race checks exercise current
request Core/Broker composition, stopped-attempt write isolation, GET quote
negatives and actual publication preflight. Required skips fail the build.
No production DB, model or channel is used. Vet and compilation passed. The
candidate binary SHA256 is
0d0c38632dd351455c6989deeb465e1becdb095c31e040f7b468a9cc705a13f0.

The rollout script requires the exact old/new hashes and an idle owner queue.
Only Core is recreated, retaining the existing Broker/browser Compose overlay,
current environment and database. Private environment JSON never enters Git or
model input. Compare actual environment hashes and peer container identities;
roll back to the pinned old binary/environment if startup or comparison fails.
No migrations or provider settings change, and no uncertainty/spending is reset.

## Frozen next native window

Before deployment/first submission, fix exactly three new controls:
browser-baseline-core-evidence-01/02/03. Each uses a new synthetic recipient and
the unchanged ordinary Russian search prompt, owner/session/model/autonomy and
existing execution budgets. Keep all earlier controls/FAIL/UNKNOWN records.
There is no retry to replace a failed sample. These are extra controls, not
replacement slots in the original15-case matrix.

Qualification requires each exact fresh record in its protected publication,
current-run successful browser observations and actual Telegram sent status,
with zero in-run owner intervention. Measure publication validation failures
and redundant web.fetch calls separately from browser calls.

Preselected performance goals against source1b6067bf controls06/07/08:

- no publication rejection/refetch solely because guarded GET evidence is absent;
- median Telegram latency <=110s (prior124.516s);
- median model attempts <=14 (prior17);
- median input tokens <=160000 (prior194795).

Monetary cost remains unavailable. These goals do not raise existing caps or
justify account/model changes. Refresh existing OAuth privately if required
before the window; do not inject tool hints or credentials into the prompt.
This small sequential comparison cannot establish p95 or causal speedup across
variable provider/queue conditions. Report successes, failures and unmet goals.

POST/login/consent remain unaccepted. A minimal Core evidence-reader update
cannot reconcile the retained opaque nav-02 effect or authorize new writes.
