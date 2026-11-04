import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'
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
