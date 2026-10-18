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

    # 1. Check curly braces {} balance and nesting
    cleaned_braces = re.sub(r"\\[\{\}]", "", text)
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

    # 2. Check \left and \right balance
    if text.count(r"\left") != text.count(r"\right"):
        return False, "unbalanced_left_right"

    # 3. Check parentheses and brackets () and []
    # Mathematical intervals [a, b), (a, b], [a, b], (a, b) are all valid.
    # Therefore, '(' and '[' can be closed by either ')' or ']'.
    # Delimiters must be properly paired without underflow or trailing open delimiters.
    # Escaped delimiters \[ and \] (LaTeX display math) and \( and \) (inline math) are ignored.
    cleaned_delims = re.sub(r"\\[\[\]\(\)]", "", text)
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
    s = s.replace(r"\left(", "(").replace(r"\right)", ")")
    s = s.replace(r"\left[", "[").replace(r"\right]", "]")
    s = s.replace(r"\left)", ")").replace(r"\right[", "[")
    s = s.replace(r"\left\{", "{").replace(r"\right\}", "}")
    s = s.replace(r"\left|", "|").replace(r"\right|", "|")
    s = s.replace(r"\left.", "").replace(r"\right.", "")
    # Remove whitespace inside parentheses, brackets and braces
    s = re.sub(r"([(\[{|])\s+", r"\1", s)
    s = re.sub(r"\s+([)\]}|])", r"\1", s)
    # Remove whitespace around operators like =, +, -
    s = re.sub(r"\s*([=+\-*/])\s*", r"\1", s)
    return s.strip()

