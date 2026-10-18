"""Keep native program syntax and make Docling enrichment failures observable."""

import threading
import re
from collections import OrderedDict

CODE_MISMATCH_WARNING = "Распознанный код отличается от текстового слоя PDF; проверьте исходник"


def _code_tokens(text):
    # Keep quoted literals intact while ignoring layout whitespace outside them.
    return re.findall(r"'(?:\\.|[^'\\])*'|\"(?:\\.|[^\"\\])*\"|\w+|[^\w\s]", text)


class AuditedEngine:
    def __init__(self, engine, warnings):
        self.engine = engine
        self.warnings = warnings

    def initialize(self):
        self.engine.initialize()
        model = getattr(self.engine, "vlm_model", None)
        if model is not None and self.engine.options.use_kv_cache:
            model.generation_config.use_cache = True

    def predict_batch(self, inputs):
        self.initialize()
        try:
            outputs = self.engine.predict_batch(inputs)
        except Exception as error:
            self.warnings.append("Код/формулы обработаны частично: " + type(error).__name__ + ": " + str(error)[:200])
            # Docling retains original items when its inference engine raises.
            raise

        if len(outputs) != len(inputs):
            self.warnings.append("Код/формулы обработаны частично: ValueError: enrichment output count mismatch")
            raise ValueError("enrichment output count mismatch")

        for request, output in zip(inputs, outputs):
            if not output.text.strip():
                self.warnings.append("Код/формулы: пустой ответ модели")
                raise ValueError("empty enrichment output")
            if (output.metadata or {}).get("num_tokens", 0) >= request.max_new_tokens:
                self.warnings.append("Код/формулы: достигнут лимит токенов модели")
                raise ValueError("enrichment token limit reached")
        return outputs


class NativeSyntaxGuard:
    def __init__(self, stage, native_text_for=None):
        self.stage = stage
        self.native_text_for = native_text_for
        self.warnings = []
        stage.engine = AuditedEngine(stage.engine, self.warnings)

    def __getattr__(self, name):
        return getattr(self.stage, name)

    @staticmethod
    def _normalized_native(text):
        if not isinstance(text, str) or not text.strip() or "\ufffd" in text:
            return None
        if any(ord(char) < 32 and char not in "\r\n\t" for char in text):
            return None
        return text.replace("\r\n", "\n").replace("\r", "\n").strip()

    def _matches_native_pdf(self, item):
        native_text = self._native_pdf_text(item)
        item_text = self._normalized_native(getattr(item, "text", None))
        exact = native_text is not None and item_text is not None and native_text == item_text
        return exact or self._prolog_layout_matches(native_text, item_text), native_text

    @staticmethod
    def _prolog_layout_matches(native, extracted):
        # Docling joins native PDF lines with spaces. Limit this exception to
        # complete, single-line Prolog rules and their numbered solution labels.
        # Operators, quoted text and intra-line whitespace must match exactly;
        # indentation-sensitive languages and wrapped clauses keep visual OCR.
        if not native or not extracted or ":-" not in native:
            return False
        lines = [line.strip() for line in native.splitlines() if line.strip()]
        if not all(re.fullmatch(r"Solution\s+\d+\.", line) or
                   re.fullmatch(r"[a-z]\w*\([^\n]*\)\s*:-\s*.+\.", line) for line in lines):
            return False
        return " ".join(lines) == extracted

    def _native_pdf_text(self, item):
        if self.native_text_for is None:
            return None
        try:
            return self._normalized_native(self.native_text_for(item))
        except Exception:
            return None

    def __call__(self, doc, element_batch):
        from docling_core.types.doc import DocItemLabel

        elements = list(element_batch)
        preserved = []
        native_code = {}
        for element in elements:
            item = element.item
            if item.label not in (DocItemLabel.CODE, DocItemLabel.FORMULA) or not item.text.strip():
                continue
            matches, native = self._matches_native_pdf(item)
            if item.label == DocItemLabel.CODE and native is not None:
                native_code[id(item)] = native
            if matches:
                if item.label == DocItemLabel.CODE:
                    if self._prolog_layout_matches(native, self._normalized_native(item.text)):
                        item.text = native
                    preserved.append(element)
                elif item.label == DocItemLabel.FORMULA and native is not None and ":-" in native:
                    item.text = native
                    item.label = DocItemLabel.TEXT
                    self.warnings.append("Правило Prolog ошибочно размечено как формула; сохранён исходный синтаксис")
                    preserved.append(element)
        preserved_ids = {id(element) for element in preserved}
        enrich = [element for element in elements if id(element) not in preserved_ids]
        output = list(self.stage(doc, enrich)) if enrich else []
        output_by_id = {id(item): item for item in output}
        for element in elements:
            item = element.item if id(element) in preserved_ids else output_by_id.get(id(element.item), element.item)
            native = native_code.get(id(element.item))
            if native is not None and _code_tokens(native) != _code_tokens(item.text):
                self.warnings.append(CODE_MISMATCH_WARNING)
            yield item


