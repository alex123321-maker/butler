# Closed local-r2 attempt: NO_GO

One prospective admitted window ran locally from producer
`e5d133d9620928dba567ca333d84825768dffec7`, collector SHA256
`aa693b65434e324853a16c6f5f147ba142675645aadd5b5ebdf9d8a7062a6e7a`.
The independent admission is `prospective-admission.json`; the independent
boundary audit is `independent-boundary.json`. No new inference was performed
to produce this report, and no mask or quality judgment was opened.

| Actual collection | Result |
| --- | --- |
| Original arms started |5/48 |
| Recorded owner outcomes |10/96 |
| Represented families |2/12 |
| Paired episode slots |2 |
| Native admissions |29 |
| Known usage observations |28 |
| Known provider-total tokens |216896 |
| Unavailable usage observations |1 |
| Local UNKNOWN stop records |0 |

The fifth fixture reached the flat recorder's65536-event bound. Its actual
trace retains65536 events,8 dropped,observer_failed=true,0 open spans and0
operation failures. The first dropped event's exact timestamp is unavailable.
The shared global stop cancelled already admitted entry29, which retained
run_started and transport_error only, no final/text/tool proposal and NULL
usage. There was no entry30. No750ms watchdog warning occurred in this window;
this does not resolve the historical observation-latency cause.

The full comparison is incomplete. Neither candidate quality nor the required
12-family comparison is established, and issue76 remains open. UNKNOWN/failed
and unavailable evidence is not reclassified or refunded. All prior376 JSONB
attempt rows remain identical; inclusive history now has5 closed windows,
405 admissions,400 measured responses,3084472 known tokens and5 unavailable
usage observations.

`raw-evidence.zip` contains all318 exact raw files (157207406 uncompressed
bytes); every archive entry was independently byte-verified. Archive SHA256:
`b18ceeeb4ccf24a8c18b626a3c3a1fae26c942c3cd83577eca188e2cde43f212`.
`files.json` and `raw-inventory.json` preserve the exact complete file inventory
in reporting and collector-guard encodings respectively. The original raw
files were marked readonly after the independent initial boundary audit.
A separate private full control-database dump was preserved locally; its list
was verified with pg_restore, which is not a restore test. It is excluded from
this public evidence.

The next streamed-recorder study requires a separate prospective protocol,
fresh independent controls and new admission. It cannot resume this window or
rewrite its observer counters, usage, protocol, raw files or judgments.
