"""Recover literal labels inside regions that Docling exports as pictures."""
import re


def compact(value):
    return re.sub(r"\s+", "", value).casefold()


def picture_texts(document, converter, page_markdown):
    readers = [p.ocr_model.reader for p in converter.initialized_pipelines.values()
               if hasattr(getattr(p, "ocr_model", None), "reader")]
    recovered = {}
    metrics = {"regions": 0, "lines": 0, "failed_regions": 0}
    if not readers:
        return recovered, metrics
    for index, picture in enumerate(document.pictures):
        if not picture.prov:
            continue
        page = picture.prov[0].page_no
        try:
            image = picture.get_image(document)
            if image is None:
                metrics["failed_regions"] += 1
                continue
            metrics["regions"] += 1
            output = readers[0](image)
            lines = []
            existing = compact(page_markdown.get(page, ""))
            for text, score in zip(output.txts or (), output.scores or ()):
                text = text.strip()
                if score >= .85 and len(re.findall(r"\w", text)) >= 3 and compact(text) not in existing:
                    lines.append(text)
            if lines:
                recovered.setdefault(page, []).append((index+1, lines))
                metrics["lines"] += len(lines)
        except Exception:
            metrics["failed_regions"] += 1
    return recovered, metrics
