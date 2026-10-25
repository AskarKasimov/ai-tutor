export type CompetencyMapSummary = {
  revision: number
  importedAt: number | null
  competencyCount: number
  constituentCount: number
  outcomeCount: number
  taskCount: number
}

export type CompetencyMapImport = CompetencyMapSummary & {
  unparsedTaskCellCount: number
  warnings: { row: number; columnIndex: number; column: string; code: string }[]
}
