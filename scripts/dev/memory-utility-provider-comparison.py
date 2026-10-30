#!/usr/bin/env python3
"""Offline, read-only research reporting; Python is stdlib artifact glue, not Core.

Compare an exact paired V3 corpus without rescoring or changing previous receipts.
API tariff estimates are distinct from invoices and subscription allocation.
"""
import argparse
import collections
import decimal
import hashlib
import json
import pathlib


def read(path):
    return json.loads(path.read_text())


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def cost(usage):
    """USD at published Standard short-context rates; unknown subsets stay bounded."""
    D = decimal.Decimal
    if usage is None:
        return None
    i, o = usage['InputTokens'], usage['OutputTokens']
    assert 0 <= i <= 272000 and i + o == usage['TotalTokens']
    cached = usage.get('CachedInputTokens')
    writes = usage.get('CacheWriteInputTokens')
    short = usage.get('CacheWrite30mTokens')
    long = usage.get('CacheWrite12hTokens')
    if cached is None:
        return [D(i)*D('.01')+D(o)*D('.50'), D(i)*D('.25')+D(o)*D('.50')]
    assert 0 <= cached <= i
    base = D(cached)*D('.01') + D(o)*D('.50')
    if writes is None:
        return [base+D(i-cached)*D('.10'), base+D(i-cached)*D('.25')]
    assert 0 <= writes <= i-cached
    base += D(i-cached-writes)*D('.10')
    if short is not None and long is not None:
        assert short+long == writes
        exact = base+D(short)*D('.125')+D(long)*D('.25')
        return [exact, exact]
    return [base+D(writes)*D('.125'), base+D(writes)*D('.25')]


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('luna',type=pathlib.Path)
    p.add_argument('gemini',type=pathlib.Path)
    p.add_argument('output',type=pathlib.Path)
    args=p.parse_args()
    new,old=(read(x/'public-report.json') for x in (args.luna,args.gemini))
    assert new['model']=='gpt-6-luna'
    assert new['suite_sha256_bytes']==old['suite_sha256_bytes']
    assert new['scheduled']==old['scheduled']==72
    assert new['all_slots_observed'] and old['all_slots_observed']
    suite=read(args.luna/'out/plan/suite.private.json')
    plan=read(args.luna/'out/plan/plan.json')
    cases={slot['Sequence']:suite['cases'][slot['Case']] for slot in plan['Slots']}
    def factual_diagnostics(root,rows):
        out=collections.Counter()
        for row in rows:
            if row['family']=='regression':continue
            expected=cases[row['sequence']]['expected']
            path=root/'out/actual'/('slot-%03d'%row['sequence'])/'snapshot.private.json'
            finals=read(path)['Tables'].get('finals',[])
            if len(finals)!=1:out['missing_single_final']+=1;continue
            try:answer=json.loads(finals[0]['content'])
            except (ValueError,TypeError):out['unparsed_final']+=1;continue
            if not isinstance(answer,dict):out['unparsed_final']+=1;continue
            # Reuse the already sealed, duplicate-safe diagnostic. Never rescore.
            value_match=row.get('answer_value_match_diagnostic_only') is True
            effect_match=answer.get('effect')==expected['effect']
            check_match=answer.get('check')==expected['check']
            out['parsed_factual_finals']+=1
            out['exact_outcome_value_match']+=value_match
            out['effect_mismatch']+=not effect_match
            out['check_mismatch']+=not check_match
            out['correct_value_but_effect_or_check_mismatch']+=value_match and not(effect_match and check_match)
        return dict(out)
    arms={}
    for arm in 'ABC':
        nr=[x for x in new['rows'] if x['arm']==arm]
        before={x['sequence']:x for x in old['rows'] if x['arm']==arm}
        dimensions=collections.Counter(); tiers=collections.Counter()
        totals=collections.Counter(); low=high=decimal.Decimal(0)
        for row in nr:
            prior=before[row['sequence']]
            assert (row['family'],row['condition'])==(prior['family'],prior['condition'])
            dimensions['luna_only_success']+=row['task_success'] and not prior['task_success']
            dimensions['gemini_only_success']+=prior['task_success'] and not row['task_success']
            dimensions['both_success']+=prior['task_success'] and row['task_success']
            dimensions['both_failure']+=not prior['task_success'] and not row['task_success']
            slot=args.luna/'out/actual'/('slot-%03d'%row['sequence'])
            for path in sorted(slot.glob('native-*.private.json')):
                item=read(path)
                if not item['dispatched']:
                    continue
                usage=item['usage']
                estimate=cost(usage)
                if estimate is None:
                    totals['unknown_usage_requests']+=1
                    continue
                low+=estimate[0]; high+=estimate[1]
                totals['known_requests']+=1
                for key in ('CachedInputTokens','CacheWriteInputTokens','CacheWrite30mTokens','CacheWrite12hTokens','OutputReasoningTokens'):
                    if usage.get(key) is None: totals[key+':unreported_requests']+=1
                    else: totals[key]+=usage[key]
                observed={e['ProviderServiceTier'] for e in item['events'] if e.get('ProviderServiceTier')}
                assert len(observed)<=1
                tiers.update(observed or ['unreported'])
        def by_family(rows):
            out={}
            for row in rows:
                group=out.setdefault(row['family'],{'scheduled':0,'task_success':0})
                group['scheduled']+=1;group['task_success']+=row['task_success']
            return out
        arms[arm]={'gemini':old['arms'][arm], 'luna':new['arms'][arm], 'paired_outcomes':dict(dimensions),
            'gemini_by_family':by_family(list(before.values())), 'luna_by_family':by_family(nr),
            'gemini_factual_diagnostic_only':factual_diagnostics(args.gemini,list(before.values())),
            'luna_factual_diagnostic_only':factual_diagnostics(args.luna,nr),
            'luna_accounting_dimensions':dict(totals),'observed_luna_tiers':dict(tiers),
            'luna_standard_tariff_known_usage_usd':{'minimum':str(low/1000000),'maximum':str(high/1000000)},
            'luna_unknown_usage_not_priced':totals['unknown_usage_requests']}
    report={'same_suite_bytes':True,'scheduled_per_provider':72,'arms':arms,
        'gemini_receipt_sha256':digest(args.gemini/'public-report.json'),
        'luna_receipt_sha256':digest(args.luna/'public-report.json'),
        'pricing_source':'https://developers.openai.com/api/docs/pricing',
        'cache_accounting_source':'https://developers.openai.com/api/docs/guides/prompt-caching',
        'pricing_checked_utc_date':'2026-10-30','invoice_cost':'unavailable',
        'preparation':'scripted sources/notes and verified Broker skill; zero creation inference, authoring labor and production learning costs unmeasured',
        'limitations':['one public development corpus, not an independent holdout','both providers preserve original V3 scores; projection rejections remain failures',
        'Standard API estimates exclude account adjustments and unknown usage; missing cache lifetimes retain a range','Gemini subscription cost is not converted to an API token price',
        'provider-specific native schema/name encoding and implicit caching differ; reasoning High is not equal compute across providers']}
    with args.output.open('x') as f:json.dump(report,f,indent=2)
    print(json.dumps({'same_suite_bytes':True,'arms':{k:{'luna_success':v['luna']['task_success'],'gemini_success':v['gemini']['task_success'],'luna_standard_tariff_known_usage_usd':v['luna_standard_tariff_known_usage_usd']} for k,v in arms.items()}}))


if __name__=='__main__':
    main()
