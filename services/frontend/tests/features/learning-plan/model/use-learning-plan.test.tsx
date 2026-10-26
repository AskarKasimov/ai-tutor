import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import { createAppDependencies } from '@/bootstrap/dependencies'
import {
  LearningPlanApiError,
  type LearningPlan,
} from '@/entities/learning-plan'
import { replaceSession, userQueryKeys } from '@/entities/user'
import { learningPlanQueryKeys } from '@/features/learning-plan'
import {
  useCreateLearningPlan,
  useLessonActions,
} from '@/features/learning-plan'
import { createQueryWrapper } from '../../../support/query-wrapper'

const user = {
  id: 'student',
  email: 'student@example.com',
  display_name: null,
  role: 'student' as const,
}
const plan = (progress: number): LearningPlan => ({
  id: 'plan-1',
  diagnostic_session_id: 'diagnostic-1',
  subject_id: 'subject-1',
  subject_name: 'Курс',
  hours: 1,
  detail: 'brief',
  status: 'active',
  current_index: 0,
  progress_percent: progress,
  planned_minutes: 30,
  pending_answer: null,
  items: [],
})

afterEach(() => vi.restoreAllMocks())

it.each(['create', 'open', 'answer', 'retry'] as const)(
  'ignores a late %s response after the same user starts a new session epoch',
  async (operation) => {
    for (const outcome of ['success', '401'] as const) {
      const dependencies = createAppDependencies()
      let resolveOld!: (value: LearningPlan) => void
      let rejectOld!: (error: Error) => void
      let oldSignal!: AbortSignal
      const deferred = () =>
        new Promise<LearningPlan>((resolve, reject) => {
          resolveOld = resolve
          rejectOld = reject
        })
      const api = dependencies.learningPlan
      if (operation === 'create')
        vi.spyOn(api, 'createLearningPlan').mockImplementation(
          (_diagnosticId, _hours, signal) => {
            oldSignal = signal
            return deferred()
          },
        )
      else if (operation === 'open')
        vi.spyOn(api, 'openLesson').mockImplementation((_planId, signal) => {
          oldSignal = signal
          return deferred()
        })
      else if (operation === 'answer')
        vi.spyOn(api, 'answerLesson').mockImplementation(
          (_planId, _answer, signal) => {
            oldSignal = signal
            return deferred()
          },
        )
      else
        vi.spyOn(api, 'retryLessonAnswer').mockImplementation(
          (_planId, signal) => {
            oldSignal = signal
            return deferred()
          },
        )

      const client = createQueryClient()
      await replaceSession(client, user)
      client.setQueryData(userQueryKeys.auth, user)
      const wrapper = createQueryWrapper(client, dependencies)
      const view = renderHook(
        () => {
          if (operation === 'create')
            return {
              kind: 'create' as const,
              mutation: useCreateLearningPlan(user.id, 'diagnostic-1'),
            }
          return {
            kind: 'actions' as const,
            actions: useLessonActions(user.id, plan(0)),
          }
        },
        { wrapper },
      )
      let pending!: Promise<unknown>
      await act(async () => {
        const result = view.result.current
        if (result.kind === 'create') pending = result.mutation.mutateAsync(1)
        else if (operation === 'open')
          pending = result.actions.open.mutateAsync()
        else if (operation === 'answer')
          pending = result.actions.answer.mutateAsync(new Blob(['answer']))
        else pending = result.actions.retry.mutateAsync()
        pending.catch(() => undefined)
      })
      await waitFor(() => expect(oldSignal).toBeDefined())

      await act(async () => {
        await replaceSession(client, null)
        await replaceSession(client, user)
      })
      const fresh = plan(100)
      const key = learningPlanQueryKeys.byDiagnostic(user.id, 'diagnostic-1')
      client.setQueryData(key, fresh)
      await act(async () => {
        if (outcome === 'success') resolveOld(plan(0))
        else rejectOld(new LearningPlanApiError(401))
        await pending.catch(() => undefined)
      })

      expect(oldSignal.aborted).toBe(true)
      expect(client.getQueryData(key)).toEqual(fresh)
      expect(client.getQueryData(userQueryKeys.auth)).toEqual(user)
      view.unmount()
    }
  },
)
