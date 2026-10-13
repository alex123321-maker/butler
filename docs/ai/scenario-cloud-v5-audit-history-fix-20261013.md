# V5 original audit denial and historical request identity

The frozen3c4089 native collector completed252 requests on the original signed
corpus and recorded a passing quality/comparison verdict. Independent review
with its frozen193fea auditor failed before original-byte/SQL correlation. That
failure remains evidence: `audit-final.json` is an exclusive0600 empty file,
SHA e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855.
It is not a completed audit and cannot authorize the release chain.

A separate error-only offline diagnostic isolated the rejection to the old
historical RunID uniqueness assertion in `AuditCloudV5Window`. Independent
recount of the exact already pinned283-row history found nine repeated RunID
groups containing27 requests. Start/Continue/Submit requests legitimately share
a RunID. The history has279 distinct nonempty reservation IDs; the separate four
dialect diagnostic entries have no reservation ID. Every original row remains
present, with257 known visible pairs,26 NULL pairs,162673 input/6484 output
subtotal,289855 known total tokens and46 unknown historical totals.

The repair changes only this offline assertion after exact frozen-history
equality. Historical RunIDs form a set forbidding collisions from any new
context; every historical row still contributes its original nullable charge.
All252 new ordinal/context/role/order/parent checks, signature, source/runtime,
original byte, PostgreSQL, cost and quality gates remain unchanged. No corpus,
golden, response, declared resource or threshold changes. Original266 stays
UNKNOWN. The new regression first failed on the original implementation, then
passed alongside history and journal tamper tests; it separately rejects changed
or omitted charges, a filled NULL and a new context colliding with a repeated
historical RunID.

The original collector source, runtime, report and sealed window remain untouched.
A separately committed read-only auditor is prepared without credentials or
network through `cloud-v5-readonly-audit-lab.sh` in its own isolated root. Its
source/executable/test hashes must be independently checked before reading the
same paid originals. A fresh exclusive `audit-reviewed.json` records that audit;
the original failed artifact is never overwritten. This correction supplies no
new inference, retry, refund, chain permission or issue33 readiness by itself.