class NativePdfTextReader:
    """Read native characters from the same PDF region Docling classified."""

    def __init__(self, pdf_content):
        import pypdfium2 as pdfium
        self._pdf = pdfium.PdfDocument(pdf_content)
        self._lock = threading.Lock()
        self._pages = OrderedDict()

    def __call__(self, item):
        provenance = getattr(item, "prov", None)
        if not provenance or len(provenance) != 1:
            return None
        proof = provenance[0]
        if not 1 <= proof.page_no <= len(self._pdf):
            return None
        with self._lock:
            cached = self._pages.get(proof.page_no)
            if cached is None:
                page = self._pdf[proof.page_no - 1]
                try:
                    textpage = page.get_textpage()
                except Exception:
                    page.close()
                    raise
                cached = (page, textpage)
                self._pages[proof.page_no] = cached
                # Docling may interleave pages; keep a bounded working set.
                if len(self._pages) > 4:
                    _, (old_page, old_text) = self._pages.popitem(last=False)
                    old_text.close()
                    old_page.close()
            self._pages.move_to_end(proof.page_no)
            page, textpage = cached
            bbox = proof.bbox.to_bottom_left_origin(page.get_height())
            return textpage.get_text_bounded(
                left=bbox.l - 0.5, bottom=bbox.b - 0.5,
                right=bbox.r + 0.5, top=bbox.t + 0.5,
            )

    def close(self):
        with self._lock:
            while self._pages:
                _, (page, textpage) = self._pages.popitem()
                textpage.close()
                page.close()
            self._pdf.close()


def audited_pipeline_class():
    from docling.pipeline.standard_pdf_pipeline import StandardPdfPipeline

    class AuditedPdfPipeline(StandardPdfPipeline):
        def _init_models(self):
            super()._init_models()
            self.enrichment_pipe = [NativeSyntaxGuard(stage) if getattr(stage, "engine", None) is not None
                                    and type(stage).__name__ == "CodeFormulaVlmModel" else stage
                                    for stage in self.enrichment_pipe]

    return AuditedPdfPipeline


def enrichment_warnings(converter, reset=False):
    warnings = []
    for pipeline in getattr(converter, "initialized_pipelines", {}).values():
        for stage in getattr(pipeline, "enrichment_pipe", []):
            if isinstance(stage, NativeSyntaxGuard):
                warnings.extend(stage.warnings)
                if reset:
                    stage.warnings.clear()
    return list(dict.fromkeys(warnings))


