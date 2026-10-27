# Independent stream closeout follow-up v2

Both concrete source blockers from public-stream-wiring-review-v1.md are
addressed in the inspected dirty source. No additional wiring blocker found.
This is source review plus read-only public-fixture validation, not a clean
semantic freeze, proof that pending tests passed, hidden authorship or native
admission.

Post-seal logic now preserves the actual trace/index/inventory first, then
checks quiescence, sealed state, failed/drop/open counters and shared GLOBAL.
An unavailable seal sets GLOBAL and calls t.Error, so the actual last t.Run
cannot remain successful. Native closeout additionally rejects shared GLOBAL,
expired context or a missing original arm after the entire loop. Its index
still retains failed actual arms; no retry, omission or denominator repair is
performed. The new public subprocess regression executes the real closeout with
an exclusive segment collision and requires an actual failed helper process,
retained failed observer index/inventory and unchanged collision bytes. Parent
will run that regression; checker did not invoke it or another PG/build gate.

The public history boundary now selects sealed local-r2/405 when present,
verifies its complete pinned 318-file raw inventory, exact pinned manifest and
closed admitted29 window, and performs the same full405 JSONB row equality plus
tamper/truncation negatives. Earlier branches remain for their original closed
histories. No database state is rewritten to satisfy the gate.

Independent pinned v2a reader also validated both actual new public Core/Input/
Broker/PostgreSQL scripted-fixture outputs without issuing PG/model requests:

| Public case | Events | Paired spans | Segments | Verified inventory files |
| --- | ---: | ---: | ---: | ---: |
| unknown | 454 | 227 | 1 | 56 |
| next-success | 752 | 376 | 1 | 56 |

Both traces have actual independently matched schema/protocol identity, zero
observer errors/open/drops, and exact segment/raw hashes and bytes. This checks
representation and quiescent publication of the original UNKNOWN followed by a
separate fixture; it does not count UNKNOWN as quality success. Crossproof is
`public-real-fixture-stream-crossproof-v1.json`, SHA256
`5c79fa1b4bc5eb77e926bff4aafd631309aad754e3557e4e0b83cee39bab8998`.

Reviewed source SHA256:

- behavior76_fresh_test.go:
  `586b253c2e8bfeb342f9bf50e60b68755123fb40a6522d5ce4af531581a7a655`
- behavior76_local_study_test.go:
  `bdf5a4b5b62d8ca05e4d91a4fa17457c5cf9058c81164cc2702bb8ee12ce5058`
- behavior76_fixture_test.go:
  `3066ea28eb8e82c36e5e80bc640943d1d4a5a20460ba784d1971a77b21de7cc2`
- behavior76_authority_stream_test.go (including subprocess regression):
  `c954a73765811238258535c7a0eb981c108ce4357dcdde8d5538e15874068792`

Existing fixed criteria/old scorers, original closed failures/accounting and
masked quality remain unchanged. No hidden-v5 controls or mappings were read.
