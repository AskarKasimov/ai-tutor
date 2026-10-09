import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import {
  createSubmission,
  createVariant,
  startDiagnostic,
  submitDiagnostic,
} from '@/entities/diagnostic-session'
import {
  createTrainingSubmission,
  fetchTrainingAudioFile,
  findTrainingForDiagnostic,
  readTrainingAudio,
  readTrainingHistory,
  readTrainingPreview,
  readTrainingSession,
  startTraining,
  submitTraining,
  TrainingApiError,
} from '@/entities/training'
import {
  listSubjects,
  readLearningState,
  SubjectApiError,
} from '@/entities/subject'
import { apiFetch, configureMockApiHandler } from '@/shared/api'
import { mockApiFetch } from '@/bootstrap/mock-api'
import { mockDiagnosticResponse } from '@/bootstrap/mock-diagnostic-api'

const signal = () => new AbortController().signal

beforeEach(async () => {
  vi.stubEnv('VITE_API_MODE', 'mock')
  configureMockApiHandler(mockApiFetch)
  await mockApiFetch('/api/v1/auth/logout', { method: 'POST' })
  await mockApiFetch('/api/v1/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: '', password: '' }),
  })
})
afterEach(async () => {
  await mockApiFetch('/api/v1/auth/logout', { method: 'POST' })
  vi.unstubAllGlobals()
})

async function completeDiagnostic(subjectId = 'subject:demo-a') {
  const response = await apiFetch('/api/v1/variants', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': crypto.randomUUID(),
    },
    body: JSON.stringify({ subject_id: subjectId }),
  })
  const variant = (await response.json()) as { id: string }
  const started = await startDiagnostic(
    variant.id,
    crypto.randomUUID(),
    signal(),
  )
  expect(started.status).toBe('active')
  const accepted = await submitDiagnostic(
    createSubmission(
      started.session_id,
      started.current!.variant_task_id,
      new Blob(['diagnostic']),
      'main',
    ),
    signal(),
  )
  expect(accepted.status).toBe('completed')
  return started.session_id
}

it('exposes the demo subject, learning state, and rejects unknown subjects', async () => {
  await expect(listSubjects(signal())).resolves.toEqual([
    { id: 'subject:demo-a', name: 'Демонстрационный предмет A', ready: true },
    { id: 'subject:demo-b', name: 'Демонстрационный предмет B', ready: true },
  ])
  await expect(
    readLearningState('subject:demo-a', signal()),
  ).resolves.toMatchObject({
    diagnostic_status: 'not_started',
    diagnostic_completed: false,
    training_available: false,
  })
  await expect(
    readLearningState('subject:unknown', signal()),
  ).rejects.toMatchObject({
    status: 404,
    code: 'SUBJECT_NOT_FOUND',
  } satisfies Partial<SubjectApiError>)
  const rejected = await apiFetch('/api/v1/variants', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': 'unknown-subject',
    },
    body: JSON.stringify({ subject_id: 'subject:unknown' }),
  })
  expect(rejected.status).toBe(404)
})

it('requires a known subject when creating a diagnostic variant', async () => {
  await expect(
    createVariant('subject:unknown', 'legacy-variant', signal()),
  ).rejects.toMatchObject({ status: 404 })
  const legacy = await createVariant(
    'subject:demo-a',
    'legacy-variant',
    signal(),
  )
  expect(legacy.id).toBeTruthy()
  const diagnosticId = await completeDiagnostic()
  await expect(
    readLearningState('subject:demo-a', signal()),
  ).resolves.toMatchObject({
    diagnostic_status: 'completed',
    diagnostic_completed: true,
    training_available: true,
    diagnostic_session_id: diagnosticId,
  })
  const nextVariant = await createVariant(
    'subject:demo-a',
    'later-variant',
    signal(),
  )
  const active = await startDiagnostic(nextVariant.id, 'later-start', signal())
  await expect(
    readLearningState('subject:demo-a', signal()),
  ).resolves.toMatchObject({
    diagnostic_status: 'active',
    diagnostic_completed: true,
    training_available: true,
    diagnostic_session_id: diagnosticId,
    active_session_id: active.session_id,
  })
})

