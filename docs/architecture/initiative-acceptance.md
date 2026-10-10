# Initiative acceptance evidence

Validation used the isolated lab on 2026-10-11 (local time), Go1.27.2, separate
disposable PostgreSQL schemas and real Broker/executor subprocesses. It did not
change production PostgreSQL, send owner messages or use Google credentials.

`TestInitiativeProjectChecksRealBrokerE2E` passed with `GOFLAGS=-race` in snapshot
`20261011-060903-795`. Three ordinary tasks admitted under an authenticated owner
goal/mandate each performed an exactly approved GET on one synthetic project.
The actual Broker authorization, effect journal and Transcript Store supplied
the observer facts; no failed run or tool journal was fabricated. A second GET
within the first run did not count as an independent failure.

The third distinct check qualified one initiative. Promotion atomically created
an ordinary task plus ordered input under the current mandate and finite model
preflight. The fixture driver then used normal exact-approval Broker APIs for
that task: GET project, POST immutable patch, GET stored patch checks. The POST
is a commit to a recipient even though the original project is left unapplied;
its matching recipient receipt confirmed the effect. The prepared patch passed
its objective checks, and `applied=false` was retained. Only an owner-verified
Task Service result completed the initiative. Tool success alone left it in
`promoted`. The task's own failed-project observation was suppressed, and
repeated digest queueing produced one count-only notice.

This fixture proves admission, transport, effect and verified-result boundaries;
it does not claim live generative planning or automated task verification. Its
public synthetic recipient has no private source lifecycle records. The draft
production observer fails closed until Catalog's current-source hooks are wired.
Source revocation tests and final production wiring must be checked with the
versioned-memory integration before deployment or issue acceptance.

Earlier actual PostgreSQL/race initiative suites cover durable schedule claims,
timezone/DST behavior, downtime skip/coalescing, concurrent promotion, finite
attention/model preflight, immutable revision-bound digests, uncertain delivery,
owner changes and late current-condition/source guard rejection. Recorded-journal
adversarial tests separately cover duplicate JSON, stale task/binding/resource,
unconfirmed effect, missing live guard and revocation after budget contention.
Those synthetic-journal parser tests are distinct from the real-process proof.

Run `scripts/dev/lab-extensions.sh` inside a read-only lab source snapshot with
an isolated `*_test` actionauth database and
`BUTLER_EXTENSION_E2E_TEST_REGEX=^TestInitiativeProjectChecksRealBrokerE2E$`.
Use a dedicated cache output directory for the three built binaries. Source
configuration never launches a network request; its URL only selects an already
approved, current Broker observation.
