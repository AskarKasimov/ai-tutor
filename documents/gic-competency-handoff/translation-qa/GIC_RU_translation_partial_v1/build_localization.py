"""Build a lossless bilingual copy of the GIC graph without touching the v1 dataset."""
from __future__ import annotations

import csv
import hashlib
import importlib.util
import json
import re
from collections import Counter
from pathlib import Path

import jsonschema

ROOT = Path(__file__).resolve().parent
BASE = Path('/mnt/data/GIC_diagnostic_handoff_v1')
INPUT = BASE / 'data/course_graph.v1.json'
BASE_SCHEMA = BASE / 'schema/course_graph.schema.json'
BASE_TYPES = BASE / 'schema/gic_types.ts'

spec = importlib.util.spec_from_file_location('manual_translations', ROOT / 'manual_translations.py')
assert spec is not None and spec.loader is not None
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)
translations: dict[str, str] = mod.TRANSLATIONS

original = json.loads(INPUT.read_text(encoding='utf-8'))
jsonschema.validate(original, json.loads(BASE_SCHEMA.read_text(encoding='utf-8')))
updated = json.loads(json.dumps(original, ensure_ascii=False))
updated['schema_version'] = '1.1.0'
updated['dataset_id'] = original['dataset_id'] + '-ru-draft'
updated['provenance']['translation_source'] = 'ai_translation_ru_v1/manual_translations.py'

header_re = re.compile(r'^\[D\d+ \|[^\n]+\| с\.\d+ \|[^\n]+\]$', re.MULTILINE)
rows: list[dict[str, str | int]] = []
pending: list[dict[str, str | int]] = []
translation_qa: list[dict[str, str | int]] = []
for node in updated['topics']:
    identifier = node['id']
    original_text = node.get('full_text', '')
    ru = translations.get(identifier)
    notes: list[str] = []
    if not original_text:
        assert ru is None
        state = 'no_source'
    elif ru:
        src_headers = header_re.findall(original_text)
        ru_headers = header_re.findall(ru)
        assert src_headers == ru_headers, (identifier, src_headers, ru_headers)
        assert re.search('[А-Яа-яЁё]', ru), identifier
        state = 'translated_from_extracted_text'
        if any(term in ru.lower() for term in ('искажен', 'искажён', 'свер', 'ошибк', 'поврежден', 'повреждён', 'восстановлен')):
            notes.append('Перевод сохраняет пометки о повреждённых формулах/схемах или ошибках исходного материала.')
        if node['source_quality'] != 'source_present_visuals_unverified':
            notes.append('У исходной ноды есть ограничения качества; см. source_quality.')
    else:
        state = 'pending_translation'
        pending.append({'id':identifier,'title':node['title'],'section':node['section'],'source_chars':len(original_text),'source_quality':node['source_quality']})
    node['full_text_ru'] = ru
    node['translation_status'] = state
    node['translation_notes'] = notes
    rows.append({'id': identifier, 'title':node['title'],'status':state,'source_chars':len(original_text),'ru_chars':len(ru or ''),'source_pages':len(node['source_pages']),'source_quality':node['source_quality']})
    if ru:
        translation_qa.append({'id':identifier,'page_markers':len(header_re.findall(original_text)),'cyrillic_share':round(sum('А'<=c<='я' or c=='ё' or c=='Ё' for c in ru)/max(len(ru),1),3),'source_quality':node['source_quality'],'notes':'; '.join(notes)})

assert set(translations) == {n['id'] for n in updated['topics'] if n['translation_status']=='translated_from_extracted_text'}

