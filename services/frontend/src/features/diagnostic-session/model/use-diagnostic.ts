import { useRef } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import {
  DiagnosticApiError,
  type DiagnosticAnswer,
  type DiagnosticSession,
} from '@/entities/diagnostic-session'
import {
  captureSession,
  isCurrentSession,
  registerSessionRequest,
  replaceSession,
} from '@/entities/user'
import { createAnswerKeys } from './answer-key'
import { useDiagnosticDependencies } from './dependencies-context'
import { diagnosticSessionQueryKeys } from './query-keys'

// Runs a request on behalf of the signed-in user: a sign-out aborts it, a
// result for another user is dropped and a 401 ends the session.
async function scoped<T>(
  client: QueryClient,
  userId: string,
  signal: AbortSignal,
  request: (signal: AbortSignal) => Promise<T>,
) {
  const token = captureSession(client)
  const controller = new AbortController()
  const unregister = registerSessionRequest(client, token, controller)
  try {
    const value = await request(AbortSignal.any([signal, controller.signal]))
    if (!isCurrentSession(client, token) || token.userId !== userId)
      throw new DOMException('Stale session', 'AbortError')
    return value
  } catch (error) {
    if (
      error instanceof DiagnosticApiError &&
      error.status === 401 &&
      isCurrentSession(client, token)
    )
      await replaceSession(client, null)
    throw error
  } finally {
    unregister()
  }
}

export function useDiagnosticHistoryQuery(userId: string, subjectId: string) {
  const { diagnostic } = useDiagnosticDependencies()
  const client = useQueryClient()
  // eslint-disable-next-line @tanstack/query/exhaustive-deps -- the API port and auth epoch are stable dependencies outside the resource key.
  return useQuery({
    queryKey: diagnosticSessionQueryKeys.history(userId, subjectId),
    enabled: !!userId && !!subjectId,
    queryFn: ({ signal }) =>
      scoped(client, userId, signal, (s) =>
        diagnostic.listDiagnostics(subjectId, s),
      ),
  })
}

const ACTIVE_REFRESH_MS = 20_000

// A diagnostic only moves forward: turns are appended and a completed or
// cancelled session stays so. A read that started before an accepted answer
// and finished after it would roll the dialogue back to the old turn, so the
// older of the two snapshots is dropped.
function newerSession(
  current: DiagnosticSession | undefined,
  fetched: DiagnosticSession,
) {
  if (!current || current.id !== fetched.id) return fetched
  if (fetched.turns.length < current.turns.length) return current
  if (
    fetched.turns.length === current.turns.length &&
    current.status !== 'active' &&
    fetched.status === 'active'
  )
    return current
  return fetched
}

export function useDiagnosticQuery(userId: string, sessionId: string) {
  const { diagnostic } = useDiagnosticDependencies()
  const client = useQueryClient()
  const key = diagnosticSessionQueryKeys.session(userId, sessionId)
  // eslint-disable-next-line @tanstack/query/exhaustive-deps -- the API port and auth epoch are stable dependencies outside the resource key.
  return useQuery({
    queryKey: key,
    enabled: !!userId && !!sessionId,
    queryFn: async ({ signal }) => {
      const fetched = await scoped(client, userId, signal, (s) =>
        diagnostic.readDiagnostic(sessionId, s),
      )
      // Compared with the cache as it is when the read lands, not as it
      // was when the read started.
      return newerSession(client.getQueryData<DiagnosticSession>(key), fetched)
    },
    // Reading an active diagnostic is what notices a rebuilt course graph,
    // so the student learns it while thinking, not after answering.
    refetchInterval: (query) =>
      query.state.data?.status === 'active' ? ACTIVE_REFRESH_MS : false,
  })
}

export function useDiagnosticResultQuery(
  userId: string,
  sessionId: string,
  enabled = true,
) {
  const { diagnostic } = useDiagnosticDependencies()
  const client = useQueryClient()
  // eslint-disable-next-line @tanstack/query/exhaustive-deps -- the API port and auth epoch are stable dependencies outside the resource key.
  return useQuery({
    queryKey: diagnosticSessionQueryKeys.result(userId, sessionId),
    enabled: enabled && !!userId && !!sessionId,
    queryFn: ({ signal }) =>
      scoped(client, userId, signal, (s) =>
        diagnostic.readDiagnosticResult(sessionId, s),
      ),
  })
}

function remember(
  client: QueryClient,
  userId: string,
  session: DiagnosticSession,
) {
  client.setQueryData(
    diagnosticSessionQueryKeys.session(userId, session.id),
    session,
  )
  return client.invalidateQueries({
    queryKey: diagnosticSessionQueryKeys.history(userId, session.subjectId),
  })
}

// Starts a diagnostic or resumes the active one. A failed attempt keeps its
// key, so pressing the button again repeats the same request.
export function useStartDiagnosticMutation(userId: string, subjectId: string) {
  const { diagnostic } = useDiagnosticDependencies()
  const client = useQueryClient()
  const key = useRef('')
  return useMutation({
    mutationKey: diagnosticSessionQueryKeys.start(userId, subjectId),
    mutationFn: () => {
      key.current ||= `start:${crypto.randomUUID()}`
      return scoped(client, userId, new AbortController().signal, (s) =>
        diagnostic.startDiagnostic(subjectId, key.current, s),
      )
    },
    onSuccess: async (session) => {
      key.current = ''
      await remember(client, userId, session)
    },
  })
}

// Sends the reply to the current Geek turn; the response holds the next turn.
export function useAnswerMutation(userId: string, sessionId: string) {
  const { diagnostic } = useDiagnosticDependencies()
  const client = useQueryClient()
  const keys = useRef(createAnswerKeys())
  return useMutation({
    mutationKey: diagnosticSessionQueryKeys.answer(userId, sessionId),
    // A background read in flight holds the session before this answer; it
    // is cancelled so it cannot land after the accepted result.
    onMutate: () =>
      client.cancelQueries({
        queryKey: diagnosticSessionQueryKeys.session(userId, sessionId),
      }),
    // display is what the dialogue shows while the answer is being sent.
    mutationFn: ({
      turnId,
      answer,
    }: {
      turnId: string
      answer: DiagnosticAnswer
      display: string
    }) =>
      scoped(client, userId, new AbortController().signal, (s) =>
        diagnostic.answerDiagnostic(
          sessionId,
          turnId,
          answer,
          keys.current(turnId, answer),
          s,
        ),
      ),
    onSuccess: (session) => remember(client, userId, session),
    onError: (error) => {
      // A stale turn means the session moved on, for example in another tab.
      if (error instanceof DiagnosticApiError && error.status === 409)
        void client.invalidateQueries({
          queryKey: diagnosticSessionQueryKeys.session(userId, sessionId),
        })
    },
  })
}
