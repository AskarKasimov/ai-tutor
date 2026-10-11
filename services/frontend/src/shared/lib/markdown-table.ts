export interface TableData {
  headers: string[]
  alignments: ('left' | 'center' | 'right')[]
  rows: string[][]
}

export type QuestionBlock =
  | { type: 'text'; text: string }
  | { type: 'table'; data: TableData }

function parseRowCells(rowStr: string): string[] {
  const parts = rowStr.split('|').map((p) => p.trim())
  if (parts.length > 0 && parts[0] === '') parts.shift()
  if (parts.length > 0 && parts[parts.length - 1] === '') parts.pop()
  return parts
}

function isSeparatorCell(cell: string): boolean {
  return /^:?-{2,}:?$/.test(cell.trim())
}

function isSeparatorLine(lineStr: string): boolean {
  const trimmed = lineStr.trim()
  if (!trimmed.includes('|')) return false
  const cells = parseRowCells(trimmed)
  if (cells.length === 0) return false
  return cells.every(isSeparatorCell)
}

/**
 * Parses question or task text into sequential text and markdown table blocks.
 */
export function parseQuestionContent(raw: string): QuestionBlock[] {
  if (!raw) return []

  const lines = raw.split(/\r?\n/)
  const blocks: QuestionBlock[] = []
  let currentTextLines: string[] = []
  let i = 0
  const n = lines.length

  while (i < n) {
    const line = lines[i]
    const nextLine = i + 1 < n ? lines[i + 1] : ''

    // Detect markdown table start: line with '|' followed by a table separator line
    if (line.includes('|') && isSeparatorLine(nextLine)) {
      if (currentTextLines.length > 0) {
        const text = currentTextLines.join('\n').trim()
        if (text) {
          blocks.push({ type: 'text', text })
        }
        currentTextLines = []
      }

      const headers = parseRowCells(line)
      const sepCells = parseRowCells(nextLine)
      const alignments = sepCells.map((cell) => {
        const c = cell.trim()
        if (c.startsWith(':') && c.endsWith(':')) return 'center' as const
        if (c.endsWith(':')) return 'right' as const
        return 'left' as const
      })

      const rows: string[][] = []
      i += 2 // Skip header and separator

      while (i < n) {
        const rowLine = lines[i]
        const trimmed = rowLine.trim()
        if (trimmed && trimmed.includes('|')) {
          rows.push(parseRowCells(rowLine))
          i++
        } else {
          break
        }
      }

      blocks.push({
        type: 'table',
        data: { headers, alignments, rows },
      })
    } else {
      currentTextLines.push(line)
      i++
    }
  }

  if (currentTextLines.length > 0) {
    const text = currentTextLines.join('\n').trim()
    if (text) {
      blocks.push({ type: 'text', text })
    }
  }

  return blocks
}
