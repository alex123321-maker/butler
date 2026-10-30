#!/usr/bin/env python3
"""Join completed, immutable workflow reports without changing any score."""
import argparse
import csv
import hashlib
import json
import pathlib


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('reports', type=pathlib.Path, nargs=3)
    p.add_argument('--output-dir', type=pathlib.Path, required=True)
    p.add_argument('--paged-reports', type=pathlib.Path, nargs=3, required=True)
    args = p.parse_args()
    reports = [json.loads(x.read_text()) for x in args.reports]
    paged = {r['model']: r for r in (json.loads(x.read_text()) for x in args.paged_reports)}
    for path, r in zip(args.reports, reports):
        fixed = paged[r['model']]
        assert fixed['legacy_reproduced_exactly'] and fixed['native_requests'] == 0
        assert fixed['original_manifest_sha256'] == hashlib.sha256(path.with_name('public-report.json').read_bytes()).hexdigest()
        assert fixed['suite_sha256_bytes'] == r['suite_sha256_bytes'] and fixed['native_source'] == r['source']
        assert all(fixed['legacy_success'].get(a, 0) == r['arms'][a]['task_success'] for a in 'ABC')
    assert {r['model']: r['reasoning_effort'] for r in reports} == {
        'gemini-3.8-flash-high': 'provider_default_high',
        'gpt-6.1-terra': 'none', 'gpt-6-luna': 'max'}
    assert len({r['suite_sha256_bytes'] for r in reports}) == 1
    assert len({r['source'] for r in reports}) == 1
    schedule = [[(x['sequence'], x['case_id'], x['arm']) for x in r['rows']] for r in reports]
    assert schedule[0] == schedule[1] == schedule[2]
    assert all(r['protocol'] == 'skill-workflow-v1' and r['all_slots_observed']
        and r['total']['scheduled'] == 36 and r['total']['timed_cases'] == 36
        and r['core_missing_attempts'] == r['core_token_mismatches'] == 0 for r in reports)
    rows = []
    for r in reports:
        for x in r['rows']:
            fixed = paged[r['model']]['rows'][x['sequence']-1]
            assert fixed['sequence'] == x['sequence'] and fixed['arm'] == x['arm'] and fixed['case_id'] == x['case_id']
            row = {'model': r['model'], 'reasoning_effort': r['reasoning_effort']}
            row['paged_task_success'] = fixed['paged']['task_success']
            row['paged_projection_error'] = fixed['paged_projection_error']
            for key in ('sequence', 'case_id', 'arm', 'condition', 'task_success',
                'physical_artifact_success', 'native_method_attempts', 'native_methods_dispatched',
                'completed_skill_loads', 'unknown_usage', 'known_total_tokens', 'native_failures'):
                row[key] = x[key]
            for key in ('execution_seconds', 'mean_provider_call_seconds', 'provider_wait_seconds',
                'butler_total_seconds', 'tool_rpc_seconds_within_butler', 'butler_other_seconds',
                'preparation_seconds', 'observer_seconds', 'full_phases_seconds'):
                row[key] = x['timing'][key]
            row['known_usage_tariff_usd_minimum'] = x['known_usage_tariff_usd']['minimum'] if x['known_usage_tariff_usd'] else None
            row['known_usage_tariff_usd_maximum'] = x['known_usage_tariff_usd']['maximum'] if x['known_usage_tariff_usd'] else None
            row['score_reasons'] = json.dumps(x['reasons'], ensure_ascii=False)
            row['finish_reasons'] = json.dumps(x['finish_reasons'], sort_keys=True)
            rows.append(row)
    assert len(rows) == 108
    args.output_dir.mkdir(parents=True, exist_ok=False)
    with (args.output_dir/'cases.csv').open('x', newline='', encoding='utf-8') as f:
        writer = csv.DictWriter(f, fieldnames=list(rows[0]))
        writer.writeheader()
        writer.writerows(rows)
    result = {'protocol': 'skill-workflow-v1', 'source': reports[0]['source'],
        'suite_sha256_bytes': reports[0]['suite_sha256_bytes'],
        'reports': [{'file': str(path), 'sha256': hashlib.sha256(path.read_bytes()).hexdigest()}
            for path in args.reports],
        'models': [{k: r[k] for k in ('model', 'reasoning_effort', 'total', 'arms',
            'native_window_wall_seconds', 'window_seconds_outside_measured_phases')} for r in reports],
        'scheduled': 108,
        'paged_success': {model: r['paged_success'] for model, r in paged.items()},
        'interpretation': ['A: no experience; B: prior notes; C: the same notes plus a reviewed skill.',
            'C minus B estimates the incremental skill effect only on this public development cohort.',
            'Provider wait includes network and adapter work; pure model reasoning duration is unknown.',
            'Butler time is execution minus provider wait. Tool RPC is already included.',
            'Full phases add fixture preparation and observation; native window additionally covers teardown and gaps.',
            'Known API tariff ranges are not invoices; Gemini subscription cost is not converted to a token price.',
            'Historical receipts and unsuccessful attempts remain preserved; no older score is replaced.']}
    with (args.output_dir/'comparison.json').open('x') as f:
        json.dump(result, f, indent=2)
    lines = ['# Skill workflow comparison', '',
        'A — без опыта; B — прошлые заметки; C — те же заметки и проверенный навык.', '',
        '## Все 36 задач каждого профиля', '',
        '| Модель | Строгий успех A/B/C | Решение, мин | Среднее обращение, с | Butler, с | Подготовка + наблюдение, с | Полное окно, мин |',
        '|---|---:|---:|---:|---:|---:|---:|']
    for r in reports:
        t = r['total']
        score = '/'.join(str(r['arms'][a]['task_success']) for a in 'ABC')
        lines.append(f"| {r['model']} ({r['reasoning_effort']}) | {score} из 12 | {t['execution_seconds']/60:.2f} | {t['mean_provider_call_seconds']:.2f} | {t['butler_total_seconds']:.2f} | {t['preparation_seconds']+t['observer_seconds']:.2f} | {r['native_window_wall_seconds']/60:.2f} |")
    lines += ['', '## Расход нового окна', '',
        '| Модель | Вызовы | Полные токены | Неизвестный расход вызовов | Расчёт по тарифам, USD |',
        '|---|---:|---:|---:|---:|']
    for r in reports:
        t = r['total']
        cost = t['known_usage_tariff_usd']
        quote = f"{float(cost['minimum']):.6f}–{float(cost['maximum']):.6f}" if cost else 'подписка; доля неизвестна'
        lines.append(f"| {r['model']} | {t['native_methods_dispatched']} | {t['known_full_tokens']} | {t['unknown_usage']} | {quote} |")
    lines += ['', 'Расчёт использует известный расход и опубликованные стандартные тарифы; это не счёт провайдера. Прежние расходы сохранены в закреплённых квитанциях и не сложены повторно с перекрывающимися окнами.', '', '## По вариантам', '',
        '| Модель | Вариант | Строгий успех | Корректные артефакты из 8 | Вызовы модели | Задачи с загрузкой навыка | Решение, с | Среднее обращение, с | Butler, с |',
        '|---|---|---:|---:|---:|---:|---:|---:|---:|']
    for r in reports:
        for a in 'ABC':
            t = r['arms'][a]
            lines.append(f"| {r['model']} | {a} | {t['task_success']}/12 | {t['physical_artifact_success']} | {t['native_methods_dispatched']} | {t['cases_with_completed_skill_load']} | {t['execution_seconds']:.2f} | {t['mean_provider_call_seconds']:.2f} | {t['butler_total_seconds']:.2f} |")
    lines += ['', '## Поправка полного чтения страниц', '',
        '| Модель | Исходный успех A/B/C | Успех с проверкой объединения страниц A/B/C |',
        '|---|---:|---:|']
    for r in reports:
        old = '/'.join(str(r['arms'][a]['task_success']) for a in 'ABC')
        fixed = '/'.join(str(paged[r['model']]['paged_success'].get(a, 0)) for a in 'ABC')
        lines.append(f"| {r['model']} | {old} | {fixed} |")
    lines += ['', 'Поправка применена офлайн ко всем трассам: полный текст может быть прочитан несколькими проверенными страницами. Исходные оценки точно воспроизведены и сохранены. Новых обращений к модели для поправки не было.', '',
        'Каждая из 108 задач, включая неуспешные, приведена в [cases.csv](cases.csv).', '',
        'Время решения: от приёма задачи до возврата выполнения. Ожидание модели включает сеть и переходник; чистое время размышления неизвестно. Butler включает RPC инструментов — повторно прибавлять их нельзя. Подготовка стенда, навыка и сбор результатов измерены отдельно; полное окно дополнительно включает завершение окружения задач и промежутки.', '',
        'Измерения относятся к исследовательскому стенду: бинарник собран с Go race, а время системы включает запись доказательного журнала и трасс. Отдельного замера накладных расходов записи нет; это не замер задержки production.', '',
        'Один открытый набор разработки, без независимой скрытой выборки. Подготовка навыка выполнена по сценарию; стоимость его автономного создания и труд автора не измерены. Оценка корректности артефакта отделена от строгого завершения задачи. Старые результаты не пересчитаны.', '']
    (args.output_dir/'README.md').write_text('\n'.join(lines), encoding='utf-8')
    print(json.dumps({'scheduled': len(rows), 'models': [r['model'] for r in reports],
        'output_dir': str(args.output_dir)}))


if __name__ == '__main__':
    main()
