"""Measure cold conversion and cache reads without changing server/user data.

Run with the service dependencies and PYTHONPATH pointing at the implementation
under comparison. Each invocation uses a fresh temporary document cache.
"""
import argparse
import hashlib
import importlib.metadata
import io
import json
import os
from pathlib import Path
import platform
import resource
import tempfile
import time
from types import SimpleNamespace

from pypdf import PdfReader, PdfWriter

from course_graph.ocr.converter import convert_document, get_ocr_configuration
from course_graph.ocr.enrichment import AuditedEngine, NativePdfTextReader


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("pdf", type=Path)
    parser.add_argument("--pages", required=True, help="Physical page numbers, e.g. 1,3,5")
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--native-only", action="store_true", help="Measure 200 reads per selected page without OCR models")
    args = parser.parse_args()
    pages = [int(value) for value in args.pages.split(",")]
    source = args.pdf.read_bytes()
    reader = PdfReader(io.BytesIO(source))
    if any(page < 1 or page > len(reader.pages) for page in pages):
        parser.error("Page numbers must be within the physical PDF page range")
    writer = PdfWriter()
    for page in pages:
        writer.add_page(reader.pages[page - 1])
    stream = io.BytesIO()
    writer.write(stream)
    content = stream.getvalue()
    if args.native_only:
        from docling_core.types.doc import BoundingBox, CoordOrigin
        native = NativePdfTextReader(content)
        started = time.perf_counter()
        texts = []
        try:
            for index, page_no in enumerate(pages):
                box = reader.pages[page_no - 1].mediabox
                item = SimpleNamespace(prov=[SimpleNamespace(page_no=index + 1, bbox=BoundingBox(
                    l=float(box.left), t=float(box.top), r=float(box.right), b=float(box.bottom), coord_origin=CoordOrigin.BOTTOMLEFT))])
                values = [native(item) for _ in range(200)]
                assert len(set(values)) == 1
                texts.append(values[0])
        finally:
            native.close()
        report = {"reads": 200 * len(pages), "seconds": time.perf_counter() - started,
                  "source_sha256": hashlib.sha256(source).hexdigest(), "pages": pages,
                  "sha256": hashlib.sha256("\n".join(texts).encode()).hexdigest()}
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(report, indent=2))
        print(json.dumps(report), flush=True)
        return
    calls = []
    predict = AuditedEngine.predict_batch

    def measured(self, inputs):
        calls.append(len(inputs))
        return predict(self, inputs)

    AuditedEngine.predict_batch = measured
    with tempfile.TemporaryDirectory(prefix="ocr-benchmark-") as cache:
        os.environ["COURSE_GRAPH_OCR_CACHE_DIR"] = cache
        started = time.perf_counter()
        result = convert_document(args.pdf.name, content)
        cold = time.perf_counter() - started
        cold_calls = len(calls)
        timings = []
        for _ in range(5):
            started = time.perf_counter()
            warm = convert_document(args.pdf.name, content)
            timings.append(time.perf_counter() - started)
            assert warm["markdown"] == result["markdown"], "Cache changed extracted text"
        report = {
            "source": args.pdf.name,
            "source_sha256": hashlib.sha256(source).hexdigest(),
            "pages": pages,
            "cold_seconds": cold,
            "cache_read_seconds": timings,
            "cold_inference_batches": cold_calls,
            "warm_inference_batches": len(calls) - cold_calls,
            "cache_hit": warm["report"]["cache_hit"],
            "markdown_sha256": hashlib.sha256(result["markdown"].encode()).hexdigest(),
            "peak_rss_kib": resource.getrusage(resource.RUSAGE_SELF).ru_maxrss,
            "environment": {
                "python": platform.python_version(),
                "packages": {name: importlib.metadata.version(name) for name in ["docling", "torch", "pypdfium2", "pypdf"]},
                "configuration": get_ocr_configuration(),
                "ocr_threads": os.environ.get("COURSE_GRAPH_OCR_THREADS", "4"),
            },
            "report": result["report"],
        }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2))
    args.output.with_suffix(".md").write_text(result["markdown"])
    print(json.dumps({key: report[key] for key in ["source", "pages", "cold_seconds", "cache_hit", "cold_inference_batches", "warm_inference_batches", "markdown_sha256"]}), flush=True)


if __name__ == "__main__":
    main()
