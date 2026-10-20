// Older reports may put a competency heading before the actual explanation.
// Hide only that standalone heading; keep emphasis inside the explanation.
export function feedbackExplanation(text: string) {
  return text
    .replace(/^\s*\*\*[КK]\d+\b[^\r\n]*?\*\*[.:]?[ \t]*(?:\r?\n|$)/u, '')
    .trim()
}
