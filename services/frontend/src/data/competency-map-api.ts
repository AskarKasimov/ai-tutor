import { z } from 'zod'
import { i18n } from '../i18n/i18n'
import type {
  CompetencyMapImport,
  CompetencyMapSummary,
} from '../shared/domain'
import { apiFetch } from './api-fetch'

const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(
  /\/$/,
  '',
)
const count = z.number().int().nonnegative()
const mapSchema = z.object({
  revision: count,
  imported_at: count.nullable(),
  competencies: z.array(
    z.object({
      constituents: z.array(
        z.object({
          outcomes: z.array(z.object({ tasks: z.array(z.unknown()) })),
        }),
      ),
    }),
  ),
})
const importSchema = z.object({
  revision: count,
  imported_at: count,
  competency_count: count,
  constituent_count: count,
  outcome_count: count,
  task_count: count,
  unparsed_task_cell_count: count,
  warnings: z.array(
    z.object({
      row: count,
      column_index: count,
      column: z.string(),
      code: z.string(),
    }),
  ),
})
const errorSchema = z.object({
  message: z.string(),
  details: z
    .array(z.object({ path: z.string().optional(), message: z.string() }))
    .optional(),
})

export class CompetencyMapApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
    readonly details: { path?: string; message: string }[] = [],
  ) {
    super(message)
  }
}

async function readResponse(response: Response): Promise<unknown> {
  const data: unknown = await response.json().catch(() => null)
  if (!response.ok) {
    const parsed = errorSchema.safeParse(data)
    const key =
      response.status === 401
        ? 'unauthorized'
        : response.status === 403
          ? 'forbidden'
          : response.status === 413
            ? 'tooLarge'
            : response.status === 415
              ? 'invalidType'
              : 'requestError'
    throw new CompetencyMapApiError(
      response.status,
      parsed.success ? parsed.data.message : i18n.t(`competencyMap.${key}`),
      parsed.success ? parsed.data.details : undefined,
    )
  }
  return data
}

export async function readCompetencyMap(
  signal: AbortSignal,
): Promise<CompetencyMapSummary> {
  const response = await apiFetch(`${apiBase}/competency-map`, {
    credentials: 'include',
    signal: AbortSignal.any([signal, AbortSignal.timeout(30_000)]),
  })
  const parsed = mapSchema.safeParse(await readResponse(response))
  if (!parsed.success) throw new Error(i18n.t('competencyMap.invalidResponse'))
  const map = parsed.data
  const constituents = map.competencies.flatMap((item) => item.constituents)
  const outcomes = constituents.flatMap((item) => item.outcomes)
  return {
    revision: map.revision,
    importedAt: map.imported_at,
    competencyCount: map.competencies.length,
    constituentCount: constituents.length,
    outcomeCount: outcomes.length,
    taskCount: outcomes.reduce((sum, item) => sum + item.tasks.length, 0),
  }
}

export async function importCompetencyMap(
  file: File,
  signal?: AbortSignal,
): Promise<CompetencyMapImport> {
  // Browsers may supply an empty or generic MIME type. The backend expects CSV/XLSX media types.
  const mediaType = file.name.toLowerCase().endsWith('.csv')
    ? 'text/csv'
    : 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
  const body = new FormData()
  body.append('file', new Blob([file], { type: mediaType }), file.name)
  const timeout = AbortSignal.timeout(120_000)
  const response = await apiFetch(`${apiBase}/admin/competency-map/import`, {
    method: 'POST',
    credentials: 'include',
    body,
    signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
  })
  const parsed = importSchema.safeParse(await readResponse(response))
  if (!parsed.success) throw new Error(i18n.t('competencyMap.unknownResult'))
  const result = parsed.data
  return {
    revision: result.revision,
    importedAt: result.imported_at,
    competencyCount: result.competency_count,
    constituentCount: result.constituent_count,
    outcomeCount: result.outcome_count,
    taskCount: result.task_count,
    unparsedTaskCellCount: result.unparsed_task_cell_count,
    warnings: result.warnings.map((item) => ({
      row: item.row,
      columnIndex: item.column_index,
      column: item.column,
      code: item.code,
    })),
  }
}
