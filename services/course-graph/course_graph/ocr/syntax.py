"""Structural syntax checks and normalization for LaTeX formulas without semantic repair."""
import re
from typing import Optional, Tuple


LEFT_RIGHT_RE = re.compile(
    r"\\(left|right)(?![a-zA-Z])(?:\s*(\\[a-zA-Z]+|\\[^a-zA-Z]|[^\\a-zA-Z\s]))?"
)


def check_latex_syntax(latex: str) -> Tuple[bool, Optional[str]]:
    """Validate LaTeX syntax balance and completeness without altering characters.

    Returns (is_valid, reason_code).
    """
    if not latex or not latex.strip():
        return False, "empty_output"

    text = latex.strip()

    # Check for trailing truncated tokens (e.g. ends with bare backslash or open script)
    trailing_backslashes = len(text) - len(text.rstrip("\\"))
    if trailing_backslashes % 2 == 1:
        return False, "truncated_trailing_escape"
    if re.search(r"[_^]\{[^}]*$", text):
        return False, "truncated_subscript"

    # 1. Check curly braces {} balance and nesting
    # Escaped literal braces \{ and \} are ignored; \\{ is a newline followed by a grouping brace.
    cleaned_braces = re.sub(r"(?<!\\)\\[\{\}]", "", text)
    brace_stack = []
    for ch in cleaned_braces:
        if ch == "{":
            brace_stack.append(ch)
        elif ch == "}":
            if not brace_stack:
                return False, "unbalanced_braces"
            brace_stack.pop()
    if brace_stack:
        return False, "unbalanced_braces"

    # 2. Check \left and \right delimiter sizing balance and nesting
    # Matches \left and \right commands with their delimiter arguments (including the invisible delimiter '.').
    # Negative lookahead (?![a-zA-Z]) prevents matching arrow commands like \rightarrow, \leftarrow, \leftrightarrow.
    left_stack = []
    for match in LEFT_RIGHT_RE.finditer(text):
        cmd = match.group(1)
        delim = match.group(2)
        if not delim or not delim.strip():
            return False, "unbalanced_left_right"
        delim = delim.strip()
        if cmd == "left":
            left_stack.append(delim)
        else:
            if not left_stack:
                return False, "unbalanced_left_right"
            left_stack.pop()
    if left_stack:
        return False, "unbalanced_left_right"

    # 3. Check standalone parentheses and brackets () and []
    # Mathematical intervals [a, b), (a, b], [a, b], (a, b) are all valid.
    # Therefore, '(' and '[' can be closed by either ')' or ']'.
    # Delimiters bound to \left and \right (including invisible delimiters \left. and \right.),
    # newlines with optional vertical spacing arguments (e.g. \\[2pt] in matrix/cases),
    # optional command arguments (e.g. \sqrt[3]{x}), and escaped delimiters \[ and \]
    # are stripped beforehand so they do not pollute the standalone delimiter stack.
    cleaned_delims = text
    # Strip newline command \\ with optional * and optional bracket dimension argument \\[2pt]
    cleaned_delims = re.sub(r"\\\\(?:\*|\s*\[[^\]]*\])?", " ", cleaned_delims)
    # Strip LaTeX commands with optional bracket arguments (e.g. \sqrt[3], \xrightarrow[sub])
    cleaned_delims = re.sub(r"\\(?!left|right)[a-zA-Z]+\s*\[[^\]]*\]", " ", cleaned_delims)
    # Strip \begin{name}[opt] optional environment arguments (e.g. \begin{array}[t]{c})
    cleaned_delims = re.sub(r"\\begin\{[^}]+\}\s*\[[^\]]*\]", " ", cleaned_delims)
    # Strip \left <delim> and \right <delim> constructs (delimiters are consumed by the pair)
    cleaned_delims = LEFT_RIGHT_RE.sub(" ", cleaned_delims)
    # Strip escaped math delimiters: \[ and \] (display math) and \( and \) (inline math)
    cleaned_delims = re.sub(r"(?<!\\)\\[\[\]\(\)]", " ", cleaned_delims)

    delim_stack = []
    for ch in cleaned_delims:
        if ch in "([":
            delim_stack.append(ch)
        elif ch in ")]":
            if not delim_stack:
                return False, "unbalanced_delimiters"
            delim_stack.pop()
    if delim_stack:
        return False, "unbalanced_delimiters"

    # 4. LaTeX environments \begin{name} ... \end{name}
    # Uses a LIFO stack to correctly support arbitrarily nested environments:
    # e.g. \begin{equation}\begin{aligned} ... \end{aligned}\end{equation}
    env_stack = []
    for match in re.finditer(r"\\(begin|end)\{([^}]+)\}", text):
        action, name = match.group(1), match.group(2).strip()
        if action == "begin":
            env_stack.append(name)
        else:
            if not env_stack or env_stack.pop() != name:
                return False, "unbalanced_environments"
    if env_stack:
        return False, "unbalanced_environments"

    return True, None


GREEK_AND_MATH_MAP = {
    r"\alpha": "α", r"\beta": "β", r"\gamma": "γ", r"\delta": "δ",
    r"\epsilon": "ε", r"\zeta": "ζ", r"\eta": "η", r"\theta": "θ",
    r"\iota": "ι", r"\kappa": "κ", r"\lambda": "λ", r"\mu": "μ",
    r"\nu": "ν", r"\xi": "ξ", r"\pi": "π", r"\rho": "ρ",
    r"\sigma": "σ", r"\tau": "τ", r"\upsilon": "υ", r"\phi": "φ",
    r"\chi": "χ", r"\psi": "ψ", r"\omega": "ω",
    r"\sum": "∑", r"\prod": "∏", r"\int": "∫",
}
UNICODE_SUBS_SUPER_MAP = {
    "⁰": "0", "¹": "1", "²": "2", "³": "3", "⁴": "4",
    "⁵": "5", "⁶": "6", "⁷": "7", "⁸": "8", "⁹": "9",
    "₀": "0", "₁": "1", "₂": "2", "₃": "3", "₄": "4",
    "₅": "5", "₆": "6", "₇": "7", "₈": "8", "₉": "9",
    "ⁱ": "i", "ⁿ": "n",
}


