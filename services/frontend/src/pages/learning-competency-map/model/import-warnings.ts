// Groups import warnings by spreadsheet row so a task split into several
// fragments reads as one problem: each reason lists its affected columns.

export type ImportWarningItem = {
  row: number
  columnIndex: number
  column: string
  code: string
}

export type WarningGroup = {
  row: number
  reasons: { code: string; columns: string[] }[]
}

export function groupWarnings(warnings: ImportWarningItem[]): WarningGroup[] {
  const rows = new Map<number, ImportWarningItem[]>()
  for (const warning of warnings) {
    const list = rows.get(warning.row) ?? []
    list.push(warning)
    rows.set(warning.row, list)
  }
  return [...rows.entries()]
    .sort(([left], [right]) => left - right)
    .map(([row, list]) => {
      const reasons: WarningGroup['reasons'] = []
      for (const warning of [...list].sort(
        (left, right) => left.columnIndex - right.columnIndex,
      )) {
        const reason = reasons.find((item) => item.code === warning.code)
        if (reason) reason.columns.push(warning.column)
        else reasons.push({ code: warning.code, columns: [warning.column] })
      }
      return { row, reasons }
    })
}