it('gates training until diagnostic completion, previews both focused categories, starts once, and accepts replay-safe rounds with paged history', async () => {
  const response = await apiFetch('/api/v1/variants', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': 'unfinished-variant',
    },
    body: JSON.stringify({ subject_id: 'subject:demo-a' }),
  })
  const variant = (await response.json()) as { id: string }
  const unfinished = await startDiagnostic(
    variant.id,
    'unfinished-start',
    signal(),
  )
  await expect(
    readTrainingPreview(unfinished.session_id, signal()),
  ).rejects.toBeInstanceOf(TrainingApiError)
  await expect(
    startTraining(unfinished.session_id, 'too-early', undefined, signal()),
  ).rejects.toMatchObject({ status: 404 })

  const diagnosticId = await completeDiagnostic()
  const preview = await readTrainingPreview(diagnosticId, signal())
  expect(preview).toMatchObject({
    mode: 'focused',
    status: 'ready',
    subject_id: 'subject:demo-a',
  })
  expect(preview.confirmed_gaps).toHaveLength(1)
  expect(preview.partial_competencies).toHaveLength(1)
  expect(preview.confirmed_gaps[0].original_score).toBe(0)

  const otherDiagnosticId = await completeDiagnostic('subject:demo-b')
  await expect(
    readLearningState('subject:demo-a', signal()),
  ).resolves.toMatchObject({ diagnostic_session_id: diagnosticId })
  await expect(
    readLearningState('subject:demo-b', signal()),
  ).resolves.toMatchObject({
    diagnostic_session_id: otherDiagnosticId,
    diagnostic_completed: true,
  })
  await expect(
    readTrainingPreview(otherDiagnosticId, signal()),
  ).resolves.toMatchObject({
    subject_id: 'subject:demo-b',
    subject_name: 'Демонстрационный предмет B',
  })
  const otherTraining = await startTraining(
    otherDiagnosticId,
    'other-training-start',
    undefined,
    signal(),
  )
  await expect(
    readTrainingSession(otherTraining.session_id, signal()),
  ).resolves.toMatchObject({
    subject_id: 'subject:demo-b',
    subject_name: 'Демонстрационный предмет B',
  })

  const started = await startTraining(
    diagnosticId,
    'training-start',
    undefined,
    signal(),
  )
  const repeatedStart = await startTraining(
    diagnosticId,
    'different-start-key',
    undefined,
    signal(),
  )
  expect(repeatedStart.session_id).toBe(started.session_id)
  await expect(
    findTrainingForDiagnostic(diagnosticId, signal()),
  ).resolves.toMatchObject({ session_id: started.session_id })
  await expect(
    readTrainingSession(started.session_id, signal()),
  ).resolves.toMatchObject({
    answer_count: 0,
    subject_id: 'subject:demo-a',
    subject_name: 'Демонстрационный предмет A',
  })
  const audio = await readTrainingAudio(
    started.session_id,
    started.current.exercise_id,
    signal(),
  )
  expect(audio).toMatchObject({
    status: 'ready',
    audio_url: expect.stringMatching(/^\/task-audio\/demo_/),
  })
  await expect(
    fetchTrainingAudioFile(audio.audio_url!, signal()),
  ).resolves.toBeInstanceOf(Blob)

  const submission = createTrainingSubmission(
    started.session_id,
    started.current.exercise_id,
    new Blob(['same audio'], { type: 'audio/webm' }),
  )
  const accepted = await submitTraining(submission, signal())
  expect(accepted.answer).toMatchObject({
    sequence: 1,
    round: 1,
    exercise_id: started.current.exercise_id,
    max_score: 2,
  })
  expect(accepted.current.exercise_id).not.toBe(started.current.exercise_id)
  await expect(submitTraining(submission, signal())).resolves.toMatchObject({
    answer: accepted.answer,
  })
  const reconstructed = { ...submission, body: new FormData() }
  reconstructed.body.append(
    'audio',
    new Blob(['same audio'], { type: 'audio/webm' }),
    'answer.webm',
  )
  reconstructed.body.append('exercise_id', submission.exerciseId)
  await expect(submitTraining(reconstructed, signal())).resolves.toMatchObject({
    answer: accepted.answer,
  })
  const conflicting = { ...submission, body: new FormData() }
  conflicting.body.append('audio', new Blob(['different audio']), 'answer.webm')
  conflicting.body.append('exercise_id', submission.exerciseId)
  await expect(submitTraining(conflicting, signal())).rejects.toMatchObject({
    status: 409,
    code: 'IDEMPOTENCY_KEY_REUSED',
  })

  const next = createTrainingSubmission(
    accepted.session_id,
    accepted.current.exercise_id,
    new Blob(['next audio']),
  )
  const nextAccepted = await submitTraining(next, signal())
  expect(nextAccepted.answer).toMatchObject({ sequence: 2, round: 2 })
  const firstPage = await readTrainingHistory(
    started.session_id,
    undefined,
    1,
    signal(),
  )
  expect(firstPage.items).toHaveLength(1)
  expect(firstPage.next_cursor).toBe('1')
  const secondPage = await readTrainingHistory(
    started.session_id,
    firstPage.next_cursor,
    1,
    signal(),
  )
  expect(secondPage.items).toHaveLength(1)
  expect(secondPage.items[0].sequence).toBe(2)
})

it('keeps diagnostic sessions private to their creating mock user', async () => {
  const diagnosticId = await completeDiagnostic()
  const otherUser = 'other-mock-user'
  const sessionPath = `/api/v1/diagnostic-sessions/${diagnosticId}`
  const requests: Array<[string, string, RequestInit]> = [
    [sessionPath, 'GET', {}],
    [`${sessionPath}/answers`, 'POST', { body: new FormData() }],
    [`${sessionPath}/result`, 'GET', {}],
    [`${sessionPath}/feedback`, 'GET', {}],
  ]
  for (const [path, method, options] of requests) {
    const response = mockDiagnosticResponse(
      otherUser,
      path,
      method,
      {},
      options,
    )
    expect(response?.status).toBe(404)
  }
})

it('starts free practice from the completed diagnostic plan revision', async () => {
  const diagnosticId = await completeDiagnostic()
  const training = await startTraining(
    diagnosticId,
    'free-practice-start',
    1,
    signal(),
  )
  expect(training).toMatchObject({ mode: 'free_practice', status: 'active' })
})

it('clears all learning state at logout and login', async () => {
  const diagnosticId = await completeDiagnostic()
  const training = await startTraining(
    diagnosticId,
    'state-reset',
    undefined,
    signal(),
  )
  expect(training.session_id).toBeTruthy()
  await mockApiFetch('/api/v1/auth/logout', { method: 'POST' })
  await mockApiFetch('/api/v1/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: '', password: '' }),
  })
  await expect(
    readLearningState('subject:demo-a', signal()),
  ).resolves.toMatchObject({
    diagnostic_status: 'not_started',
    diagnostic_completed: false,
    training_available: false,
  })
  await expect(
    readTrainingSession(training.session_id, signal()),
  ).rejects.toMatchObject({ status: 404 })
})
