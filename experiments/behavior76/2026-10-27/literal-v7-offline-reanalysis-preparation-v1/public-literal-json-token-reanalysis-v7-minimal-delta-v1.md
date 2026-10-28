# Exact delta — additive JSON token boundary, not an old-file patch

Old eleven runtime files: zero changed bytes, verified against the existing v6 dependency pins. Source predicates:27 statement hashes and12 helper hashes unchanged. Corpus/goldens/raw/model outputs: untouched and not evaluated here.

One new runtime file defines a local parameter-binding scope. The substantive decoding intervention is the following two assignments to a CLONED LOCAL default table:

    $local:PSDefaultParameterValues['ConvertFrom-Json:DateKind']='String'
    $local:PSDefaultParameterValues['Microsoft.PowerShell.Utility\ConvertFrom-Json:DateKind']='String'

The new wrapper checks the installed cmdlet/capability and a generic public token probe, then invokes the same old v6 entrypoint with the same arguments inside that scope. All matching conversions within it are affected. Returning/leaving via throw discards the local default table; no caller/global default is changed. Remaining new code validates old runtime pins/original failed-execution identity and labels the separately named post hoc output. It does not define new success/failure predicates or recast an already converted DateTime.

The actual complete new-file bytes and SHA256 belong in the prepared dependency receipt. This is not presented as a two-line patch to the old frozen sources: there is deliberately no old-file diff, and the new guarded wrapper is explicit and separately versioned. Generic/public regression scripts and their outputs are test artifacts, not additional production or model dependencies. No actual r1 scoring call is made until a later explicit reviewed instruction.
