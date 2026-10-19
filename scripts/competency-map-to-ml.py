#!/usr/bin/env python3
"""Converts a competency map in the course template (Субкомпетенция, Уровень по
Блуму, Задание + Критерии оценивания) into the ML map CSV that the learning
release v0.0.2.x imports at /admin/competency-map.

    python scripts/competency-map-to-ml.py map.xlsx [-o map.csv] [--sheet NAME]

Standard library only. The mapping and its losses are listed in the --help text.
"""

import argparse
import csv
import re
import sys
import zipfile
import xml.etree.ElementTree as ET
from pathlib import Path

MAIN = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
RELS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

# Source columns are found by prefix: the template adds English glosses in brackets.
SOURCE = {
    "competency": "Субкомпетенция",
    "level": "Уровень темы",
    "constituent": "Составляющая",
    "outcome": "Образовательный результат",
    "bloom": "Уровень по Блуму",
    "ald": "Уровень ALDs",
    "importance": "Важность",
    "include": "Что должно войти в тест",
    "materials": "Тема курса",
    "task": "Задание",
    "criteria": "Критерии оценивания",
}
REQUIRED = ("competency", "constituent", "outcome", "bloom", "ald", "importance", "include", "task", "criteria")

# The importer knows four taxonomy levels; the upper Bloom levels become «Анализ».
TAXONOMY = {
    "помнить": "Знание",
    "понимать": "Понимание",
    "применять": "Применение",
    "анализировать": "Анализ",
    "оценивать": "Анализ",
    "создавать": "Анализ",
}
LEVELS = {"базовый", "средний", "продвинутый"}
DEFAULT_LEVEL = "средний"

HEADERS = [
    "",
    "Компетенция",
    "Уровень темы",
    "Составляющая",
    "Образовательный результат",
    "Что должно войти в тест",
    "Таксономия",
    "Уровень ALDs",
    "Важность",
    "Раздел РПД · компетенции РПД",
    "Задание 1",
    "ОС",
]

# A task cell holds four blocks separated by two blank lines.
BLOCK_SEPARATOR = "\n\n\n"
TTS_LIMIT = 500


def column_index(ref):
    index = 0
    for ch in re.match(r"[A-Z]+", ref).group():
        index = index * 26 + ord(ch) - 64
    return index - 1


def read_workbook(path):
    """Returns {sheet name: [(row number, [cell text, ...]), ...]} in book order."""
    book = zipfile.ZipFile(path)
    ns = {"m": MAIN}
    shared = []
    if "xl/sharedStrings.xml" in book.namelist():
        for item in ET.fromstring(book.read("xl/sharedStrings.xml")).findall("m:si", ns):
            shared.append("".join(t.text or "" for t in item.iter(f"{{{MAIN}}}t")))
    workbook = ET.fromstring(book.read("xl/workbook.xml"))
    targets = {r.get("Id"): r.get("Target") for r in ET.fromstring(book.read("xl/_rels/workbook.xml.rels"))}
    sheets = {}
    for sheet in workbook.find("m:sheets", ns):
        target = targets[sheet.get(f"{{{RELS}}}id")].lstrip("/")
        if not target.startswith("xl/"):
            target = "xl/" + target
        rows = []
        for row in ET.fromstring(book.read(target)).findall(".//m:sheetData/m:row", ns):
            cells = {}
            for cell in row.findall("m:c", ns):
                value, kind = cell.find("m:v", ns), cell.get("t")
                if kind == "s" and value is not None:
                    text = shared[int(value.text)]
                elif kind == "inlineStr":
                    text = "".join(t.text or "" for t in cell.iter(f"{{{MAIN}}}t"))
                elif kind == "b" and value is not None:
                    text = "TRUE" if value.text == "1" else "FALSE"
                else:
                    text = value.text if value is not None and value.text else ""
                cells[column_index(cell.get("r"))] = text
            width = max(cells) + 1 if cells else 0
            rows.append((int(row.get("r")), [cells.get(i, "") for i in range(width)]))
        sheets[sheet.get("name")] = rows
    return sheets


def find_columns(header):
    columns = {}
    for key, prefix in SOURCE.items():
        for i, title in enumerate(header):
            if title.strip().startswith(prefix):
                columns[key] = i
                break
    return columns


def pick_sheet(sheets, name):
    """The named sheet, or the first one with the template headers."""
    for sheet_name, rows in sheets.items():
        if name is not None and sheet_name != name:
            continue
        for index, (_, row) in enumerate(rows):
            if not any(cell.strip() for cell in row):
                continue
            columns = find_columns(row)
            if all(key in columns for key in REQUIRED):
                return sheet_name, rows[index + 1 :], columns
            break
    available = ", ".join(f"«{s}»" for s in sheets)
    if name is not None:
        sys.exit(f"Лист «{name}» не найден или без заголовков карты. Листы книги: {available}.")
    sys.exit(f"Не найден лист с заголовками карты (Субкомпетенция, Составляющая, Образовательный результат…). Листы: {available}.")


def block(text):
    """One non-empty block without the separator inside."""
    return re.sub(r"\n{3,}", "\n\n", text.replace("\r\n", "\n").strip())


def importance(value):
    try:
        number = float(value.replace(",", "."))
    except ValueError:
        return None
    if number != int(number) or not 1 <= number <= 5:
        return None
    return str(int(number))


