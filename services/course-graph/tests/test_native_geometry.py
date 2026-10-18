"""Native PDF text is read from the rectangle Docling found, on rotated and cropped pages."""
import io
from pathlib import Path

import pytest
from pypdf import PdfWriter
from pypdf.generic import DecodedStreamObject, DictionaryObject, NameObject, RectangleObject


def make_pdf(rotate, crop):
    writer = PdfWriter()
    page = writer.add_blank_page(width=400, height=300)
    font = DictionaryObject({NameObject("/Type"): NameObject("/Font"), NameObject("/Subtype"): NameObject("/Type1"),
                             NameObject("/BaseFont"): NameObject("/Helvetica")})
    page[NameObject("/Resources")] = DictionaryObject({NameObject("/Font"): DictionaryObject({NameObject("/F1"): writer._add_object(font)})})
    stream = DecodedStreamObject()
    stream.set_data(b"BT /F1 12 Tf 30 30 Td (xone) Tj ET BT /F1 12 Tf 300 260 Td (ytwo) Tj ET")
    page[NameObject("/Contents")] = writer._add_object(stream)
    if crop:
        page.cropbox = RectangleObject(crop)
    if rotate:
        page.rotate(rotate)
    out = io.BytesIO()
    writer.write(out)
    return out.getvalue()


def docling_cells(content, tmp_path):
    # The same parser the pipeline uses gives the displayed-page boxes.
    from docling.backend.docling_parse_backend import ThreadedDoclingParseDocumentBackend
    from docling.datamodel.base_models import InputFormat
    from docling.datamodel.document import InputDocument
    path = Path(tmp_path, "page.pdf")
    path.write_bytes(content)
    document = InputDocument(path_or_stream=path, format=InputFormat.PDF, backend=ThreadedDoclingParseDocumentBackend)
    try:
        page = next(iter(document._backend.iter_pages()))
        height = page.get_size().height
        return [(cell.text, cell.rect.to_bounding_box().to_top_left_origin(height)) for cell in page.get_text_cells()]
    finally:
        document._backend.unload()


@pytest.mark.parametrize("rotate", [0, 90, 180, 270])
@pytest.mark.parametrize("crop", [None, (10, 10, 390, 290)])
def test_native_text_follows_rotation_and_crop_box(tmp_path, rotate, crop):
    import pypdfium2 as pdfium
    from types import SimpleNamespace as S
    from course_graph.ocr.enrichment import NativePdfTextReader, read_native_text

    content = make_pdf(rotate, crop)
    cells = docling_cells(content, tmp_path)
    assert sorted(text for text, _ in cells) == ["xone", "ytwo"]
    pdf = pdfium.PdfDocument(content)
    page = pdf[0]
    textpage = page.get_textpage()
    reader = NativePdfTextReader(content)
    try:
        for text, bbox in cells:
            assert read_native_text(textpage, page, bbox).strip() == text
            assert reader(S(prov=[S(page_no=1, bbox=bbox)])).strip() == text
    finally:
        reader.close()
        textpage.close()
        page.close()
        pdf.close()
