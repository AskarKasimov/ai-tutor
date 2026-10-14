export function splitLessonParagraphs(text: string): string[] {
  // Decode paragraph separators in prose, preserving code and LaTeX commands.
  const normalized = text
    .split(/(```[\s\S]*?```|`[^`\n]*`)/g)
    .map((part) =>
      part.startsWith('`') ? part : part.replace(/(?:\\r\\n|\\n){2,}/g, '\n\n'),
    )
    .join('')
  return normalized
    .split(/\r?\n\s*\r?\n/)
    .map((part) => part.trim())
    .filter(Boolean)
}