def recover_native_programs(document, pdf_content):
    """Recover misclassified rules from the *same PDF rectangle*, never guess.

    Docling clears native text for some FORMULA items before enrichment. Reading
    their PDF character boxes is therefore necessary to preserve operators.
    """
    import pypdfium2 as pdfium
    from docling_core.types.doc import DocItemLabel, FormulaItem, TextItem

    warnings = []
    with pdfium.PdfDocument(pdf_content) as pdf:
        for index, item in enumerate(document.texts):
            is_formula = isinstance(item, FormulaItem) or item.label == DocItemLabel.FORMULA
            if not (item.label == DocItemLabel.CODE or is_formula) or not item.prov:
                continue
            prov = item.prov[0]
            if not 1 <= prov.page_no <= len(pdf):
                continue
            page = pdf[prov.page_no - 1]
            try:
                bbox = prov.bbox.to_bottom_left_origin(page.get_height())
                textpage = page.get_textpage()
                try:
                    native = textpage.get_text_bounded(
                        left=bbox.l-.5, bottom=bbox.b-.5,
                        right=bbox.r+.5, top=bbox.t+.5).strip().replace("\r\n", "\n")
                finally:
                    textpage.close()
            finally:
                page.close()
            if is_formula and ":-" in native:
                native_text = NativeSyntaxGuard._normalized_native(native)
                item_text = NativeSyntaxGuard._normalized_native(item.text)
                if native_text is None or item_text != native_text:
                    warnings.append("Правило в PDF не совпало с исходным текстом Docling; автоматическая подмена отклонена")
                    continue
                item.text = native_text
                item.label = DocItemLabel.TEXT
                if isinstance(item, FormulaItem):
                    # The Markdown serializer dispatches on the Python class,
                    # not its label; changing only label still adds $$ wrappers.
                    fields = {key:value for key,value in item.model_dump().items()
                              if key in TextItem.model_fields}
                    document.texts[index] = TextItem.model_validate(fields)
                warnings.append("Правило Prolog восстановлено из текстового слоя PDF по координатам ошибочно размеченной формулы")
    return warnings


