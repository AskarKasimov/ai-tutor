import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { replaceSession } from '@/entities/user'
import { SubjectSelectionDependenciesProvider } from '@/features/subject-selection'
import { AudioPreparationPanel } from '@/widgets/admin-course-detail-shell/ui/audio-preparation-panel'
import type { SubjectApi } from '@/entities/subject'
import '@/shared/i18n'

it('shows ready percentage and every nonzero queue counter for the selected direction', async () => {
  window.history.pushState({}, '', '/admin/courses/s1')
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  await replaceSession(
    client,
    { id: 'admin-user', role: 'admin', email: '', display_name: null },
    'admin',
  )
  const subjects: SubjectApi = {
    listSubjects: async () => [],
    createSubject: async () => {
      throw new Error('unused')
    },
    readAudioPreparation: async () => ({
      subject_id: 's1',
      exam: {
        version: 'graph|voice',
        published: true,
        state: 'processing',
        total: 4,
        unchecked: 0,
        queued: 1,
        processing: 1,
        retry_wait: 1,
        ready: 1,
        failed: 0,
        unavailable: 0,
        observed_at: '2026-11-04T10:00:00Z',
      },
      learn: {
        version: 'revision:1',
        revision: 1,
        state: 'ready',
        total: 1,
        unchecked: 0,
        queued: 0,
        processing: 0,
        retry_wait: 0,
        ready: 1,
        failed: 0,
        unavailable: 0,
        observed_at: '2026-11-04T10:00:00Z',
      },
    }),
  }
  render(
    <QueryClientProvider client={client}>
      <SubjectSelectionDependenciesProvider value={{ subjects }}>
        <AudioPreparationPanel
          userId="admin-user"
          subjectId="s1"
          direction="exam"
        />
      </SubjectSelectionDependenciesProvider>
    </QueryClientProvider>,
  )
  expect(await screen.findByText('Готово 1 из 4 (25%)')).toBeInTheDocument()
  expect(
    screen.getByRole('progressbar', { name: 'Озвучка вопросов: готово 25%' }),
  ).toHaveAttribute('aria-valuenow', '25')
  expect(screen.getByText('В очереди: 1')).toBeInTheDocument()
  expect(screen.getByText('Готовится: 1')).toBeInTheDocument()
  expect(screen.getByText('Ожидает повтора: 1')).toBeInTheDocument()
})

it('explains why exam and learning audio are not counted before publication or map import', async () => {
  window.history.pushState({}, '', '/admin/courses/s2')
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  await replaceSession(
    client,
    { id: 'admin-user', role: 'admin', email: '', display_name: null },
    'admin',
  )
  const direction = {
    version: '',
    state: 'empty' as const,
    total: 0,
    unchecked: 0,
    queued: 0,
    processing: 0,
    retry_wait: 0,
    ready: 0,
    failed: 0,
    unavailable: 0,
    observed_at: '2026-11-04T10:00:00Z',
  }
  const subjects: SubjectApi = {
    listSubjects: async () => [],
    createSubject: async () => {
      throw new Error('unused')
    },
    readAudioPreparation: async () => ({
      subject_id: 's2',
      exam: {
        ...direction,
        version: 'none',
        state: 'no_graph',
        published: false,
      },
      learn: { ...direction, version: 'revision:0', revision: 0 },
    }),
  }
  render(
    <QueryClientProvider client={client}>
      <SubjectSelectionDependenciesProvider value={{ subjects }}>
        <AudioPreparationPanel
          userId="admin-user"
          subjectId="s2"
          direction="exam"
        />
        <AudioPreparationPanel
          userId="admin-user"
          subjectId="s2"
          direction="learn"
        />
      </SubjectSelectionDependenciesProvider>
    </QueryClientProvider>,
  )
  expect(
    await screen.findByText('Озвучка будет учтена после публикации графа.'),
  ).toBeInTheDocument()
  expect(
    screen.getByText('Озвучка будет учтена после импорта карты обучения.'),
  ).toBeInTheDocument()
  expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
})

