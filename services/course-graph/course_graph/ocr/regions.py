"""Registry and canonical coordinate representations for formula and code regions."""
from dataclasses import asdict, dataclass
from typing import Any, Dict, List, Optional


@dataclass
class CanonicalBBox:
    left: float
    top: float
    right: float
    bottom: float
    width: float
    height: float
    coord_origin: str = "TOPLEFT"
    page_width: Optional[float] = None
    page_height: Optional[float] = None

    @classmethod
    def from_docling(cls, bbox: Any, page_width: float, page_height: float) -> "CanonicalBBox":
        """Convert a Docling or PDFium BoundingBox to canonical top-left PDF points."""
        coord_origin = getattr(bbox, "coord_origin", None)
        origin_str = str(coord_origin) if coord_origin is not None else ""

        l = float(getattr(bbox, "l", getattr(bbox, "left", 0.0)))
        t = float(getattr(bbox, "t", getattr(bbox, "top", 0.0)))
        r = float(getattr(bbox, "r", getattr(bbox, "right", 0.0)))
        b = float(getattr(bbox, "b", getattr(bbox, "bottom", 0.0)))

        left = min(l, r)
        right = max(l, r)

        if "BOTTOMLEFT" in origin_str:
            # Docling bottom-left origin: t and b are distances from bottom
            # In bottom-left: b is the bottom (lower y), t is top (higher y)
            # When converted to top-left:
            top = page_height - max(t, b)
            bottom = page_height - min(t, b)
        else:
            top = min(t, b)
            bottom = max(t, b)

        width = max(0.0, right - left)
        height = max(0.0, bottom - top)

        return cls(
            left=round(left, 2),
            top=round(top, 2),
            right=round(right, 2),
            bottom=round(bottom, 2),
            width=round(width, 2),
            height=round(height, 2),
            coord_origin="TOPLEFT",
            page_width=round(page_width, 2) if page_width else None,
            page_height=round(page_height, 2) if page_height else None,
        )


@dataclass
class FormulaRegionRecord:
    region_id: str
    source_sha256: str
    page: int
    bbox: Dict[str, Any]
    kind: str  # "display" | "inline"
    status: str  # "recognized" | "unresolved" | "suspect" | "failed_transient"
    method: str  # "native_pdf" | "visual_ocr" | "visual_retry"
    reason_code: Optional[str] = None
    verification: str = "not_expert_validated"
    latex: Optional[str] = None
    candidate: Optional[str] = None
    attempts: int = 1

    def to_dict(self) -> Dict[str, Any]:
        return asdict(self)


def summarize_formula_regions(
    regions: List[FormulaRegionRecord],
    marker_count: int,
    warnings: List[str],
    picture_ocr: Optional[Dict[str, Any]] = None,
) -> Dict[str, Any]:
    """Calculate formula_summary, work_complete, and content_complete from the region registry."""
    detected = len(regions)
    recognized = sum(1 for r in regions if r.status == "recognized")
    unresolved = sum(1 for r in regions if r.status == "unresolved")
    suspect = sum(1 for r in regions if r.status == "suspect")
    failed_transient = sum(1 for r in regions if r.status == "failed_transient")

    formula_summary = {
        "detected": detected,
        "recognized": recognized,
        "unresolved": unresolved,
        "suspect": suspect,
        "failed_transient": failed_transient,
        "marker_count": marker_count,
    }

    has_failed_pictures = bool(picture_ocr and picture_ocr.get("failed_regions", 0) > 0)

    # work_complete: All planned pipeline operations finished (no transient crashes, no picture failures)
    work_complete = failed_transient == 0 and not has_failed_pictures

    # content_complete: Zero unresolved/suspect regions, no picture failures, no partial warnings
    has_partial_warnings = any("частично" in w.lower() for w in warnings)
    content_complete = (
        unresolved == 0
        and suspect == 0
        and failed_transient == 0
        and marker_count == 0
        and not has_failed_pictures
        and not has_partial_warnings
    )

    return {
        "formula_summary": formula_summary,
        "work_complete": work_complete,
        "content_complete": content_complete,
    }
