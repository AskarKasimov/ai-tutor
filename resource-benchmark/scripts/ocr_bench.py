"""One independent OCR run in a fresh course-graph container (see ocr.sh).

Phases, timed separately:
  init       converter + layout/OCR/VLM models loaded and initialized, as the
             service bootstrap does (weights already in the HF cache volume)
  first      first conversion in the process (CUDA kernels, lazy parts) of a
             1-page warm-up PDF, empty result cache
  cold       each document with warm models and an EMPTY result cache: a new
             temporary COURSE_GRAPH_OCR_CACHE_DIR per document
  cached     the same document again with its cache filled

The result cache is a temporary directory inside the container; the service OCR
cache volume is not mounted. Memory per document: peak VmRSS of this process
(sampled every 0.1 s) and, on GPU, torch.cuda.max_memory_allocated/reserved.
The container cgroup memory.peak (whole run) is stored as cgroup_memory_peak.
"""
import hashlib
import io
import json
import os
import sys
import tempfile
import threading
import time
from pathlib import Path


def rss_kib():
    for line in open("/proc/self/status"):
        if line.startswith("VmRSS:"):
            return int(line.split()[1])
    return 0


class Peak:
    def __init__(self):
        self.value, self.stop = 0, threading.Event()
        threading.Thread(target=self.run, daemon=True).start()

    def run(self):
        while not self.stop.wait(0.1):
            self.value = max(self.value, rss_kib())

    def reset(self):
        self.value = rss_kib()


def main():
    out = Path(sys.argv[1])
    docs = [Path(p) for p in sys.argv[2:]]
    peak = Peak()
    report = {"device": os.environ.get("COURSE_GRAPH_DEVICE"), "threads": os.environ.get("COURSE_GRAPH_OCR_THREADS"),
              "started": time.time(), "documents": []}

    started = time.perf_counter()
    import torch
    from docling.datamodel.base_models import InputFormat
    from course_graph.ocr.converter import convert_document, get_converter, get_ocr_configuration
    from course_graph.ocr.enrichment import NativeSyntaxGuard
    torch.set_num_threads(int(os.environ.get("COURSE_GRAPH_OCR_THREADS", "4")))
    converter = get_converter()
    converter.initialize_pipeline(InputFormat.PDF)
    for pipeline in converter.initialized_pipelines.values():
        for stage in getattr(pipeline, "enrichment_pipe", []):
            if isinstance(stage, NativeSyntaxGuard):
                stage.engine.initialize()
    report["init_seconds"] = time.perf_counter() - started
    report["init_peak_rss_kib"] = peak.value
    report["configuration"] = get_ocr_configuration()
    gpu = torch.cuda.is_available() and os.environ.get("COURSE_GRAPH_DEVICE") == "gpu"
    report["torch"] = torch.__version__
    report["cuda_device"] = torch.cuda.get_device_name(0) if gpu else None

    def convert(name, content):
        with tempfile.TemporaryDirectory(prefix="ocr-bench-cache-") as cache:
            os.environ["COURSE_GRAPH_OCR_CACHE_DIR"] = cache
            if gpu:
                torch.cuda.reset_peak_memory_stats()
            peak.reset()
            t = time.perf_counter()
            result = convert_document(name, content)
            cold = time.perf_counter() - t
            cold_peak = peak.value
            t = time.perf_counter()
            again = convert_document(name, content)
            cached = time.perf_counter() - t
        entry = {"cold_seconds": cold, "cached_seconds": cached, "cache_hit": again["report"].get("cache_hit"),
                 "peak_rss_kib": cold_peak, "markdown_chars": len(result["markdown"]),
                 "markdown_sha256": hashlib.sha256(result["markdown"].encode()).hexdigest(),
                 "engine": result["report"].get("engine"), "warnings": len(result["report"].get("warnings") or []),
                 "content_complete": result["report"].get("content_complete"),
                 "work_complete": result["report"].get("work_complete")}
        if gpu:
            entry["cuda_max_allocated_mib"] = torch.cuda.max_memory_allocated() / 2**20
            entry["cuda_max_reserved_mib"] = torch.cuda.max_memory_reserved() / 2**20
        return entry, result["markdown"]

    from pypdf import PdfReader, PdfWriter
    first = PdfWriter()
    first.add_page(PdfReader(str(docs[0])).pages[0])
    stream = io.BytesIO()
    first.write(stream)
    warm, _ = convert("warmup-page1.pdf", stream.getvalue())
    report["first_conversion"] = warm

    for path in docs:
        content = path.read_bytes()
        entry, markdown = convert(path.name, content)
        entry.update({"file": path.name, "sha256": hashlib.sha256(content).hexdigest(), "bytes": len(content),
                      "pages": len(PdfReader(io.BytesIO(content)).pages)})
        entry["seconds_per_page"] = entry["cold_seconds"] / entry["pages"]
        report["documents"].append(entry)
        (out.parent / (out.stem + "-" + path.stem.replace(" ", "_") + ".md")).write_text(markdown)
        print(json.dumps({k: entry[k] for k in ("file", "pages", "cold_seconds", "cached_seconds", "peak_rss_kib")}),
              flush=True)
    report["peak_rss_kib"] = max([report["init_peak_rss_kib"]] + [d["peak_rss_kib"] for d in report["documents"]])
    report["finished"] = time.time()
    # Whole-container peak (cgroup v2 inside the container's own namespace).
    for name in ("memory.peak", "memory.swap.peak"):
        try:
            report["cgroup_" + name.replace(".", "_")] = int(open("/sys/fs/cgroup/" + name).read())
        except OSError:
            pass
    out.write_text(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
