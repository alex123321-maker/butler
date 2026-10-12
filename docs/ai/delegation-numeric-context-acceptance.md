# Known numeric delegation output format

Issue16's typed numeric acceptance previously checked a six-field final without
requesting that schema in the trusted child prompt. The strict contract body is
only a JSON kind and source reference; it cannot carry free-form instructions.
The new owned adapter recognizes that known marker under current Catalog proof,
requires its exact witness in the selected roots and supplies a typed pin to core.

Core compares the pin to the current selected DATA and known marker before adding
one fixed kernel instruction: JSON only, no Markdown, exactly source_id,
source_revision, source_hash, observed_value (signed int64), observed_date (valid
YYYY-MM-DD) and uncertainty=source_only. Only the quoted exact witness reference
is interpolated. Body, title, original values and arbitrary candidate prose stay
DATA. The instruction's sole origin reference is the fixed kernel schema, so it
does not elevate a selected source. Permissions and independent acceptance remain
the ordinary owning contract/Broker/owner boundaries.

Recovery removes any saved instruction for this schema and rebuilds it from the
fresh contract/witness proof under the new lease. Saved references cannot choose
an extra witness, revive a deleted source or alter field meanings. Existing tool
and unknown-effect observations remain intact; narrative handoffs retain their
existing behavior.

The ordinary input-worker tests use real disposable PostgreSQL, pool2, Task,
Session, input, Catalog and completion stores. Their provider is a scripted
input observer, never a native inference or independent verifier. It checks the
actual fixed kernel item and current private witness DATA, zero child tools,
no inherited provider session, no parent transcript/ancestor body and no ambient
memory query. Deleted/corrected/unselected witnesses and Unicode contract aliases
must deny before StartRun. A fresh adapter/new lease reloads the exact current
contract; the superseded token and deletion deny. Core tests also rebuild a
tampered saved schema instruction while retaining separate unknown effect DATA.

Validation runs only in `/srv/storage/butler/agents/delegation-numeric-context`;
all caches, temporary files, disposable PostgreSQL and reports are root-local.
Main laba, private credentials, the signed native corpus and sealed historical
windows are unchanged. Actual native High width1/width2 conflict and independent
accept/reject acceptance remain root's separate fixture. New model calls:0.

Code snapshot `20261012-110503-864` passed the narrow numeric-context/core and
actual PG provider-entry/reconstruction checks:18 successful tests/subtests,
0 failures/skips, race and affected vet. Evidence:
`reports/narrow-20261012T040512Z-4003846/events.jsonl`, SHA256
`3344891a1510abb534db9d6d268d2e378c0be34a81273887c7be8ab1f0609e76`.

The same code snapshot passed the wider delegation selection across core,
Orchestrator app, Task, delegation parser and owner API:207 successful
tests/subtests, 0 failures/skips, race and affected vet. The app suite included
the separately compiled actual Broker process and current admission/source/stop
boundaries; the Broker subprocess binary itself was compiled without race.
Evidence: `reports/broad-20261012T040642Z-4010010/events.jsonl`, SHA256
`37a6234819b0072f09d2188741b845dfbb8d598c6e391a2f3a24371a3b32908c`.
Both `vet.log` files are empty, SHA256
`e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

The first preparation failed because the new parser/fixture initially read the
outer content envelope instead of its `data` member; the correction fixture also
omitted its expected source revision. Those failed reports remain retained at
`reports/narrow-20261012T040251Z-3994654/`. An earlier cold-cache preparation
could not download modules on the private network and is likewise retained.
Neither failed preparation is model evidence. No production guard was relaxed.

Commands were a source Sync through `scripts/dev/lab.ps1`, then the retained
agent-local `numeric-context-lab.sh narrow` and `broad`. That runner mounts the
source read-only, supplies the disposable `_test` PG URL, rejects skips/failures,
uses only this root's caches and removes its containers/network after each run.
