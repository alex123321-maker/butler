# Retained logical capture diagnosis

This standard-library-only Go command audits a closed #87 v5 review packet.
Run it with the pinned cached Go toolchain, offline, after producing
`review-cases.jsonl` with `memory-adaptive-review.py`:

```sh
go build -o /private/diagnosis scripts/dev/memory-adaptive-capture/main.go
/private/diagnosis /srv/storage/butler/agents/issue87-native-v5b-20261018/reports/native-v5
```

It creates `native-capture-diagnosis.json` exclusively and never overwrites an
original. Exact RawMessage hashes, original expected hashes, frozen typed
roundtrip hashes and a separate provenance-struct hypothesis remain distinct.
The copied structs deliberately match the frozen v5 transport shape; no provider
frames, auth, hidden reasoning or network access are used. Hypothesis agreement
does not turn an original capture mismatch into qualification.
