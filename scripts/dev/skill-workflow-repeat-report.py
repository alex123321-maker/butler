#!/usr/bin/env python3
"""Stdlib artifact analysis only: compare closed windows without changing scores."""
import argparse
import csv
import hashlib
import json
import pathlib


def read(path):
    return json.loads(path.read_text())


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--previous', type=pathlib.Path, required=True)
    p.add_argument('--current', type=pathlib.Path, required=True)
    p.add_argument('--output-dir', type=pathlib.Path, required=True)
    args = p.parse_args()
    result, paired = [], []
    for label in ('gemini', 'terra', 'luna'):
        old = read(args.previous/label/'workflow-report.json')
        new = read(args.current/label/'workflow-report.json')
        old_paged = read(args.previous/'paged'/(label+'-paged.json'))
        new_paged = read(args.current/label/'derived/paged-report.json')
        assert old['model'] == new['model']
        assert old['suite_sha256_bytes'] == new['suite_sha256_bytes']
        assert old['all_slots_observed'] and new['all_slots_observed']
        assert old_paged['legacy_reproduced_exactly'] and new_paged['legacy_reproduced_exactly']
        for r, e, root in ((old, old_paged, args.previous), (new, new_paged, args.current)):
            manifest = root/label/'public-report.json'
            assert e['original_manifest_sha256'] == hashlib.sha256(manifest.read_bytes()).hexdigest()
            assert e['native_source'] == r['source'] and e['suite_sha256_bytes'] == r['suite_sha256_bytes']
            assert r['total']['scheduled'] == r['total']['timed_cases'] == 36
            assert r['core_missing_attempts'] == r['core_token_mismatches'] == 0
        arms = []
        for arm in 'ABC':
            a, b = old['arms'][arm], new['arms'][arm]
            values = {'arm': arm, 'previous_success': old_paged['paged_success'].get(arm, 0),
                'current_success': new_paged['paged_success'].get(arm, 0),
                'previous': a, 'current': b,
                'delta': {k: b[k]-a[k] for k in ('execution_seconds', 'provider_wait_seconds',
                    'butler_total_seconds', 'native_methods_dispatched', 'known_full_tokens',
                    'physical_artifact_success', 'unknown_usage')}}
            arms.append(values)
        for a, b, x, y in zip(old['rows'], new['rows'], old_paged['rows'], new_paged['rows']):
            assert (a['sequence'], a['case_id'], a['arm']) == (b['sequence'], b['case_id'], b['arm'])
            assert a['sequence'] == x['sequence'] == y['sequence']
            item = {'model': new['model'], 'sequence': a['sequence'], 'case_id': a['case_id'],
                'arm': a['arm'], 'previous_effort': old['reasoning_effort'], 'current_effort': new['reasoning_effort'],
                'previous_success': x['paged']['task_success'], 'current_success': y['paged']['task_success'],
                'previous_physical_artifact_success': a['physical_artifact_success'],
                'current_physical_artifact_success': b['physical_artifact_success']}
            for prefix, row in (('previous', a), ('current', b)):
                for key in ('execution_seconds', 'mean_provider_call_seconds', 'provider_wait_seconds',
                    'butler_total_seconds', 'tool_rpc_seconds_within_butler', 'preparation_seconds',
                    'observer_seconds', 'full_phases_seconds'):
                    item[prefix+'_'+key] = row['timing'][key]
                for key in ('native_methods_dispatched', 'known_total_tokens', 'unknown_usage', 'completed_skill_loads'):
                    item[prefix+'_'+key] = row[key]
            paired.append(item)
        result.append({'model': new['model'], 'previous_effort': old['reasoning_effort'],
            'current_effort': new['reasoning_effort'], 'previous': old['total'], 'current': new['total'],
            'previous_success': old_paged['paged_success'], 'current_success': new_paged['paged_success'],
            'previous_window_wall_seconds': old['native_window_wall_seconds'],
            'current_window_wall_seconds': new['native_window_wall_seconds'], 'arms': arms})
    assert len(paired) == 108
    args.output_dir.mkdir(parents=True, exist_ok=False)
    with (args.output_dir/'paired-cases.csv').open('x', newline='', encoding='utf-8') as f:
        writer = csv.DictWriter(f, fieldnames=list(paired[0])); writer.writeheader(); writer.writerows(paired)
    with (args.output_dir/'repeat.json').open('x') as f:
        json.dump({'models': result, 'paired_cases': len(paired), 'new_inference_for_reporting': 0,
            'limitations': ['Terra changed none to medium as requested; its difference does not isolate the stream fix.',
                'One public development cohort per profile; model randomness, provider latency and cache state are uncontrolled.',
                'System time includes race instrumentation and in-run research recording; provider boundary includes network/adapter.',
                'Creation is scripted, authorship costs and subscription allocation/invoices are unmeasured.',
                'Prior paid windows, failed attempts and unknown usage are never overwritten or added again from overlapping receipts.']}, f, indent=2)
    lines = ['# Повторное A/B/C-сравнение после исправления проверок навыка', '',
        'A — без опыта; B — заметки; C — те же заметки и проверенный навык. По 12 задач на вариант, 36 на профиль.', '',
        'Основная оценка использует одинаковую проверку полного чтения страниц для старых и новых трасс. Исходные оценки сохранены.', '',
        '| Профиль | Успех A/B/C до → после | Решение всех задач, мин до → после | Среднее обращение, с до → после | Система, с до → после |',
        '|---|---:|---:|---:|---:|']
    for r in result:
        a, b = r['previous'], r['current']
        score = lambda d: '/'.join(str(d.get(arm, 0)) for arm in 'ABC')
        lines.append(f"| {r['model']} ({r['previous_effort']} → {r['current_effort']}) | {score(r['previous_success'])} → {score(r['current_success'])} | {a['execution_seconds']/60:.2f} → {b['execution_seconds']/60:.2f} | {a['mean_provider_call_seconds']:.2f} → {b['mean_provider_call_seconds']:.2f} | {a['butler_total_seconds']:.2f} → {b['butler_total_seconds']:.2f} |")
    lines += ['', '| Профиль | Вариант | Успех до → после | Корректные артефакты до → после | Вызовы до → после | Система, с до → после |', '|---|---|---:|---:|---:|---:|']
    for r in result:
        for x in r['arms']:
            a, b = x['previous'], x['current']
            lines.append(f"| {r['model']} | {x['arm']} | {x['previous_success']} → {x['current_success']} | {a['physical_artifact_success']} → {b['physical_artifact_success']} | {a['native_methods_dispatched']} → {b['native_methods_dispatched']} | {a['butler_total_seconds']:.2f} → {b['butler_total_seconds']:.2f} |")
    lines += ['', '| Профиль | Полное окно, мин до → после | Токены до → после | Расчётная стоимость USD до → после | Неизвестный расход вызовов до → после |', '|---|---:|---:|---:|---:|']
    def quote(t):
        c = t['known_usage_tariff_usd']
        return f"{float(c['minimum']):.6f}–{float(c['maximum']):.6f}" if c else 'доля подписки неизвестна'
    for r in result:
        a, b = r['previous'], r['current']
        lines.append(f"| {r['model']} | {r['previous_window_wall_seconds']/60:.2f} → {r['current_window_wall_seconds']/60:.2f} | {a['known_full_tokens']} → {b['known_full_tokens']} | {quote(a)} → {quote(b)} | {a['unknown_usage']} → {b['unknown_usage']} |")
    lines += ['', 'Смена Terra none → medium — отдельная переменная: её результат нельзя целиком приписать исправлению навыков.', '',
        'Один открытый набор разработки, без статистической гарантии. Система включает Go race и исследовательскую запись; ожидание провайдера включает сеть и переходник. Чистое время рассуждения неизвестно. Стоимость — оценка известных токенов по стандартным тарифам, с диапазоном неизвестного cache-write retention; это не счёт провайдера. Ошибки и неизвестный расход сохранены.', '',
        '[Все 108 пар задач с отдельными замерами](paired-cases.csv). Полные новые результаты, исходные оценки и каждый вызов находятся рядом в каталогах профилей. Прежние расходы не суммируются повторно из пересекающихся квитанций.', '']
    (args.output_dir/'README.md').write_text('\n'.join(lines), encoding='utf-8')
    print(json.dumps({'paired_cases': len(paired), 'output_dir': str(args.output_dir)}))


if __name__ == '__main__':
    main()
