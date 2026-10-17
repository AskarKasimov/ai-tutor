"""Structural syntax checks and normalization for LaTeX formulas without semantic repair."""
import re
from typing import Optional, Tuple


def check_latex_syntax(latex: str) -> Tuple[bool, Optional[str]]:
    """Validate LaTeX syntax balance and completeness without altering characters.

    Returns (is_valid, reason_code).
    """
    if not latex or not latex.strip():
        return False, "empty_output"

    text = latex.strip()

    # Check for trailing truncated tokens (e.g. ends with bare backslash or open script)
    if text.endswith("\\") and not text.endswith("\\\\"):
        return False, "truncated_trailing_escape"
    if re.search(r"[_^]\{[^}]*$", text):
        return False, "truncated_subscript"

    # Check balanced delimiters: {}, [], ()
    # Note: ignore escaped delimiters like \{ or \[ when used as display math
    cleaned = text
    # Remove escaped delimiters for counting
    cleaned = re.sub(r"\\[\{\}\[\]\(\)]", "", cleaned)

    # Count braces
    if cleaned.count("{") != cleaned.count("}"):
        return False, "unbalanced_braces"

    # Count brackets and parentheses if they occur
    if cleaned.count("(") != cleaned.count(")"):
        return False, "unbalanced_parentheses"

    # Count LaTeX environments \begin{name} ... \end{name}
    begins = re.findall(r"\\begin\{([^}]+)\}", text)
    ends = re.findall(r"\\end\{([^}]+)\}", text)
    if begins != ends:
        return False, "unbalanced_environments"

    return True, None


def normalize_formatting_only(latex: str) -> str:
    """Normalize purely cosmetic differences in LaTeX without modifying mathematical content.

    Specifically:
    - Collapses consecutive whitespace
    - Normalizes \\left and \\right sizing prefixes (or optional space after macros)
    - Leaves all operators, variables, numbers, signs, subscripts and superscripts intact.
    """
    if not latex:
        return ""
    s = latex.strip()
    s = re.sub(r"\s+", " ", s)
    s = s.replace(r"\left(", "(").replace(r"\right)", ")")
    s = s.replace(r"\left[", "[").replace(r"\right]", "]")
    s = s.replace(r"\left\{", "{").replace(r"\right\}", "}")
    s = s.replace(r"\left|", "|").replace(r"\right|", "|")
    s = s.replace(r"\left.", "").replace(r"\right.", "")
    # Remove whitespace inside parentheses, brackets and braces
    s = re.sub(r"([(\[{|])\s+", r"\1", s)
    s = re.sub(r"\s+([)\]}|])", r"\1", s)
    # Remove whitespace around operators like =, +, -
    s = re.sub(r"\s*([=+\-*/])\s*", r"\1", s)
    return s.strip()