def convert(rows, columns):
    """Returns (CSV rows, warnings, errors)."""
    out, warnings, errors = [], [], []
    skipped = []
    competency = constituent = level = materials = ""
    levels_by_competency = {}
    seen_constituents = set()

    def cell(row, key):
        index = columns.get(key)
        return row[index].strip() if index is not None and index < len(row) else ""

    for line, row in rows:
        if not any(value.strip() for value in row):
            continue
        if cell(row, "competency"):
            competency = cell(row, "competency")
            level = levels_by_competency.get(competency, "")
        if cell(row, "constituent") and cell(row, "constituent") != constituent:
            constituent = cell(row, "constituent")
            materials = ""
        if cell(row, "level"):
            level = cell(row, "level").lower()
            if level not in LEVELS:
                errors.append(f"строка {line}: уровень темы «{cell(row, 'level')}» — ожидается базовый, средний или продвинутый")
            levels_by_competency[competency] = level
        if cell(row, "materials"):
            materials = cell(row, "materials")

        outcome = cell(row, "outcome")
        if not outcome:
            skipped.append(line)
            continue
        if not competency or not constituent:
            errors.append(f"строка {line}: ОР без субкомпетенции или составляющей выше")
            continue

        # The importer needs a topic level on the first row of every constituent.
        row_level = ""
        if (competency, constituent) not in seen_constituents:
            seen_constituents.add((competency, constituent))
            row_level = level or DEFAULT_LEVEL
            if not level:
                warnings.append(f"строка {line}: у составляющей «{constituent[:60]}» нет уровня темы — поставлен «{DEFAULT_LEVEL}»")

        taxonomy = TAXONOMY.get(cell(row, "bloom").lower())
        if taxonomy is None:
            errors.append(f"строка {line}: уровень по Блуму «{cell(row, 'bloom')}» не распознан")
        ald = cell(row, "ald").lower()
        if ald not in LEVELS:
            errors.append(f"строка {line}: уровень ALDs «{cell(row, 'ald')}» — ожидается базовый, средний или продвинутый")
        weight = importance(cell(row, "importance"))
        if weight is None:
            errors.append(f"строка {line}: важность «{cell(row, 'importance')}» — ожидается целое от 1 до 5")
        include = cell(row, "include").upper()
        if include not in ("TRUE", "FALSE"):
            errors.append(f"строка {line}: «Что должно войти в тест» = «{cell(row, 'include')}» — ожидается TRUE или FALSE")

        task, criteria = block(cell(row, "task")), block(cell(row, "criteria"))
        task_cell = ""
        if task and criteria:
            # Open oral task: no options; the voice instruction is the task itself.
            task_cell = BLOCK_SEPARATOR.join([task, "[]", task, criteria])
            if len(task) > TTS_LIMIT:
                warnings.append(f"строка {line}: задание длиннее {TTS_LIMIT} символов ({len(task)}) — озвучка его не прочитает")
        elif task or criteria:
            warnings.append(f"строка {line}: есть только {'задание' if task else 'критерии'} — ОР импортируется без задания")
        elif include == "TRUE":
            warnings.append(f"строка {line}: ОР с TRUE без задания — в вариант не попадёт")

        out.append([
            "",
            competency,
            row_level,
            constituent,
            outcome,
            include,
            taxonomy or "",
            ald,
            weight or "",
            "",
            task_cell,
            materials,
        ])
    if skipped:
        warnings.append(f"пропущено строк без ОР: {len(skipped)} (первая — {skipped[0]})")
    return out, warnings, errors


def main():
    parser = argparse.ArgumentParser(
        description="Карта компетенций в шаблоне курса → CSV ML-карты для импорта в «Обучение» (v0.0.2.x).",
        epilog=(
            "Соответствие: Субкомпетенция → Компетенция; Уровень по Блуму → Таксономия "
            "(Помнить → Знание, Понимать → Понимание, Применять → Применение, "
            "Анализировать/Оценивать/Создавать → Анализ); Важность → целое; "
            "Задание + Критерии оценивания → «Задание 1» из четырёх блоков: вопрос, варианты [], "
            "голосовая инструкция (текст задания), эталон (критерии); "
            "Тема курса · недели · материалы → ОС (на всю составляющую). "
            "Уровень темы переносится на все составляющие субкомпетенции. "
            "Раздел РПД остаётся пустым; Свидетельство, DOK, Соответствие целям, Код и Реакция не переносятся."
        ),
    )
    parser.add_argument("input", type=Path, help="карта .xlsx")
    parser.add_argument("-o", "--output", type=Path, help="куда записать CSV [рядом с input, .ml.csv]")
    parser.add_argument("--sheet", help="лист с картой [первый лист с заголовками шаблона]")
    args = parser.parse_args()

    try:
        sheets = read_workbook(args.input)
    except (OSError, zipfile.BadZipFile, KeyError, ET.ParseError) as err:
        sys.exit(f"Не удалось прочитать {args.input}: {err}")
    sheet, rows, columns = pick_sheet(sheets, args.sheet)
    out, warnings, errors = convert(rows, columns)

    for warning in warnings:
        print(f"Предупреждение: {warning}", file=sys.stderr)
    if errors:
        for error in errors:
            print(f"Ошибка: {error}", file=sys.stderr)
        sys.exit(f"CSV не записан: {len(errors)} ошибок на листе «{sheet}».")
    if not out:
        sys.exit(f"На листе «{sheet}» нет ОР.")

    output = args.output or args.input.with_suffix(".ml.csv")
    # BOM keeps Cyrillic readable when the file is opened in Excel; the importer strips it.
    with open(output, "w", encoding="utf-8-sig", newline="") as file:
        writer = csv.writer(file, lineterminator="\n")
        writer.writerow(HEADERS)
        writer.writerows(out)
    tasks = sum(1 for row in out if row[10])
    in_test = sum(1 for row in out if row[5] == "TRUE")
    print(f"Лист «{sheet}»: {len(out)} ОР, {tasks} с заданием, {in_test} с TRUE → {output}")


if __name__ == "__main__":
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8")
        sys.stderr.reconfigure(encoding="utf-8")
    main()