it('keeps the last audio snapshot visible when the direction becomes unavailable', async () => {
  window.history.pushState({}, '', '/admin/courses/s3')
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  await replaceSession(
    client,
    { id: 'admin-user', role: 'admin', email: '', display_name: null },
    'admin',
  )
  let calls = 0
  const subjects: SubjectApi = {
    listSubjects: async () => [],
    createSubject: async () => {
      throw new Error('unused')
    },
    readAudioPreparation: async () => {
      calls += 1
      if (calls > 1) return { subject_id: 's3', exam: null, learn: null }
      return {
        subject_id: 's3',
        exam: {
          version: 'graph|voice',
          published: true,
          state: 'ready',
          total: 1,
          unchecked: 0,
          queued: 0,
          processing: 0,
          retry_wait: 0,
          ready: 1,
          failed: 0,
          unavailable: 0,
          observed_at: '2026-11-04T10:00:00Z',
        },
        learn: {
          version: 'revision:1',
          revision: 1,
          state: 'empty',
          total: 0,
          unchecked: 0,
          queued: 0,
          processing: 0,
          retry_wait: 0,
          ready: 0,
          failed: 0,
          unavailable: 0,
          observed_at: '2026-11-04T10:00:00Z',
        },
      }
    },
  }
  render(
    <QueryClientProvider client={client}>
      <SubjectSelectionDependenciesProvider value={{ subjects }}>
        <AudioPreparationPanel
          userId="admin-user"
          subjectId="s3"
          direction="exam"
        />
      </SubjectSelectionDependenciesProvider>
    </QueryClientProvider>,
  )
  expect(await screen.findByText('Готово 1 из 1 (100%)')).toBeInTheDocument()
  await client.refetchQueries({ type: 'active' })
  expect(
    await screen.findByText('Не удалось загрузить прогресс озвучки.'),
  ).toBeInTheDocument()
  expect(screen.getByText('Готово 1 из 1 (100%)')).toBeInTheDocument()
  expect(
    screen.getByRole('button', {
      name: 'Повторить загрузку прогресса озвучки',
    }),
  ).toBeInTheDocument()
})

it('resumes polling when a hidden tab becomes visible with unchanged data', async () => {
  window.history.pushState({}, '', '/admin/courses/s1')
  let visibility: DocumentVisibilityState = 'hidden'
  const visibilitySpy = vi
    .spyOn(document, 'visibilityState', 'get')
    .mockImplementation(() => visibility)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  await replaceSession(
    client,
    { id: 'admin-user', role: 'admin', email: '', display_name: null },
    'admin',
  )
  const direction = {
    version: 'v',
    state: 'ready' as const,
    total: 1,
    unchecked: 0,
    queued: 0,
    processing: 0,
    retry_wait: 0,
    ready: 1,
    failed: 0,
    unavailable: 0,
    observed_at: '2026-11-04T10:00:00Z',
  }
  const read = vi.fn(async () => ({
    subject_id: 's1',
    exam: { ...direction, published: true },
    learn: { ...direction, revision: 1 },
  }))
  const subjects: SubjectApi = {
    listSubjects: async () => [],
    createSubject: async () => {
      throw new Error('unused')
    },
    readAudioPreparation: read,
  }
  const view = render(
    <QueryClientProvider client={client}>
      <SubjectSelectionDependenciesProvider value={{ subjects }}>
        <AudioPreparationPanel
          userId="admin-user"
          subjectId="s1"
          direction="exam"
        />
      </SubjectSelectionDependenciesProvider>
    </QueryClientProvider>,
  )
  try {
    await screen.findByText('Готово 1 из 1 (100%)')
    visibility = 'visible'
    window.dispatchEvent(new Event('visibilitychange'))
    await waitFor(() => expect(read).toHaveBeenCalledTimes(2))
    await waitFor(
      () => expect(read.mock.calls.length).toBeGreaterThanOrEqual(3),
      { timeout: 6500 },
    )
  } finally {
    view.unmount()
    client.clear()
    visibilitySpy.mockRestore()
  }
}, 9000)

it('shows learning progress when exam progress is unavailable', async () => {
  window.history.pushState({}, '', '/admin/courses/s1')
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  await replaceSession(
    client,
    { id: 'admin-user', role: 'admin', email: '', display_name: null },
    'admin',
  )
  const subjects: SubjectApi = {
    listSubjects: async () => [],
    createSubject: async () => {
      throw new Error('unused')
    },
    readAudioPreparation: async () => ({
      subject_id: 's1',
      exam: null,
      learn: {
        version: 'revision:1',
        revision: 1,
        state: 'ready',
        total: 1,
        unchecked: 0,
        queued: 0,
        processing: 0,
        retry_wait: 0,
        ready: 1,
        failed: 0,
        unavailable: 0,
        observed_at: '2026-11-04T10:00:00Z',
      },
    }),
  }
  const view = render(
    <QueryClientProvider client={client}>
      <SubjectSelectionDependenciesProvider value={{ subjects }}>
        <AudioPreparationPanel
          userId="admin-user"
          subjectId="s1"
          direction="learn"
        />
      </SubjectSelectionDependenciesProvider>
    </QueryClientProvider>,
  )
  expect(await screen.findByText('Готово 1 из 1 (100%)')).toBeInTheDocument()
  view.rerender(
    <QueryClientProvider client={client}>
      <SubjectSelectionDependenciesProvider value={{ subjects }}>
        <AudioPreparationPanel
          userId="admin-user"
          subjectId="s1"
          direction="exam"
        />
      </SubjectSelectionDependenciesProvider>
    </QueryClientProvider>,
  )
  expect(
    await screen.findByText('Не удалось загрузить прогресс озвучки.'),
  ).toBeInTheDocument()
  expect(
    screen.getByRole('button', {
      name: 'Повторить загрузку прогресса озвучки',
    }),
  ).toBeInTheDocument()
})