def audit_and_retry_formulas(document, pdf_content: bytes, converter=None):
    """Audit recognized formulas for syntax balance, track regions, and retry crops if needed.

    Never guesses formulas from text context; only relies on native PDF text or visual VLM crops.
    """
    import hashlib
    from dataclasses import asdict
    import pypdfium2 as pdfium
    from docling_core.types.doc import DocItemLabel, FormulaItem
    from .regions import CanonicalBBox, FormulaRegionRecord
    from .syntax import check_latex_syntax, normalize_formatting_only, has_substantive_conflict

    regions = []
    warnings = []

    formula_items = []
    for item in getattr(document, "texts", []):
        if isinstance(item, FormulaItem) or getattr(item, "label", None) == DocItemLabel.FORMULA:
            formula_items.append(item)

    if not formula_items:
        return regions, warnings

    source_hash = hashlib.sha256(pdf_content).hexdigest()

    pdf = None
    try:
        pdf = pdfium.PdfDocument(pdf_content)
    except Exception as err:
        pass

    vlm_stage = None
    if converter and hasattr(converter, "initialized_pipelines"):
        for p in converter.initialized_pipelines.values():
            for stage in getattr(p, "enrichment_pipe", []):
                if getattr(stage, "engine", None) is not None and type(stage).__name__ in ("CodeFormulaVlmModel", "NativeSyntaxGuard"):
                    vlm_stage = stage
                    break
            if vlm_stage:
                break

    for idx, item in enumerate(formula_items):
        prov = item.prov[0] if getattr(item, "prov", None) else None
        page_no = prov.page_no if prov else 1

        page_w, page_h = 612.0, 792.0
        native_text = None
        if pdf and 1 <= page_no <= len(pdf):
            try:
                page = pdf[page_no - 1]
                try:
                    page_w, page_h = page.get_size()
                    if prov and prov.bbox and hasattr(page, "get_textpage"):
                        try:
                            textpage = page.get_textpage()
                            try:
                                page_height = page.get_height() if hasattr(page, "get_height") else page_h
                                bbox = prov.bbox.to_bottom_left_origin(page_height) if hasattr(prov.bbox, "to_bottom_left_origin") else prov.bbox
                                if hasattr(textpage, "get_text_bounded"):
                                    l = getattr(bbox, "l", getattr(bbox, "left", 0.0))
                                    b = getattr(bbox, "b", getattr(bbox, "bottom", 0.0))
                                    r = getattr(bbox, "r", getattr(bbox, "right", 0.0))
                                    t = getattr(bbox, "t", getattr(bbox, "top", 0.0))
                                    native_raw = textpage.get_text_bounded(
                                        left=l - 0.5, bottom=b - 0.5,
                                        right=r + 0.5, top=t + 0.5,
                                    ).strip()
                                    if native_raw:
                                        native_text = native_raw
                            finally:
                                textpage.close()
                        except Exception:
                            pass
                finally:
                    page.close()
            except Exception:
                pass

        if prov and prov.bbox:
            cbbox = CanonicalBBox.from_docling(prov.bbox, page_w, page_h)
        else:
            cbbox = CanonicalBBox(0.0, 0.0, 0.0, 0.0, 0.0, 0.0, coord_origin="TOPLEFT", page_width=page_w, page_height=page_h)

        region_id = f"p{page_no}_f{idx}"
        kind = "inline" if getattr(item, "is_inline", False) else "display"
        raw_text = (getattr(item, "text", "") or "").strip()

        if raw_text:
            is_valid, reason = check_latex_syntax(raw_text)
            if is_valid:
                has_native_divergence = (
                    native_text is not None
                    and has_substantive_conflict(raw_text, native_text)
                )
                if not has_native_divergence:
                    regions.append(FormulaRegionRecord(
                        region_id=region_id,
                        source_sha256=source_hash,
                        page=page_no,
                        bbox=asdict(cbbox),
                        kind=kind,
                        status="recognized",
                        method="visual_ocr",
                        reason_code=None,
                        latex=raw_text,
                        candidate=None,
                        attempts=1,
                    ))
                    continue
                primary_reason = "suspect_native_divergence"
            else:
                primary_reason = reason
        else:
            primary_reason = "empty_output"

        # Attempt visual retry on a higher-DPI crop with 4 pt margin if PDF & model available
        retry_img = None
        retry_failed = False
        retry_attempted = False
        if vlm_stage is not None and pdf and prov and prov.bbox and 1 <= page_no <= len(pdf):
            retry_attempted = True
            try:
                page = pdf[page_no - 1]
                try:
                    # PDFium crop is removed margins (left, bottom, right, top).
                    # Docling provenance may use either coordinate origin.
                    crop = (max(0.0, cbbox.left - 4.0),
                            max(0.0, page_h - cbbox.bottom - 4.0),
                            max(0.0, page_w - cbbox.right - 4.0),
                            max(0.0, cbbox.top - 4.0))
                    bitmap = page.render(scale=3.0, crop=crop)
                    try:
                        retry_img = bitmap.to_pil().copy()
                    finally:
                        bitmap.close()
                finally:
                    page.close()
            except Exception as err:
                retry_failed = True
                warnings.append(f"Не удалось подготовить кроп региона {region_id}: {err}")

        if retry_img is not None and vlm_stage is not None:
            try:
                from docling.models.stages.code_formula.code_formula_vlm_model import VlmEngineInput
                input_item = VlmEngineInput(
                    image=retry_img,
                    prompt="<formula>",
                    temperature=0.0,
                    max_new_tokens=2048,
                    extra_generation_config={"skip_special_tokens": False},
                )
                engine = getattr(vlm_stage, "engine", None)
                if engine is not None:
                    # Unwrap AuditedEngine to avoid polluting global warnings on deterministic empty output / token limit
                    real_engine = getattr(engine, "engine", engine)
                    if hasattr(real_engine, "initialize"):
                        real_engine.initialize()
                    outputs = real_engine.predict_batch([input_item])
                    post_proc = getattr(vlm_stage, "_post_process", lambda texts: texts)
                    raw_pred = outputs[0].text if outputs else ""
                    retry_raw = post_proc([raw_pred])[0].strip() if raw_pred else ""
                    num_tokens = (getattr(outputs[0], "metadata", None) or {}).get("num_tokens", 0) if outputs else 0

                    if not retry_raw:
                        # Deterministic empty model output on this crop is a stable unresolved state, not a transient crash
                        item.text = ""
                        item.orig = "formula"
                        regions.append(FormulaRegionRecord(
                            region_id=region_id,
                            source_sha256=source_hash,
                            page=page_no,
                            bbox=asdict(cbbox),
                            kind=kind,
                            status="unresolved",
                            method="visual_retry",
                            reason_code="empty_output",
                            latex=None,
                            candidate=None,
                            attempts=2,
                        ))
                        continue

                    if num_tokens >= input_item.max_new_tokens:
                        # Deterministic token limit on this crop is a stable unresolved state
                        item.text = ""
                        item.orig = "formula"
                        regions.append(FormulaRegionRecord(
                            region_id=region_id,
                            source_sha256=source_hash,
                            page=page_no,
                            bbox=asdict(cbbox),
                            kind=kind,
                            status="unresolved",
                            method="visual_retry",
                            reason_code="token_limit",
                            latex=None,
                            candidate=retry_raw,
                            attempts=2,
                        ))
                        continue

                    is_valid, retry_reason = check_latex_syntax(retry_raw)
                    if is_valid:
                        if raw_text and check_latex_syntax(raw_text)[0]:
                            if normalize_formatting_only(raw_text) == normalize_formatting_only(retry_raw):
                                item.text = retry_raw
                                regions.append(FormulaRegionRecord(
                                    region_id=region_id,
                                    source_sha256=source_hash,
                                    page=page_no,
                                    bbox=asdict(cbbox),
                                    kind=kind,
                                    status="recognized",
                                    method="visual_retry",
                                    reason_code=None,
                                    latex=retry_raw,
                                    candidate=None,
                                    attempts=2,
                                ))
                                continue
                            else:
                                item.text = ""
                                item.orig = "formula"
                                regions.append(FormulaRegionRecord(
                                    region_id=region_id,
                                    source_sha256=source_hash,
                                    page=page_no,
                                    bbox=asdict(cbbox),
                                    kind=kind,
                                    status="suspect",
                                    method="visual_retry",
                                    reason_code="suspect_divergence",
                                    latex=None,
                                    candidate=retry_raw,
                                    attempts=2,
                                ))
                                continue
                        else:
                            item.text = retry_raw
                            regions.append(FormulaRegionRecord(
                                region_id=region_id,
                                source_sha256=source_hash,
                                page=page_no,
                                bbox=asdict(cbbox),
                                kind=kind,
                                status="recognized",
                                method="visual_retry",
                                reason_code=None,
                                latex=retry_raw,
                                candidate=None,
                                attempts=2,
                            ))
                            continue
                    else:
                        item.text = ""
                        item.orig = "formula"
                        regions.append(FormulaRegionRecord(
                            region_id=region_id,
                            source_sha256=source_hash,
                            page=page_no,
                            bbox=asdict(cbbox),
                            kind=kind,
                            status="unresolved",
                            method="visual_retry",
                            reason_code=retry_reason,
                            latex=None,
                            candidate=retry_raw if retry_raw else None,
                            attempts=2,
                        ))
                        continue

            except Exception as err:
                retry_failed = True
                warnings.append(f"Ошибка повторного визуального OCR {region_id}: {err}")
            finally:
                retry_img.close()

        # Fallback when retry was not performed or failed
        item.text = ""
        item.orig = "formula"
        regions.append(FormulaRegionRecord(
            region_id=region_id,
            source_sha256=source_hash,
            page=page_no,
            bbox=asdict(cbbox),
            kind=kind,
            status="failed_transient" if retry_failed else "unresolved",
            method="visual_ocr",
            reason_code="visual_retry_failed" if retry_failed else primary_reason,
            latex=None,
            candidate=raw_text if raw_text else None,
            attempts=2 if retry_attempted else 1,
        ))

    if pdf:
        try:
            pdf.close()
        except Exception:
            pass

    return regions, warnings
