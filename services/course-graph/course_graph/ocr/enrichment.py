"""Keep native program syntax and make Docling enrichment failures observable."""

import threading


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
        try:
            # Transformers >=4.50 replaces global GenerationConfig defaults with
            # model defaults. CodeFormulaV2 ships use_cache=False, overriding the
            # True requested by Docling unless the model config is corrected too.
            self.initialize()
            outputs = self.engine.predict_batch(inputs)
            if len(outputs) != len(inputs):
                raise ValueError("enrichment output count mismatch")
            for request, output in zip(inputs, outputs):
                if not output.text.strip():
                    raise ValueError("empty enrichment output")
                if (output.metadata or {}).get("num_tokens", 0) >= request.max_new_tokens:
                    raise ValueError("enrichment token limit reached")
            return outputs
        except Exception as error:
            self.warnings.append("Код/формулы обработаны частично: " + type(error).__name__ + ": " + str(error)[:200])
            # Docling retains original items when its inference engine raises.
            raise


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
        return native_text is not None and item_text is not None and native_text == item_text, native_text

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
        native_by_element = {}
        preserved = []
        for element in elements:
            item = element.item
            if item.label not in (DocItemLabel.CODE, DocItemLabel.FORMULA) or not item.text.strip():
                continue
            matches, native = self._matches_native_pdf(item)
            native_by_element[id(element)] = native
            if matches:
                preserved.append(element)
        preserved_ids = {id(element) for element in preserved}
        enrich = [element for element in elements if id(element) not in preserved_ids]
        output = list(self.stage(doc, enrich)) if enrich else []
        output_by_id = {id(item): item for item in output}
        for element in elements:
            item = element.item if id(element) in preserved_ids else output_by_id.get(id(element.item), element.item)
            native = native_by_element.get(id(element))
            if (id(element) in preserved_ids and element.item.label == DocItemLabel.FORMULA
                    and native is not None and ":-" in native):
                # Logic-program rules are code, even if layout classified them
                # as mathematics. A LaTeX colon cannot replace Prolog's ':-'.
                item.text = native
                item.label = DocItemLabel.TEXT
                self.warnings.append("Правило Prolog ошибочно размечено как формула; сохранён исходный синтаксис")
            yield item


class NativePdfTextReader:
    """Read native characters from the same PDF region Docling classified."""

    def __init__(self, pdf_content):
        import pypdfium2 as pdfium
        self._pdf = pdfium.PdfDocument(pdf_content)
        self._lock = threading.Lock()

    def __call__(self, item):
        provenance = getattr(item, "prov", None)
        if not provenance or len(provenance) != 1:
            return None
        proof = provenance[0]
        if not 1 <= proof.page_no <= len(self._pdf):
            return None
        with self._lock:
            page = self._pdf[proof.page_no - 1]
            try:
                bbox = proof.bbox.to_bottom_left_origin(page.get_height())
                textpage = page.get_textpage()
                try:
                    return textpage.get_text_bounded(
                        left=bbox.l - 0.5, bottom=bbox.b - 0.5,
                        right=bbox.r + 0.5, top=bbox.t + 0.5,
                    )
                finally:
                    textpage.close()
            finally:
                page.close()

    def close(self):
        with self._lock:
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
