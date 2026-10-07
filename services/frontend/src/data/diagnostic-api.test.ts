import { afterEach, expect, it, vi } from 'vitest'
import { createSubmission, DiagnosticApiError, readDiagnostic, submitDiagnostic } from './diagnostic-api'

const accepted = { session_id: 's1', status: 'completed', completed_tasks: 1, skipped_tasks: 2, total_tasks: 3, score: 2, grader_score: 2, grader_max_score: 2, verdict: 'correct', criterion_results: [{ key: 'choice', satisfied: true, explanation: 'Верно.' }], feedback: ['Верно.', 'Причина.', 'Совет.'] }
afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals() })

it('allows an answer to finish after sequential transcription and grading exceed two minutes', async () => {
  vi.useFakeTimers()
  vi.spyOn(AbortSignal, 'timeout').mockImplementation((ms) => {
    const request = new AbortController()
    setTimeout(() => request.abort(new DOMException('Timeout', 'TimeoutError')), ms)
    return request.signal
  })
  vi.stubGlobal('fetch', vi.fn((_url, init: RequestInit) => new Promise((resolve, reject) => {
    const timer = setTimeout(() => resolve(Response.json(accepted)), 150_000)
    init.signal?.addEventListener('abort', () => { clearTimeout(timer); reject(init.signal?.reason) })
  })))
  const result = submitDiagnostic(createSubmission('s1', 't1', new Blob(['audio'], { type: 'audio/webm' }), 'main'), new AbortController().signal).then((value) => ({ value }), (error) => ({ error }))
  await vi.advanceTimersByTimeAsync(150_000)
  expect(await result).toMatchObject({ value: { session_id: 's1', score: 2, status: 'completed' } })
})

it.each([
  { ...accepted, completed_tasks: 5 },
  { ...accepted, feedback: ['one'] },
  { ...accepted, session_id: 'other-user-session' },
  { ...accepted, score: 9 },
  { ...accepted, grader_max_score: 1 },
])('rejects malformed accepted responses without recording a score', async (response) => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json(response)))
  await expect(submitDiagnostic(createSubmission('s1', 't1', new Blob(['audio']), 'main'), new AbortController().signal)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
})

it('rejects an active session missing the current task', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json({ session_id: 's1', status: 'active', completed_tasks: 0, skipped_tasks: 0, total_tasks: 3 })))
  await expect(readDiagnostic('s1', new AbortController().signal)).rejects.toBeInstanceOf(DiagnosticApiError)
})
