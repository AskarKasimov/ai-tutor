"""Structural signals, not semantic grades. Compare both translations to source."""
from collections import Counter
import csv
import hashlib
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parent
SOURCE = ROOT.parent / 'GIC_diagnostic_handoff_v1/data/course_graph.v1.json'
LOCALIZED = ROOT / 'GIC_RU_translation_partial_v1/course_graph.ru-draft.v1.1.json'
MARKER = re.compile(r'\[D\d+\s*\|[^\]\n]+\]')
NUMBER = re.compile(r'(?<![\w])\d+(?:[.,]\d+)*(?:%|\b)')
CODE = re.compile(r'```[\s\S]*?```|`[^`\n]+`')
MATH = re.compile(r'\$\$[\s\S]*?\$\$|\$[^$\n]+\$|\\\[[\s\S]*?\\\]')


def dump(path, obj):
    path.write_text(json.dumps(obj, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


def signals(source, output):
    src = MARKER.sub('', source)
    dst = MARKER.sub('', output)
    numbers_a, numbers_b = Counter(NUMBER.findall(src)), Counter(NUMBER.findall(dst))
    return {
        'markers_exact': MARKER.findall(source) == MARKER.findall(output),
        'source_markers': len(MARKER.findall(source)),
        'output_markers': len(MARKER.findall(output)),
        'missing_numeric_tokens': list((numbers_a - numbers_b).elements()),
        'added_numeric_tokens': list((numbers_b - numbers_a).elements()),
        'code_exact': CODE.findall(source) == CODE.findall(output),
        'delimited_math_exact': MATH.findall(source) == MATH.findall(output),
        'length_ratio': round(len(output) / max(1, len(source)), 3),
    }


def main():
    canonical = json.loads(SOURCE.read_text(encoding='utf-8'))
    localized = json.loads(LOCALIZED.read_text(encoding='utf-8'))
    original = {t['id']: t for t in canonical['topics']}
    assert len(localized['topics']) == len(original)
    assert {t['id'] for t in localized['topics']} == set(original)
    for t in localized['topics']:
        assert all(t[k] == v for k, v in original[t['id']].items()), t['id']
    for k in ['prerequisites', 'diagnostic_groups', 'unmapped_requirements', 'curriculum_gaps']:
        assert localized[k] == canonical[k], k
    results_path = ROOT / 'vache_results.jsonl'
    results = [json.loads(line) for line in results_path.read_text(encoding='utf-8').splitlines()] if results_path.exists() else []
    assert len(results) == len({r['id'] for r in results})
    by_id = {r['id']: r for r in results}
    details = []
    comparison = ['# Оригинал / предоставленный перевод / модуль Ваче\n',
                  'Предоставленный перевод — сравнительный черновик, не безошибочный эталон.\n']
    for t in localized['topics']:
        if not t.get('full_text_ru'):
            continue
        r = by_id.get(t['id'])
        row = {'id': t['id'], 'title': t['title'], 'source_chars': len(t['full_text']),
               'reference': signals(t['full_text'], t['full_text_ru']),
               'candidate': signals(t['full_text'], r['full_text_ru']) if r and not r.get('error') else None,
               'error': r.get('error') if r else 'not_run'}
        details.append(row)
        if r and not r.get('error'):
            comparison.extend([f"\n## {t['id']} — {t['title']}\n", '### Оригинал\n', t['full_text'],
                               '\n### Предоставленный перевод\n', t['full_text_ru'],
                               '\n### Модуль Ваче\n', r['full_text_ru']])
    dump(ROOT / 'structural_details.json', details)
    summaries = {}
    for label in ['reference', 'reference_matched_subset', 'candidate']:
        field = 'reference' if label == 'reference_matched_subset' else label
        entries = [d[field] for d in details if d[field] is not None and (label != 'reference_matched_subset' or d['candidate'] is not None)]
        summaries[label] = {
            'topics': len(entries),
            'marker_exact_topics': sum(x['markers_exact'] for x in entries),
            'numeric_difference_topics': sum(bool(x['missing_numeric_tokens'] or x['added_numeric_tokens']) for x in entries),
            'code_difference_topics': sum(not x['code_exact'] for x in entries),
            'delimited_math_difference_topics': sum(not x['delimited_math_exact'] for x in entries),
        }
    summaries['source_sha256'] = hashlib.sha256(SOURCE.read_bytes()).hexdigest()
    summaries['localized_sha256'] = hashlib.sha256(LOCALIZED.read_bytes()).hexdigest()
    summaries['original_fields_preserved'] = True
    summaries['limitations'] = 'Counts flag review candidates, not semantic errors; plain-text mathematics requires manual review. Reference covers 13.2% of original text.'
    dump(ROOT / 'structural_summary.json', summaries)
    (ROOT / 'comparison.md').write_text('\n\n'.join(comparison) + '\n', encoding='utf-8')
    with (ROOT / 'structural_comparison.csv').open('w', encoding='utf-8', newline='') as f:
        writer = csv.writer(f)
        writer.writerow(['id', 'variant', 'markers_exact', 'missing_numbers', 'added_numbers', 'code_exact', 'math_exact', 'length_ratio'])
        for row in details:
            for label in ['reference', 'candidate']:
                s = row[label]
                if s:
                    writer.writerow([row['id'], label, s['markers_exact'], ' '.join(s['missing_numeric_tokens']), ' '.join(s['added_numeric_tokens']), s['code_exact'], s['delimited_math_exact'], s['length_ratio']])
    print(json.dumps(summaries, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
