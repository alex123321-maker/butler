#!/usr/bin/env python3
"""Stdlib reporting only; frozen native/legacy evidence is never modified."""
import argparse
import csv
import hashlib
import json
import pathlib
import decimal
import runpy

PROFILES = ('gemini-medium', 'gemini-low', 'luna-max', 'luna-high', 'luna-low')


def read(p):
    return json.loads(p.read_text())


def load(root, label):
    r = read(root/label/'workflow-report.json')
    e = read(root/label/'derived/paged-report.json')
    assert r['all_slots_observed'] and r['total']['scheduled'] == r['total']['timed_cases'] == 36
    assert r['core_missing_attempts'] == r['core_token_mismatches'] == 0
    assert len(r['rows']) == len(e['rows']) == e['scheduled'] == 36
    assert e['legacy_reproduced_exactly'] and e['native_source'] == r['source']
    assert e['suite_sha256_bytes'] == r['suite_sha256_bytes']
    assert e['original_manifest_sha256'] == hashlib.sha256((root/label/'public-report.json').read_bytes()).hexdigest()
    assert all(a['sequence'] == b['sequence'] and a['case_id'] == b['case_id'] and a['arm'] == b['arm']
        for a, b in zip(r['rows'], e['rows']))
    return r, e


def cost(t, reconciled=None):
    c = reconciled['known_usage_tariff_usd'] if reconciled else t['known_usage_tariff_usd']
    if c is None:
        return 'доля подписки неизвестна'
    known = f"{float(c['minimum']):.6f}–{float(c['maximum']):.6f}"
    unknown = reconciled['remaining_unknown_usage'] if reconciled else t['unknown_usage']
    return known + (f" + неизвестно ({unknown} обращ.)" if unknown else '')


