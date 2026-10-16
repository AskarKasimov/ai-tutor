"""Сборка русской локализации Gic v1.5 без изменения структуры графа."""
from __future__ import annotations
import csv
from copy import deepcopy
from hashlib import sha256
import json
from pathlib import Path
import re
import sys
from collections import Counter
from typing import Callable

ROOT=Path(__file__).parent
BASE=ROOT/'baseline_course_graph.v1.4.json'
OUT=ROOT/'course_graph.ru-draft.v1.5.json'
BLOCK=re.compile(r'(?ms)^(\[D\d+ \| [^\n]+\])\n(.*?)(?=^\[D\d+ \||\Z)')
S='\n---PAGE---\n'
from translations_a import TRANSLATIONS as A
from translations_b import TRANSLATIONS as B
from translations_c import TRANSLATIONS as C
from translations_d import TRANSLATIONS as D
from translations_e import TRANSLATIONS as E
from translations_f import TRANSLATIONS as F
from translations_g import TRANSLATIONS as G
from translations_h import TRANSLATIONS as H

TRANSLATIONS=A|B|C|D|E|F|G|H

def dump(path:Path, obj:object) -> None:
    path.write_text(json.dumps(obj, ensure_ascii=False,indent=2)+'\n',encoding='utf-8')

def fragments(body:str)->list[tuple[str,str]]:
    return [(a,b.strip()) for a,b in BLOCK.findall(body)]

def auto_translate(token:str, original:str)->str:
    if token=='@@AUTO_GRID@@':
        return original.replace('Cut Examples','Примеры альфа-бета-отсечений').replace('MAX','MAX (максимизирующий игрок)').replace('MIN','MIN (минимизирующий игрок)')
    if token=='@@AUTO_PUZZLE@@':
        return original.replace('Hill Climbers on 8-puzzle Problem','Поиск hill climbing в головоломке 8-puzzle').replace('Goal state','Целевое состояние').replace('Alternative Heuristics?','Альтернативные эвристики?').replace('Test it by yourself','Проверьте самостоятельно')
    raise ValueError(token)