schema = json.loads(BASE_SCHEMA.read_text(encoding='utf-8'))
schema['properties']['schema_version'] = {'const':'1.1.0'}
schema['$defs']['topic']['properties'].update({
    'full_text_ru': {'type':['string','null']},
    'translation_status': {'enum':['translated_from_extracted_text','translated_draft_unreviewed','pending_translation','no_source']},
    'translation_notes': {'type':'array','items':{'type':'string'}},
})
schema['$defs']['topic']['required'].extend(['full_text_ru','translation_status','translation_notes'])
jsonschema.validate(updated, schema)
(ROOT/'course_graph.ru-draft.v1.1.json').write_text(json.dumps(updated,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
(ROOT/'course_graph.ru-draft.schema.json').write_text(json.dumps(schema,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')

names = [n['id'] for n in original['topics']]
assert names == [n['id'] for n in updated['topics']]
for orig,cur in zip(original['topics'],updated['topics']):
    for key in orig:
        assert cur[key] == orig[key], (orig['id'],key)
for key in ['prerequisites','diagnostic_groups','curriculum_gaps','unmapped_requirements','semantics','external_review_status']:
    assert original[key] == updated[key], key

with (ROOT/'translation_status.csv').open('w',newline='',encoding='utf-8-sig') as f:
    writer = csv.DictWriter(f,fieldnames=list(rows[0]));writer.writeheader();writer.writerows(rows)
with (ROOT/'pending_topics.csv').open('w',newline='',encoding='utf-8-sig') as f:
    writer = csv.DictWriter(f,fieldnames=list(pending[0]));writer.writeheader();writer.writerows(pending)
with (ROOT/'qa_translated_topics.csv').open('w',newline='',encoding='utf-8-sig') as f:
    writer = csv.DictWriter(f,fieldnames=list(translation_qa[0]));writer.writeheader();writer.writerows(translation_qa)

with (ROOT/'translation_jobs_pending.jsonl').open('w',encoding='utf-8') as f:
    for node in original['topics']:
        if node.get('full_text') and node['id'] not in translations:
            payload = {'id':node['id'],'title':node['title'],'topic_question':node['question'],'source_quality':node['source_quality'],'source_pages':node['source_pages'],'full_text_en':node['full_text']}
            f.write(json.dumps(payload,ensure_ascii=False)+'\n')

with (ROOT/'sample_translated_topics.md').open('w',encoding='utf-8') as f:
    for sample_id in ['AI-01-01','AI-02-02','AI-04-07','AI-05-08','AI-10-09']:
        node = next(n for n in updated['topics'] if n['id']==sample_id)
        f.write(f"# {node['id']} — {node['title']}\n\n**Вопрос:** {node['question']}\n\n**Provides:** {'; '.join(node['provides'])}\n\n**Requires:** {'; '.join(node['requires'])}\n\n## Перевод\n\n{node['full_text_ru']}\n\n## Оригинал\n\n{node['full_text']}\n\n---\n\n")

# Non-breaking topic augmentation in TS.
typescript=BASE_TYPES.read_text(encoding='utf-8').replace("  full_text: string;\n}","  full_text: string;\n  full_text_ru?: string | null;\n  translation_status?: 'translated_from_extracted_text' | 'pending_translation' | 'no_source';\n  translation_notes?: string[];\n}").replace("schema_version: '1.0.0';","schema_version: '1.0.0' | '1.1.0';")
(ROOT/'gic_types.localized.ts').write_text(typescript,encoding='utf-8')

stats=Counter(n['translation_status'] for n in updated['topics'])
orig_sha=hashlib.sha256(INPUT.read_bytes()).hexdigest()
meta = {'dataset_id':updated['dataset_id'],'source_dataset':original['dataset_id'],'source_sha256':orig_sha,'counts':dict(stats),'source_chars':sum(len(n.get('full_text','')) for n in original['topics']),'translated_source_chars':sum(len(n['full_text']) for n in updated['topics'] if n['translation_status']=='translated_from_extracted_text'),'ru_chars':sum(len(n.get('full_text_ru') or '') for n in updated['topics']),'translation_method':'expert-style manual draft against previously extracted English slides; no independent complete visual PDF audit','all_prerequisites_unchanged':True,'all_diagnostic_groups_unchanged':True}
(ROOT/'build_report.json').write_text(json.dumps(meta,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(json.dumps(meta,ensure_ascii=False,indent=2))