def reconcile(root, report):
    path = root/'reconciliation/retrieval-accounting.json'
    if not path.exists():
        return None
    receipt = read(path)
    manifest = read(path.with_name('manifest.json'))
    assert manifest['receipt_sha256'] == hashlib.sha256(path.read_bytes()).hexdigest()
    assert receipt['native_generation_requests'] == 0 and receipt['original_unknown_and_scores_unchanged']
    with (root/'luna-max/workflow-report.calls.csv').open(newline='') as f:
        calls = {(int(x['slot']), int(x['turn'])): x for x in csv.DictReader(f)}
    price = runpy.run_path(str(pathlib.Path(__file__).with_name('memory-utility-provider-comparison.py')))['cost']
    low = high = decimal.Decimal(0)
    tokens = 0
    recovered = []
    for row in receipt['rows']:
        assert calls[row['slot'], row['turn']]['unknown_usage'] == 'True'
        usage = row['usage']
        if usage is None:
            continue
        assert row['http_status'] == 200 and row['model'] == 'gpt-6-luna'
        assert row['status'] in ('completed', 'incomplete', 'failed', 'cancelled')
        assert 0 <= usage['input_tokens'] <= 65536 and 0 <= usage['output_tokens'] <= 32768
        assert usage['total_tokens'] == usage['input_tokens'] + usage['output_tokens']
        native = dict(zip(('InputTokens', 'OutputTokens', 'TotalTokens'),
            (usage['input_tokens'], usage['output_tokens'], usage['total_tokens'])))
        details = usage.get('input_tokens_details', {})
        for src, dst in [('cached_tokens', 'CachedInputTokens'), ('cache_write_tokens', 'CacheWriteInputTokens')]:
            if src in details:
                native[dst] = details[src]
        quote = price(native, 'gpt-6-luna')
        low += quote[0]/1000000
        high += quote[1]/1000000
        tokens += usage['total_tokens']
        recovered.append({'slot': row['slot'], 'turn': row['turn'], 'known_tokens': usage['total_tokens']})
    assert len({(x['slot'], x['turn']) for x in recovered}) == len(recovered)
    original = report['total']
    assert len(recovered) <= original['unknown_usage']
    return {'recovered_calls': recovered, 'api_read_requests': receipt['api_read_requests'],
        'native_generation_requests': 0, 'receipt_sha256': manifest['receipt_sha256'],
        'original_capture_unknown_usage': original['unknown_usage'],
        'remaining_unknown_usage': original['unknown_usage']-len(recovered),
        'known_full_tokens': original['known_full_tokens']+tokens,
        'additional_known_usage_tariff_usd': {'minimum': str(low), 'maximum': str(high)},
        'known_usage_tariff_usd': {'minimum': str(decimal.Decimal(original['known_usage_tariff_usd']['minimum'])+low),
            'maximum': str(decimal.Decimal(original['known_usage_tariff_usd']['maximum'])+high)}}


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--root', type=pathlib.Path, required=True)
    p.add_argument('--baseline', type=pathlib.Path, required=True)
    p.add_argument('--output', type=pathlib.Path, required=True)
    a = p.parse_args()
    reports = [(name, *load(a.root, name)) for name in PROFILES]
    assert len({r['source'] for _, r, _ in reports}) == 1
    assert len({r['suite_sha256_bytes'] for _, r, _ in reports}) == 1
    for label, r, _ in reports:
        assert r['profile'] == label and r['optimization'] == 'latency-v1' and r['maximum_output_tokens'] == 32768
    old = {label: load(a.baseline, label) for label in ('gemini', 'luna')}
    accounting = reconcile(a.root, next(r for label, r, _ in reports if label == 'luna-max'))
    comparisons, rows = [], []
    for label, new, score in reports:
        baseline, baseline_score = old['gemini' if label.startswith('gemini-') else 'luna']
        assert baseline['suite_sha256_bytes'] == new['suite_sha256_bytes']
        comparisons.append({'profile': label, 'model': new['model'], 'effort': new['reasoning_effort'],
            'success': score['paged_success'], 'total': new['total'], 'arms': new['arms'],
            'window_wall_seconds': new['native_window_wall_seconds'],
            'previous_effort': baseline['reasoning_effort'], 'previous_success': baseline_score['paged_success'],
            'previous_total': baseline['total'], 'previous_arms': baseline['arms']})
        comparisons[-1]['post_run_accounting'] = accounting if label == 'luna-max' else None
        for x, y, sx, sy in zip(baseline['rows'], new['rows'], baseline_score['rows'], score['rows']):
            assert (x['sequence'], x['case_id'], x['arm']) == (y['sequence'], y['case_id'], y['arm'])
            row = {'profile': label, 'sequence': y['sequence'], 'case_id': y['case_id'], 'arm': y['arm'],
                'task_success': sy['paged']['task_success'], 'legacy_task_success': y['task_success'],
                'physical_artifact_success': y['physical_artifact_success'],
                'previous_task_success': sx['paged']['task_success'],
                'completed_skill_loads': y['completed_skill_loads'],
                'native_methods_dispatched': y['native_methods_dispatched'],
                'known_total_tokens': y['known_total_tokens'], 'unknown_usage': y['unknown_usage'],
                'failure_reasons': json.dumps(sy['paged']['reasons'], ensure_ascii=False),
                'projection_error': y['projection_error'],
                'answer_states': json.dumps(y['answer_states']),
                'known_usage_tariff_usd': json.dumps(y['known_usage_tariff_usd'])}
            for k in ('execution_seconds', 'provider_wait_seconds', 'mean_provider_call_seconds',
                'butler_total_seconds', 'tool_rpc_seconds_within_butler', 'preparation_seconds', 'observer_seconds'):
                row[k] = y['timing'][k]
                row['previous_'+k] = x['timing'][k]
            row['first_text_delivery_seconds'] = y['timing']['first_text_delivery_seconds']
            row['post_run_recovered_tokens'] = sum(x['known_tokens'] for x in accounting['recovered_calls'] if x['slot']==y['sequence']) if accounting and label=='luna-max' else 0
            rows.append(row)
    a.output.mkdir()
    with (a.output/'comparison.json').open('x') as f:
        json.dump({'profiles': comparisons, 'paired_cases': len(rows),
            'native_source': reports[0][1]['source'], 'suite_sha256_bytes': reports[0][1]['suite_sha256_bytes'],
            'reporter_sha256': hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest(),
            'limitations': ['One public 12-case development corpus, one sample per arm/profile.',
                'Old/new comparisons change optimization, output bound and sometimes effort; no causal isolation.',
                'First text is measured at a local Core sink, not Web/Telegram rendering or delivery ACK.',
                'Concurrent stream guarding overlaps provider wait; residual system elapsed is not system CPU.',
                'Unknown usage, old failed calls and overlapping historical cost allocations are preserved.']}, f, indent=2)
    with (a.output/'paired-cases.csv').open('x', newline='', encoding='utf-8') as f:
        writer = csv.DictWriter(f, fieldnames=list(rows[0]))
        writer.writeheader()
        writer.writerows(rows)
    lines = ['# Сравнение после сокращения шагов и изменения уровней рассуждений', '',
        'A — без прошлого опыта; B — заметки; C — те же заметки и навык. В каждом варианте 12 задач.', '',
        '| Профиль | Успех A/B/C | Все 36 задач, мин | В среднем задача, с | Обращение к модели, с | Система всего, с |',
        '|---|---:|---:|---:|---:|---:|']
    fmt_score = lambda e: '/'.join(str(e.get(k, 0)) for k in 'ABC')
    for label, r, e in [('Gemini High, прежний', *old['gemini']), ('Luna max, прежний', *old['luna'])] + reports:
        t = r['total']
        lines.append(f"| {label} | {fmt_score(e['paged_success'])} | {t['execution_seconds']/60:.2f} | {t['execution_seconds']/36:.2f} | {t['mean_provider_call_seconds']:.2f} | {t['butler_total_seconds']:.2f} |")
    lines += ['', '| Профиль | Обращения | Полные токены | Стоимость USD | Неизвестный расход обращений | Первый текст Core, среднее с / задач с замером |',
        '|---|---:|---:|---:|---:|---:|']
    for label, r, _ in reports:
        t = r['total']
        rec = accounting if label == 'luna-max' else None
        first = t['mean_first_text_delivery_seconds']
        first = f"{first:.2f}" if first is not None else 'неизвестно'
        tokens = rec['known_full_tokens'] if rec else t['known_full_tokens']
        unknown = rec['remaining_unknown_usage'] if rec else t['unknown_usage']
        lines.append(f"| {label} | {t['native_methods_dispatched']} | {tokens} | {cost(t,rec)} | {unknown} | {first} / {t['cases_with_first_text_delivery']} |")
    lines += ['', '| Профиль | Вариант | Успех | Правильные проверенные артефакты из 8 | Обращения | Время задач, с | Система, с |',
        '|---|---|---:|---:|---:|---:|---:|']
    for label, r, e in reports:
        for arm in 'ABC':
            t = r['arms'][arm]
            lines.append(f"| {label} | {arm} | {e['paged_success'].get(arm,0)}/12 | {t['physical_artifact_success']} | {t['native_methods_dispatched']} | {t['execution_seconds']:.2f} | {t['butler_total_seconds']:.2f} |")
    lines += ['', 'Основная оценка использует одинаковую проверку полного чтения страниц; исходные оценки сохранены. Стоимость включает известный расход неудачных обращений, но не является счётом провайдера. Доля подписки Gemini и труд создания навыков неизвестны.', '',
        'Для Luna max отдельная сверка GET уже созданных ответов восстановила квитанцию одного из трёх UNKNOWN. Исходные журналы и оценки не изменены; числовые расходы добавлены ровно один раз. Два 404 не доказывают нулевого расхода. Полная стоимость max неизвестна. Три чтения API не создавали генераций и выполнены после закрытия временного окна.', '',
        'Система — прошедшее время вне ожидания провайдера; это не CPU. Ожидание включает сеть и переходник. Проверки и выдача текста могут идти параллельно генерации. Полное окно, подготовка, наблюдение и каждый вызов сохранены в JSON/CSV.', '',
        'Новые профили имеют одинаковые оптимизации и бюджет выхода 32768. Прежний бюджет — 8192; у Gemini также изменён уровень рассуждений. Поэтому разницу с прежними результатами нельзя приписать одной правке. На этом небольшом открытом наборе нельзя доказать отсутствие потери качества в общем случае.', '',
        '[Все 180 пар задач с отдельными замерами](paired-cases.csv).', '']
    (a.output/'README.md').write_text('\n'.join(lines), encoding='utf-8')
    print(json.dumps({'paired_cases': len(rows), 'profiles': [{'profile': x['profile'], 'success': x['success'],
        'seconds': x['total']['execution_seconds']} for x in comparisons]}))


if __name__ == '__main__':
    main()
