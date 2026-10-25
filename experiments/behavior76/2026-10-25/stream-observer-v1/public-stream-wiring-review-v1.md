# Independent stream wiring review, public dirty source v1

Verdict: two concrete closeout/gate issues require correction before semantic
freeze. Parent has acknowledged the first; no native/hidden admission is given.
No PG, compilation, model, hidden controls, mappings or quality review performed.

1. `behavior76_local_study_test.go` sealFixtureEvidence serializes sealStream's
   returned failure state but does not itself report a failed stream seal to the
   test. `behavior76_fresh_test.go` native defer then appends the indexed arm.
   After the last t.Run the collector has no terminal shared-GLOBAL assertion.
   A final-arm segment/header seal failure could therefore retain a full index
   and let the test return normally. The independent reader would reject that
   trace, but collector PASS must not hide the fault. Preserve the actual raw
   failure/index and make failed post-seal state fail the native collector,
   including its final arm. Composition already has an explicit post-seal check.
2. TestBehavior76FreshHistoryBoundaryIntegration chooses at most the sealed
   fresh-v3 376-row export, then FULL JOINs the entire control table. With the
   current exact closed 405-row history this required public gate will fail.
   Add the sealed local-r2/405 selector with exact raw/manifest identity, while
   retaining previous branches for their original history. The new native
   admission itself already uses the correct 405-row predecessor export and
   aggregate counters; do not modify any history to satisfy the older gate.

Other reviewed paths preserve the contract: new Stream* entrypoints are explicit,
native requires EnableNative, UNBOUND hashes reject before credential/model
access, old functions select streaming=false, stream starts before seed, actual
quiesce failure keeps streamQuiescent=false, backup/export precede stream seal,
and only classified original local UNKNOWN skips the owning fixture's tail.
All segments are included in the existing exact sealed case inventory. Old
flat-v1/scorer/adapter files remain unchanged. No synthetic v1 evidence is made.

During this review parent additionally introduced an explicit trusted
newBehavior76StreamFixture path: it chooses the new v5 protocol file before
binding scope identity, whereas original wrappers choose the old protocol.
The new native entry uses that constructor. This corrects the previously fixed
old-document read without allowing model/corpus data to choose the protocol.

Reviewed dirty source pins (subsequent fixes need their own source pin):

| File | SHA256 |
| --- | --- |
| behavior76_fresh_test.go | 006d2ea65f4fce313ea9abc44e2bdba6b283902d16444dbe9fb19b7ae2a6317b |
| behavior76_local_study_test.go | fbff52d58d87015155c7df1062947b63328c7b232586f633d10d7d95cfa78a6f |
| behavior76_fixture_test.go | 3066ea28eb8e82c36e5e80bc640943d1d4a5a20460ba784d1971a77b21de7cc2 |
| issue76-fixture-stream-protocol.md | 3e573f54cf61fe2ddd28066c225e55421b47cd0b4cc17d66e36bca9116a57cb5 |

The prior actual Go-to-reader public crossproof remains valid and immutable.
Neither this source review nor a scripted fixture establishes language quality,
historical authority-latency repair or completeness of the closed studies.
