"""Render observed numbers without turning AI diagnostics into acceptance."""
import json,pathlib
p=pathlib.Path(__file__).parent;d=json.loads((p/'evidence/retrieval-results.json').read_text())
def rate(r):return f"{r['numerator']}/{r['denominator']} ({r['rate']:.1%})"
def interval(r):return f"[{r[0]:.3f}, {r[1]:.3f}]"
lines=['# Измеренные результаты поиска','',
'Все оценки ниже основаны на одном AI, который также писал аннотации и запросы. Это не независимая человеческая приёмка. Для исходных450 метрики учитывают только известные положительные seeds; для Колобков оценены все52 кандидата на каждый запрос.','']
for split,title in [('calibration','Calibration —20 положительных, настройка порога'),('test','Исходные450 —70 положительных запросов'),('kolobki','Колобки —20 положительных запросов')]:
    lines+=['## '+title,'','| Система | Success@5 (seed-hit для450) | MRR@10 | nDCG@10 |','|---|---:|---:|---:|']
    for system,v in d['cohorts'][split].items():
        n=v.get('ndcg_at10');lines.append(f"| {system} | {rate(v['success_at5'])} | {v['mrr_at10']['mean']:.3f} | {n['mean']:.3f} |" if n else f"| {system} | {rate(v['success_at5'])} | {v['mrr_at10']['mean']:.3f} | не вычисляется |")
    lines+=['','| Система | False acceptance на no-match | Positive coverage | Из принятых top1 совпали с qrels/seeds |','|---|---:|---:|---:|']
    for system,v in d['abstention'].items():
        c=v['cohorts'][split];a=c['accepted_top1_known_correct'];lines.append(f"| {system} | {rate(c['false_accept'])} | {rate(c['positive_coverage'])} | {rate(a) if a else '0/0'} |")
    lines+=['','Пороги выбраны только на исходной calibration. Для Колобков это перенос, а не настройка на этих25 запросах.','']
lines+=['## Парные различия','',
'2000 bootstrap resamples по одним и тем же положительным запросам. Общие визуальные семейства и один автор ограничивают интерпретацию интервалов.','',
'| Когорта / сравнение | Δ Success@5, 95% CI | Δ nDCG@10, 95% CI |','|---|---:|---:|']
for name,v in d['paired_comparisons'].items():
    s=v['success_at5_difference'];n=v.get('ndcg_at10_difference');nv=f"{n['mean']:.3f} {interval(n['query_bootstrap_95'])}" if n else 'не вычисляется'
    lines.append(f"| {name} | {s['mean']:.3f} {interval(s['query_bootstrap_95'])} | {nv} |")
r=d['runtime'];lines+=['','## Ресурсы','',f"Загрузка модели: {r['load_seconds']:.2f} s. Полный wall time: {r['total_seconds']:.2f} s.",f"Peak torch allocated/reserved: {r['gpu_peak_allocated_bytes']/2**20:.1f}/{r['gpu_peak_reserved_bytes']/2**20:.1f} MiB. Peak process RSS: {r['rss_peak_kib']/1024:.1f} MiB.",f"Encode batches: {r['encode_batches']}; изображений/кадров: {r['encoded_frames']}; текстовые входы: {r['text_input_counts']}. Это embedding calls, не новые generative captioning calls.",f"Повтор первого текстового запроса: max absolute vector delta {r['text_repeat_max_abs_delta']:.8g}. Один такой повтор не оценивает общую нестабильность.",'']
for bg in ['light','dark']:
    v=r[bg];lines.append(f"- {bg}: {v['items']} элементов, p50/p95 {v['p50_seconds']:.3f}/{v['p95_seconds']:.3f} s на элемент, суммарно {v['total_seconds']:.2f} s.")
lines+=['','Время включает encode с batch<=4 и синхронизацией CUDA; не является задержкой production-запроса. Полные per-batch/per-item timings сохранены отдельно. RSS включает file mappings, не равен cgroup charge. Actual visual-token instrumentation и энергопотребление для этого runner отсутствуют; значения model card не выдаются за измерение. Платные API/аренда не использованы.','',
'Сырые знаменатели, Wilson95 для долей, категории, отдельные ошибки и thresholds: `evidence/retrieval-results.json`. Top10: `evidence/retrieval-top10.json`. Независимый pool исходных450 подготовлен отдельно и пока не оценён.','']
with (p/'retrieval-results.md').open('x') as f:f.write('\n'.join(lines))
