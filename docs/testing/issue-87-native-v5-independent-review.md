# Full adaptive v5 comparison: NO_GO

Root inspected all96 fixed questions, rubrics and original delivered outcomes
after collection closed. There are95 completed originals, one failed static
original and no unexecuted slots. No reply, rubric, failure, charge or expected
hash was replaced. This is a retrospective diagnostic content review; it does
not qualify the adaptive policy or claim human annotation.

Source: `2bdbeb463c38ebc16bdd2a10402481908accf1c9`.
The prospective96-slot corpus/order/rule are unchanged. Actual main auth returned
by exact CAS, actual paid discovery returned200 and a fresh private snapshot was
saved before the common auth lock was released. Main application deployment and
production qualification flags were unchanged.

| Arm | Scheduled | Completed | Correct | Partial | Incorrect | Semantic unknown |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| No optional memory |32|32|11|20|1|0|
| Static retrieval |32|31|27|4|0|1|
| Adaptive retrieval |32|32|12|19|1|0|

| Fixed group | No optional memory | Static | Adaptive |
| --- | ---: | ---: | ---: |
| Self-contained8 |8|8|8|
| Historical memory16 |0|12|1|
| Uncertainty8 |3|7|3|

Correct counts require the requested facts or sufficient explanation of genuine
uncertainty. A context-correct refusal is incomplete when the permitted history
contains the answer. The single failed static original, t01, has a typed
`transport_connection_error` and unknown usage; no semantic judgment is invented.
Completed-only correctness is therefore11/32,27/31 and12/32 respectively.

The adaptive arm created32 real plans, but31 chose `none` and only e01 chose
`catalog_exact`. That original performed real search/open and correctly answered
from the actually provided68-character source. The other exact-ID, cross-session,
multi-hop and event-time questions usually received no optional memory. All eight
self-contained adaptive questions answered correctly with zero optional reads.
The failure is usefulness and strategy selection, not evidence of a general
provider inability to read or use Broker tools.

Static retrieval correctly supplies the12 exact/cross-session/multi-hop answers.
Its completed temporal answers honestly report missing source event-time metadata:
the actual selected document text is present, but its dates are not attached in
the model input. Those three refusals remain incomplete relative to the corpus.
The c03 no-memory and adaptive answers substitute unsupported general maintenance
procedures and omit the agreed dry case; a coincidental battery-removal suggestion
does not establish grounded recovery.

Static u03 identifies both equal conflicting16/61 records and selects no winner,
but does not request or functionally identify the required clarification. It is
partial, grounded and uncertainty-correct. A separate reviewer, temporal88_v6,
independently agreed with that distinction; this is not a canonical-phrase test.
The same reviewer agreed that e01's exact opened quote entails its claim.

The reviewer verified all96 safe visible-input artifact hashes against the packet.
Each annotation binds original input/output and visible-artifact hashes and
retains original-source quote witnesses with separate actual-provision flags.
Self-contained answers use the exact current question witness. The
[machine-readable review](fixtures/issue87/native-v5-independent-review.json)
contains all96 annotations; no original packet field was edited.

## Evidence integrity and accounting

All96 original InputSHA and visible-text aggregate correlations match. The
retained CaptureSHA recomputation does not match any of the96 expected hashes.
A subsequent offline red/green collector regression proves that decoding JSON
back into a nonempty pointer slice mutates retained provenance metadata types.
Restoring only the frozen `content.Origin` dynamic type reproduces all96 expected
hashes diagnostically. Original expected/recomputed artifacts stay unchanged;
that explanation is not a replacement capture or a promotion receipt.

There are98 actual native entries:32 no-memory,32 static and34 adaptive. Native
full usage is known for97 and unknown for one. Known totals are66781,142784 and
84985 tokens respectively,294550 combined, including135638 input and4716 visible
output;154196 additional tokens remain in the reported totals. Static's total is
a lower bound. The predeclared full-known paired saving branch cannot qualify,
and adaptive correctness is inferior independently of any cost comparison.

The owning ledger retains226 model quotes:98 cloud and128 actual local operations
(96 indexing and32 static queries), plus2 read-tool quotes. Its conservative
resource vector is106741760 tokens,37320000 active milliseconds,822181710 provider
input bytes and8388608 egress bytes; external calls2. Owner-declared zero
incremental subscription billing is not an invoice or zero hardware cost.
No quote is refunded and NULL is not converted to zero.

One original native envelope has one adaptive citation. Root checked its exact
provided quote, read_ref, current source revision/access/hash, quote offset and
confirmed opened receipt; semantic entailment is correct. Generic delivered-text
JSON citation parsing remains a separate unknown dimension. No other claimed
adaptive citation is manufactured from this sample. Observed actions are the
two authorized reads; this small contour does not prove all adversarial behavior.

Reports: `/srv/storage/butler/agents/issue87-native-v5b-20261018/reports/native-v5`.

| Artifact | SHA256 |
| --- | --- |
|review-cases.jsonl|4a0a85cbd89d63432dc93b1cb27e28efb7c3fd68086e43bfe8373c42f3dd831c|
|review-ready-manifest.json|591c4485eb85d99a2ec3ee91a581aab37e3ee71f48df808c30f7fb9bac548d4e|
|full-ledger.json|c51b06b2a04ce92d6e901ef59e35584715c4d128e0e35f85ab32f46b84f724a4|
|strategy-accounting.json|34d50a1507de92b458b127cbaff023ae5378084060bd989af8cd0a99d4f2e4ea|
|native-capture-diagnosis.json|406adbba9b36c7e9098dee692c0bb9d0c7cc2475b13f5c8c9890e82feeaff83c|
|Repository independent-review JSON|593cedc0e5914fc9f596b64e06531a4c30a3dbf61daf6635789847e611174a83|

The declared adoption rule fails overall, historical-memory and uncertainty
noninferiority. Keep the adaptive release gate false. Issue87 permits this
measured rejection; a policy redesign is a separate candidate. These exposed
public cases cannot become an untouched holdout after a routing change. Any
future benefit claim needs a new independently frozen sample and retained old
failures/costs, alongside the ordinary lifecycle/privacy gates.
