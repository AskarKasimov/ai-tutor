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