def main()->None:
    source=json.loads(BASE.read_text(encoding='utf-8'))
    out=deepcopy(source)
    pending={t['id']:t for t in out['topics'] if t['translation_status']=='pending_translation'}
    assert len(pending)==22,(len(pending),sorted(pending))
    assert len(TRANSLATIONS)==22, (len(TRANSLATIONS),sorted(TRANSLATIONS))
    assert set(TRANSLATIONS)==set(pending)
    memory={}
    for t in out['topics']:
        if t.get('full_text_ru'):
            en=fragments(t['full_text']); ru=fragments(t['full_text_ru'])
            assert [h for h,_ in en]==[h for h,_ in ru],t['id']
            for (h,_),(_,r) in zip(en,ru):
                memory.setdefault(h,r)
    # Страница, которой нет в существующей памяти переводов.
    memory.setdefault('[D04 | PDDL (Lecture +Tutorial).pdf | с.15 | shared]',
        """Синтаксис PDDL: файл предметной области `domain.pddl`.
```
(define (domain <domain-name>)
 (:requirements <requirements-list>)
 (:types <types-list>)
 (:predicates <predicate-list>)
 (:action <action-details>)
)
```
`<domain-name>` — имя моделируемого мира.
`:requirements` — используемые возможности PDDL.
`:types` — необязательное описание типов объектов.
`:predicates` — список свойств состояния, истинных или ложных фактов.
`:action` — список допустимых переходов между состояниями.
[Ключевые слова PDDL и имена плейсхолдеров сохранены без перевода.]""")
    staged={}
    for ident in pending:
        t=pending[ident]
        en=fragments(t['full_text'])
        instructions=TRANSLATIONS[ident]
        parts=[instructions(v,i) for i,(h,v) in enumerate(en)] if callable(instructions) else instructions.split(S)
        assert len(en)==len(parts),(ident,len(en),len(parts))
        staged[ident]=(en,parts)
        for (header,eng),translated in zip(en,parts):
            if translated.strip() not in ('@@REUSE@@',):
                if translated.startswith('@@AUTO_'):
                    translated=auto_translate(translated,eng)
                assert len(translated.strip())>12,(ident,header)
                memory.setdefault(header,translated.strip())
    missing=[]
    for ident,(en,parts) in staged.items():
        result=[]
        for (header,eng),part in zip(en,parts):
            if part=='@@REUSE@@':
                part=memory.get(header)
                if not part:
                    missing.append((ident,header))
                    continue
            elif part.startswith('@@AUTO_'):
                part=auto_translate(part,eng)
            result.append((header,part.strip()))
        if len(result)==len(en):
            t=pending[ident]
            t['full_text_ru']='\n\n'.join(f'{h}\n{b}' for h,b in result)
            t['translation_status']='translated_from_extracted_text'
            t['translation_notes']=(t.get('translation_notes') or [])+[
              'Редакторский перевод извлечённого текста по всем привязанным страницам; оригинал сохранён без изменений.',
              'Текст внутри квадратных скобок — пояснение переводчика о повреждённом извлечении, неоднозначности или ошибке исходного слайда.',
              'Числовые состояния игровых полей, псевдокод, термы и примеры оставлены без искажения чисел/синтаксиса. Графические рисунки без текстового слоя требуют сверки PDF.',
            ]
            assert [h for h,_ in fragments(t['full_text_ru'])]==[h for h,_ in en], ident
    if missing:
        raise ValueError('Not found for reuse: '+str(missing))
    for x,y in zip(source['topics'],out['topics']):
        assert x['id']==y['id']
        if x['id'] in pending:
            for k in x:
                if k not in ('full_text_ru','translation_status','translation_notes'):
                    assert x[k]==y[k], (x['id'],k)
        else:assert x==y, x['id']
    for k in source:
        if k!='topics':assert source[k]==out[k],k
    statuses=Counter(t['translation_status'] for t in out['topics'])
    assert statuses=={'translated_from_extracted_text':132,'no_source':5},statuses
    dump(OUT,out)
    with (ROOT/'translation_patch_batch5.jsonl').open('w',encoding='utf-8') as f:
        for t in out['topics']:
            if t['id'] in pending:
                f.write(json.dumps({k:t[k] for k in ('id','full_text_ru','translation_status','translation_notes')},ensure_ascii=False)+'\n')
    cards=ROOT/'translated_topics_batch5';cards.mkdir(exist_ok=True)
    for t in out['topics']:
        if t['id'] in pending:
            (cards/f'{t["id"]}.md').write_text(f'# {t["id"]} — {t["title"]}\n\n## Русский текст\n\n{t["full_text_ru"]}\n\n## Английский оригинал\n\n{t["full_text"]}\n',encoding='utf-8')
    with (ROOT/'translation_status.csv').open('w',newline='',encoding='utf-8-sig') as f:
        writer=csv.writer(f);writer.writerow(['id','title','translation_status','english_characters','russian_characters','page_count','new_in_batch5'])
        for t in out['topics']:
            writer.writerow([t['id'],t['title'],t['translation_status'],len(t.get('full_text') or ''),len(t.get('full_text_ru') or ''),len(t.get('source_pages') or []),t['id'] in pending])
    total=sum(len(t.get('full_text') or '') for t in out['topics'])
    report={
        'before_sha256':sha256(BASE.read_bytes()).hexdigest(),
        'after_sha256':sha256(OUT.read_bytes()).hexdigest(),
        'topics':len(out['topics']),'topics_with_material':132,'without_source':5,
        'previously_translated':110,'newly_translated':22,'translated_total':132,'pending':0,
        'english_source_characters_total':total,'source_char_coverage_percent':100.0,
        'english_source_characters_new_batch':sum(len(t['full_text']) for t in out['topics'] if t['id'] in pending),
        'prerequisites_count':len(out['prerequisites']),
        'diagnostic_groups_count':len(out['diagnostic_groups']),
        'original_nodes_and_edges_unchanged':True,
        'source_marker_sequences_verified':True,
        'newly_translated_ids':sorted(pending),
    }
    dump(ROOT/'build_report.json',report)
    print(json.dumps(report,ensure_ascii=False,indent=2))

if __name__=='__main__':main()
