# Exact additive extraction delta

No old runtime/scorer file is patched. The v1 extractor and failure remain unchanged. V2 is a new helper plus separately named, nonexecuting-by-default driver.

V1 unconditionally decoded a selected final and each automatic final:

    ConvertFrom-Json $f[0].content -AsHashtable -Depth 100

V2 sends the selected owner final through Get-Issue76MaskedOwnerFinalV2 with the already frozen owner row, semantic row, mechanical proof checks and actual run identity. That helper first retains exact raw/SHA privately. It only returns a decoded answer when whole-document syntax AND the unique existing mechanical availability/provenance proof agree. Otherwise decoded answer is null and communication is UNAVAILABLE. Automatic finals are retained as separate exact raw/SHA records, never owner answers. Kernel results retain their separate existing path.

The new driver keeps the original envelope/golden/snapshot/effect/source projection and pinned v7 JSON string scope. It uses fresh masked-evidence-v2 output, retains all 68 evaluated opportunities, records an execution receipt, and never calls mechanical scoring. Its default mode returns PREPARED_ONLY before loading actual data. The explicit switch has not been invoked.

The successful mechanical result is consumed byte-exact; it is neither repaired nor rerun. Conservative extraction unavailability is recorded separately from original mechanical verdicts and never overwrites them. No raw text, first object or repaired JSON is graded as a valid model answer.
