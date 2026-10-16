"""Import completed machine translation jobs without overwriting approved draft translations.

Accepted JSONL rows: {"id":"AI-...", "full_text_ru":"..."}.
The importer preserves every English source, original topic, graph edge and gate.
New outputs are explicitly labelled translated_draft_unreviewed.
"""
from __future__ import annotations

import argparse
import csv
import json
import re
from pathlib import Path
import jsonschema

HERE = Path(__file__).resolve().parent
MARKER = re.compile(r'^\[D\d+ \|[^\n]+\| с\.\d+ \|[^\n]+\]$', re.M)

def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument('completed_jsonl', type=Path)
    parser.add_argument('--output', type=Path, default=HERE/'course_graph.ru-imported.v1.1.json')
    args = parser.parse_args()
    graph = json.loads((HERE/'course_graph.ru-draft.v1.1.json').read_text(encoding='utf-8'))
    schema = json.loads((HERE/'course_graph.ru-draft.schema.json').read_text(encoding='utf-8'))
    nodes = {node['id']:node for node in graph['topics']}
    seen: set[str] = set()
    count = 0
    for raw_line in args.completed_jsonl.read_text(encoding='utf-8').splitlines():
        if not raw_line.strip():
            continue
        payload = json.loads(raw_line)
        identifier = payload['id']
        translation = payload['full_text_ru']
        if identifier in seen:
            raise ValueError(f'Duplicate ID {identifier}')
        seen.add(identifier)
        if identifier not in nodes:
            raise ValueError(f'Unknown ID {identifier}')
        node = nodes[identifier]
        if node['translation_status'] != 'pending_translation':
            raise ValueError(f'{identifier} is not awaiting translation; refusing overwrite')
        if not isinstance(translation,str) or len(translation)<20:
            raise ValueError(f'Translation is empty/too short for {identifier}')
        if MARKER.findall(translation) != MARKER.findall(node['full_text']):
            raise ValueError(f'Source page markers differ for {identifier}')
        if not re.search(r'[А-Яа-яЁё]', translation):
            raise ValueError(f'Russian text not found for {identifier}')
        node['full_text_ru'] = translation
        node['translation_status'] = 'translated_draft_unreviewed'
        node['translation_notes'] = ['Черновой перевод импортирован автоматически. Нужны проверка смысла, формул и ссылок на оригинальные страницы.']
        count += 1
    jsonschema.validate(graph,schema)
    args.output.write_text(json.dumps(graph,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(f'Imported {count} translation drafts into {args.output}')

if __name__ == '__main__':
    main()
