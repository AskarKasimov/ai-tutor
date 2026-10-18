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
    r"\epsilon": "ε", r"\varepsilon": "ε", r"\zeta": "ζ", r"\eta": "η",
    r"\theta": "θ", r"\vartheta": "θ", r"\iota": "ι", r"\kappa": "κ",
    r"\lambda": "λ", r"\mu": "μ", r"\nu": "ν", r"\xi": "ξ",
    r"\pi": "π", r"\varpi": "π", r"\rho": "ρ", r"\varrho": "ρ",
    r"\sigma": "σ", r"\varsigma": "σ", r"\tau": "τ", r"\upsilon": "υ",
    r"\phi": "φ", r"\varphi": "φ", r"\chi": "χ", r"\psi": "ψ", r"\omega": "ω",
    r"\Gamma": "Γ", r"\Delta": "Δ", r"\Theta": "Θ", r"\Lambda": "Λ",
    r"\Xi": "Ξ", r"\Pi": "Π", r"\Sigma": "Σ", r"\Upsilon": "Υ",
    r"\Phi": "Φ", r"\Psi": "Ψ", r"\Omega": "Ω",
    r"\sum": "∑", r"\prod": "∏", r"\int": "∫",
    # Relations and signs that a PDF text layer keeps as single glyphs.
    r"\leq": "≤", r"\le": "≤", r"\geq": "≥", r"\ge": "≥",
    r"\neq": "≠", r"\ne": "≠", r"\pm": "±", r"\mp": "∓", r"\ast": "*",
}
# Glyph variants that PDF text layers use for the same symbol.
NATIVE_GLYPH_MAP = {
    "−": "-", "–": "-", "∗": "*", "ϵ": "ε", "ϑ": "θ", "ϕ": "φ", "ϖ": "π", "ϱ": "ρ", "ς": "σ",
}
UNICODE_SUBS_SUPER_MAP = {
    "⁰": "0", "¹": "1", "²": "2", "³": "3", "⁴": "4",
    "⁵": "5", "⁶": "6", "⁷": "7", "⁸": "8", "⁹": "9",
    "₀": "0", "₁": "1", "₂": "2", "₃": "3", "₄": "4",
    "₅": "5", "₆": "6", "₇": "7", "₈": "8", "₉": "9",
    "ⁱ": "i", "ⁿ": "n",
}
OPERATORS = "+-=<>*/≤≥≠±∓"
TOKEN_RE = re.compile(r"[a-zA-Zа-яА-Яα-ωΑ-Ω0-9]|[+\-=<>*/≤≥≠±∓]")
# Longest commands first, and never inside a longer name: \le must not match \left.
_COMMANDS = sorted(GREEK_AND_MATH_MAP, key=len, reverse=True)
_COMMAND_RE = re.compile("(?:" + "|".join(re.escape(c) for c in _COMMANDS) + r")(?![a-zA-Z])")
# Constructs whose parts a PDF text layer stores in no reliable order.
_STACKED_RE = re.compile(r"\\(?:sum|prod|int|iint|oint|lim|frac|dfrac|tfrac|binom|sqrt|overset|underset|stackrel|substack)"
                         r"(?![a-zA-Z])|\\begin\{")
# Spellings of one command that a retry may choose differently.
_COMMAND_SYNONYMS = [(r"\\le(?![a-zA-Z])", r"\\leq"), (r"\\ge(?![a-zA-Z])", r"\\geq"),
                     (r"\\ne(?![a-zA-Z])", r"\\neq"), (r"\\to(?![a-zA-Z])", r"\\rightarrow")]


def normalize_formatting_only(latex: str) -> str:
    """Normalize purely cosmetic differences in LaTeX without modifying mathematical content.

    Specifically:
    - Collapses consecutive whitespace
    - Normalizes fraction commands (\\dfrac, \\tfrac -> \\frac)
    - Normalizes spellings of one command (\\le -> \\leq, \\ne -> \\neq, \\to -> \\rightarrow)
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
    for pattern, replacement in _COMMAND_SYNONYMS:
        s = re.sub(pattern, replacement, s)
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


def _symbols(text: str, latex: bool) -> str:
    """Map both sides to one alphabet: LaTeX commands and PDF glyph variants
    become the same Unicode symbols, sub/superscript digits become digits."""
    if latex:
        text = _COMMAND_RE.sub(lambda m: GREEK_AND_MATH_MAP[m.group(0)], text)
        text = re.sub(r"\\begin\{[^}]+\}|\\end\{[^}]+\}", "", text)
        text = re.sub(r"\\[a-zA-Z]+", "", text)
    for glyph, symbol in NATIVE_GLYPH_MAP.items():
        text = text.replace(glyph, symbol)
    for script, digit in UNICODE_SUBS_SUPER_MAP.items():
        text = text.replace(script, digit)
    return text


def has_substantive_conflict(raw_latex: str, native_text: str) -> bool:
    """Check if native PDF text layer explicitly contradicts the OCR LaTeX candidate.

    Digital PDF text layers contain flattened glyphs (e.g. 'x1 − 2', 'α2', 'x ≤ y' or
    'n ∑ i=1 xi'), which differ in syntax and often in symbol order from LaTeX
    ('x_1 - 2', '\\alpha^2', 'x \\leq y', '\\sum_{i=1}^n x_i'). Both sides are mapped to
    one alphabet: a different operator, digit or letter, a dropped or an extra term is a
    contradiction. The order of letters and digits is compared only for formulas without
    stacked constructs (sums, integrals, limits, fractions, roots, environments), whose
    parts a text layer does not keep in reading order; so inverted indices ('x_2^i'
    against 'xi2') are caught while '\\sum_{i=1}^n x_i' against 'n ∑ i=1 xi' is not a
    conflict. A text layer that itself carries LaTeX scripts ('x_i^2') is compared with
    its structure.
    """
    if not raw_latex or not native_text:
        return False
    if "_" in native_text or "^" in native_text:
        return normalize_formatting_only(_symbols(raw_latex, True)) != normalize_formatting_only(_symbols(native_text, True))

    raw_tokens = TOKEN_RE.findall(_symbols(raw_latex, True))
    native_tokens = TOKEN_RE.findall(_symbols(native_text, False))
    if not raw_tokens or not native_tokens:
        return False

    from collections import Counter

    # Operators must match in counts if native layer has operators
    raw_ops = [t for t in raw_tokens if t in OPERATORS]
    nat_ops = [t for t in native_tokens if t in OPERATORS]
    if nat_ops and Counter(raw_ops) != Counter(nat_ops):
        return True

    raw_alpha = [t.lower() for t in raw_tokens if t not in OPERATORS]
    nat_alpha = [t.lower() for t in native_tokens if t not in OPERATORS]
    if not raw_alpha or not nat_alpha:
        return False
    if Counter(raw_alpha) != Counter(nat_alpha):
        return True
    # Same symbols: the order counts only where the text layer keeps reading order.
    return not _STACKED_RE.search(raw_latex) and raw_alpha != nat_alpha
