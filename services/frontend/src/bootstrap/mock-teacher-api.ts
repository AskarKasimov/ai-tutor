import { findMockSubject, mockSubjectCatalog } from './mock-subject-catalog'

// Mock uploads simulate an imported demonstration map. Real uploads are parsed
// and validated by the backend; teacher access uses the same UI and API ports.
const maps = new Map<string, ReturnType<typeof demonstrationMap>>()
let revision = 0
function demonstrationMap(subjectId: string) {
  return {
    revision: ++revision,
    imported_at: Math.floor(Date.now() / 1000),
    competencies: [
      {
        id: `${subjectId}:competency`,
        name: 'Демонстрационная компетенция',
        constituents: [
          { outcomes: [{ tasks: [{ id: `${subjectId}:task` }] }] },
        ],
      },
    ],
  }
}
export function mockTeacherResponse(
  role: 'student' | 'admin',
  path: string,
  method: string,
  body: Record<string, unknown>,
  options: RequestInit,
): Response | undefined {
  const create = method === 'POST' && path.endsWith('/admin/subjects')
  const upload = path.match(
    /\/admin\/subjects\/([^/]+)\/competency-map\/import$/,
  )
  if (create || (upload && method === 'POST')) {
    if (role !== 'admin')
      return Response.json({ code: 'FORBIDDEN' }, { status: 403 })
    if (create) {
      const name = typeof body.name === 'string' ? body.name.trim() : ''
      if (!name || [...name].length > 200)
        return Response.json({ code: 'VALIDATION_ERROR' }, { status: 422 })
      const subject = {
        id: `subject:${crypto.randomUUID()}`,
        name,
        ready: false,
      }
      mockSubjectCatalog.push(subject)
      return Response.json(subject, { status: 201 })
    }
    const subject = findMockSubject(decodeURIComponent(upload![1]))
    if (!subject)
      return Response.json({ code: 'SUBJECT_NOT_FOUND' }, { status: 404 })
    const file =
      options.body instanceof FormData ? options.body.get('file') : null
    if (
      !(file instanceof File) ||
      !file.size ||
      !/\.(csv|xlsx)$/i.test(file.name)
    )
      return Response.json({ code: 'VALIDATION_ERROR' }, { status: 422 })
    if (file.size > 25 * 1024 * 1024)
      return Response.json({ code: 'UPLOAD_TOO_LARGE' }, { status: 413 })
    const map = demonstrationMap(subject.id)
    maps.set(subject.id, map)
    subject.ready = true
    return Response.json({
      revision: map.revision,
      imported_at: map.imported_at,
      competency_count: 1,
      constituent_count: 1,
      outcome_count: 1,
      task_count: 1,
      unparsed_task_cell_count: 0,
      warnings: [],
    })
  }
  const read = path.match(/\/subjects\/([^/]+)\/competency-map$/)
  if (read && method === 'GET') {
    const map = maps.get(decodeURIComponent(read[1]))
    if (map) return Response.json(map)
  }
  return undefined
}
