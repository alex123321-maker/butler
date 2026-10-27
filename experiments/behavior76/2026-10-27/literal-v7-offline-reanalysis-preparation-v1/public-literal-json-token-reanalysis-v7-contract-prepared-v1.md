# Literal v7 offline representation reanalysis — PREPARED_ONLY

This is a separately named post hoc JSON representation boundary, not a new native protocol or permission to rerun the measured experiment. Original frozen v6 execution remains STRICT FAIL/NO_GO exactly as recorded in receipt b4bfa23b…; its private diagnostic and public redacted diagnosis remain unchanged. R3 candidate rejection remains unchanged. No actual r1 response, communication judgment or mapping is read by preparing/testing this fix.

## Exact intervention and scope

The new runtime is literal-json-token-boundary-reanalysis-v7.ps1. Its future entrypoint Invoke-Issue76LiteralOfflineReanalysisFilesV7 first defaults to PREPARED_ONLY_NO_EVALUATION. A later reviewed invocation must explicitly opt into ExecuteReviewedOfflineReanalysis and provide the exact original-failure receipt hash/window. This document and the runtime do not provide that later authorization.

Inside Invoke-Issue76JsonStringTokenScopeV7, the installed Microsoft.PowerShell.Utility ConvertFrom-Json cmdlet must support the String value of DateKind. The observed local runtime is PowerShell7.6.5. A fresh local copy of PSDefaultParameterValues sets only:

    ConvertFrom-Json:DateKind = String
    Microsoft.PowerShell.Utility\ConvertFrom-Json:DateKind = String

This affects ALL matching nested ConvertFrom-Json calls during the invocation: manifest/index/snapshot/table/native-proof/ingress/golden parsing and the old nested expected-value JSON copy, not merely the private expectation loader. The scope is deliberately stated broadly. It does not change the C# trace reader's own System.Text.Json implementation. Existing explicit non-DateKind arguments and caller behavior, including root-array pipeline enumeration, stay unchanged. NoEnumerate is used only in public scalar/array test containers, never installed as a new old-reader default.

The actual cmdlet receives DateKind String before parsing. There is no DateTime.ToString, culture-dependent format, timestamp backfill or reconstruction after information loss. Decoded JSON string content, offset spelling and fractional digits remain the exact same UTF8 bytes; ordinary JSON escape decoding is unchanged and is not claimed to preserve wire escaping. Null/boolean/number/object/array value kinds remain JSON kinds. Exact explicit-key/duplicate/alias checks remain the old ones. A disabled parameter-default environment, shadowed command or missing capability fails before future private reads.

The default table is local and cloned, not a mutation of caller/global defaults. Public tests prove restored caller behavior after success and throw, preservation of unrelated caller defaults, and no callback execution under rejected capability/default/shadow conditions.

## Unchanged analysis and evidence

All original eleven runtime files stay byte-exact: seven prior PS/C# dependencies plus four literal v6 helpers. The new scope delegates directly to the identical Invoke-Issue76LiteralStreamScoringFilesV6 entrypoint and exact old25 per-case predicate types. All27 original statement AST hashes and12 original helper-function AST hashes are identical before/inside the scope and to the formal source witness. The two old24/48/96 global extents remain old witnesses; literal16/32/68/8/6 cardinality is unchanged. No predicate, applicability, threshold, rubric, stop rule, UNKNOWN treatment or model claim changes.

The future result is wrapped in a NEW issue76-literal-offline-reanalysis-v7 envelope containing original_execution_status=FROZEN_V6_STRICT_FAILURE_RETAINED and posthoc_representation_reanalysis=true. Its inner result preserves the original v6 result version; the envelope never relabels the failed original execution as PASS. A fresh output path and a separately frozen reanalysis-input plan must retain raw2553files, corpus/golden hashes,13/1134/1122/9,010,959/12NULL accounting, all local/global faults, automatic work, kernel/model provenance,68 planned rows and source/effect/clock/authority proofs. No new native window, quota use, source repair, receipt fabrication or resampling is authorized.

## Public proof and limitations

Only generic public2041/2042 ISO examples reproduce the original default JSON-string-to-DateTime conversion and ensuing String.Contains type error. DateKind String preserves the same literal text and the same containment expression succeeds without editing it. Generic tests cover scalar/object/array trees, nested strings/offsets/nanosecond spelling/Unicode/JSON escapes, duplicate/case checks, the exact old ReadJson function and old literal reader, caller scope cleanup, and AST equality. Original public regression scripts and the retained actual Go-to-PS public exports are exercised separately under the proposed scope; their completed receipts, including any failure, must be supplied before review. No public test result is an actual-r1 evaluation.

The intervention is POST HOC after a real frozen checker failure. Public plumbing/negative-regression success cannot prove that every unseen interaction is unaffected. It may change old implicit timestamp normalization wherever the old PowerShell parser misrepresented a JSON string. That is why all matching decoder calls are declared, the failed execution remains, new dependency/entrypoint pins are separate, and parent review must precede a frozen offline reanalysis. Any later strict failure is retained; no private expected value is cast/edited to make a predicate pass. Masked judgments must still seal BEFORE separately authorized mapping disclosure. No candidate promotion or original-protocol success follows.