def normalize_formatting_only(latex: str) -> str:
    """Normalize purely cosmetic differences in LaTeX without modifying mathematical content.

    Specifically:
    - Collapses consecutive whitespace
    - Normalizes fraction commands (\\dfrac, \\tfrac -> \\frac)
    - Normalizes braces around single tokens in sub/superscripts (_{1} -> _1, ^{2} -> ^2)
    - Standardizes relative order of adjacent subscript and superscript on same base (x^2_1 -> x_1^2)
    - Normalizes \\left and \\right sizing prefixes (or optional space after macros)
    - Leaves all operators, variables, numbers, signs, subscripts and superscripts intact.
    """
    if not latex:
        return ""
    s = latex.strip()
    # Normalize fraction variants
    s = re.sub(r"\\[dt]frac\b", r"\\frac", s)
    # Remove single-token braces around subscripts and superscripts
    s = re.sub(r"([_~^])\{([a-zA-Z0-9])\}", r"\1\2", s)
    s = re.sub(r"\s*([_~^])\s*", r"\1", s)
    # Standardize order of adjacent subscript and superscript on same base: x^2_1 -> x_1^2
    s = re.sub(r"([a-zA-Z0-9])\^([a-zA-Z0-9]|(?:\{[^}]+\}))_([a-zA-Z0-9]|(?:\{[^}]+\}))", r"\1_\3^\2", s)
    s = re.sub(r"\s+", " ", s)
    s = re.sub(r"\\left\s*\(", "(", s)
    s = re.sub(r"\\right\s*\)", ")", s)
    s = re.sub(r"\\left\s*\[", "[", s)
    s = re.sub(r"\\right\s*\]", "]", s)
    s = re.sub(r"\\left\s*\)", ")", s)
    s = re.sub(r"\\right\s*\[", "[", s)
    s = re.sub(r"\\left\s*\\\{", "{", s)
    s = re.sub(r"\\right\s*\\\}", "}", s)
    s = re.sub(r"\\left\s*\|", "|", s)
    s = re.sub(r"\\right\s*\|", "|", s)
    s = re.sub(r"\\left\s*\.", "", s)
    s = re.sub(r"\\right\s*\.", "", s)
    # Remove whitespace inside parentheses, brackets and braces
    s = re.sub(r"([(\[{|])\s+", r"\1", s)
    s = re.sub(r"\s+([)\]}|])", r"\1", s)
    # Remove whitespace around operators like =, +, -
    s = re.sub(r"\s*([=+\-*/])\s*", r"\1", s)
    return s.strip()


def has_substantive_conflict(raw_latex: str, native_text: str) -> bool:
    """Check if native PDF text layer explicitly contradicts the OCR LaTeX candidate.

    Digital PDF text layers contain flattened glyphs (e.g. 'x1 + 2' or 'α2' or '∑i=1n xi'),
    which naturally differ in syntax from LaTeX ('x_1 + 2', '\\alpha^2', '\\sum_{i=1}^n x_i').
    This function detects TRUE substantive contradictions (e.g. inverted index order 'x_2^i' vs 'x_i^2'/'xi2',
    mismatched operators '+' vs '-', or missing/extra terms) rather than ordinary syntax differences.
    """
    if not raw_latex or not native_text:
        return False

    raw_clean = raw_latex
    for cmd, sym in GREEK_AND_MATH_MAP.items():
        raw_clean = re.sub(re.escape(cmd) + r"\b", sym, raw_clean)
    raw_clean = re.sub(r"\\begin\{[^}]+\}|\\end\{[^}]+\}", "", raw_clean)
    raw_clean = re.sub(r"\\[a-zA-Z]+", "", raw_clean)

    nat_clean = native_text
    for u, a in UNICODE_SUBS_SUPER_MAP.items():
        raw_clean = raw_clean.replace(u, a)
        nat_clean = nat_clean.replace(u, a)

    raw_tokens = re.findall(r"[a-zA-Zа-яА-Яα-ωΑ-Ω0-9]|[+\-=<>*/≤≥≠±]", raw_clean)
    native_tokens = re.findall(r"[a-zA-Zа-яА-Яα-ωΑ-Ω0-9]|[+\-=<>*/≤≥≠±]", nat_clean)
    if not raw_tokens or not native_tokens:
        return False

    from collections import Counter

    # 1. Operators must match in counts if native layer has operators
    raw_ops = [t for t in raw_tokens if t in "+-=<>*/≤≥≠±"]
    nat_ops = [t for t in native_tokens if t in "+-=<>*/≤≥≠±"]
    if nat_ops and Counter(raw_ops) != Counter(nat_ops):
        return True

    # 2. Alphanumeric sequence order
    raw_alpha = [t.lower() for t in raw_tokens if t not in "+-=<>*/≤≥≠±"]
    nat_alpha = [t.lower() for t in native_tokens if t not in "+-=<>*/≤≥≠±"]
    if raw_alpha and nat_alpha:
        # If lengths are equal and sequences differ -> clear contradiction (e.g. inverted indices x2i vs xi2)
        if len(raw_alpha) == len(nat_alpha) and raw_alpha != nat_alpha:
            return True
        # If multiset of alphanumerics differs -> clear contradiction (e.g. dropped term or different digit)
        if Counter(raw_alpha) != Counter(nat_alpha):
            return True

    return False
