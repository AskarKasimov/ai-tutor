import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import {
  createMemoryHistory,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { afterEach, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import { routeTree } from '@/routeTree.gen'

const admin = {
  id: 'admin-1',
  email: 'admin@example.com',
  display_name: 'Администратор',
  role: 'admin',
  created_at: 1,
}
const material = {
  id: 'mat-1',
  subject_id: 'subject-b',
  filename: 'lecture.pdf',
  content_type: 'application/pdf',
  file_size: 100,
  s3_raw_key: 'raw',
  s3_parsed_key: 'parsed',
  parsed_text: 'Текст лекции',
  char_count: 12,
  status: 'parsed',
  created_at: 1,
  updated_at: 1,
}
function setup({
  processing = false,
  graphReady = false,
  uploadGate,
}: {
  processing?: boolean
  graphReady?: boolean
  uploadGate?: Promise<void>
} = {}) {
  const calls = vi.fn(async (url: string, init?: RequestInit) => {
    if (url.endsWith('/auth/me')) return Response.json(admin)
    if (url.endsWith('/subjects'))
      return Response.json([
        { id: 'subject-a', name: 'Первый курс', ready: false },
        { id: 'subject-b', name: 'Второй курс', ready: graphReady },
      ])
    if (url.endsWith('/materials') && init?.method === 'POST') {
      await uploadGate
      const files = (init.body as FormData).getAll('file') as File[]
      return Response.json({
        uploaded: files.map((file) => ({ ...material, filename: file.name })),
        failed: [],
      })
    }
    if (url.endsWith('/materials'))
      return Response.json([
        processing
          ? {
              ...material,
              processing: {
                stage: 'translation',
                status: 'processing',
                updated_at: 1,
                attempts: 1,
              },
            }
          : material,
      ])
    if (url.endsWith('/graph') || url.endsWith('/graph/build'))
      return Response.json({
        status:
          init?.method === 'POST'
            ? 'pending'
            : graphReady
              ? 'ready'
              : 'not_requested',
        attempts: 0,
        error_message: '',
        graph: graphReady
          ? {
              topics: [
                {
                  id: 'topic-1',
                  title: 'Линейная регрессия',
                  provides: ['regression'],
                  requires: [],
                },
              ],
              prerequisites: [],
              hierarchy: [],
            }
          : null,
        stale: false,
        inputs_ready: !processing,
        updated_at: 1,
      })
    throw new Error(`Unexpected API call: ${url}`)
  })
  vi.stubGlobal('fetch', calls)
  return calls
}
function open(path = '/admin/competency-map/subject-b') {
  window.history.replaceState({}, '', path)
  const router = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: [path] }),
    routeTree,
  })
  const container = document.createElement('div')
  container.id = 'root'
  document.body.appendChild(container)
  render(<RouterProvider router={router} />, { container })
  return router
}
afterEach(() => vi.unstubAllGlobals())

it('opens the selected course with the master API without metadata or legacy build requests', async () => {
  const calls = setup()
  open()
  expect(
    await screen.findByRole('heading', { name: 'Второй курс' }),
  ).toBeInTheDocument()
  expect(await screen.findByText('lecture.pdf')).toBeInTheDocument()
  expect(
    calls.mock.calls.some(([url]) =>
      url.endsWith('/admin/subjects/subject-b/materials'),
    ),
  ).toBe(true)
  expect(
    calls.mock.calls.some(([url]) => /material-kinds|graph-builds/.test(url)),
  ).toBe(false)
  expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
})

it('shows the master pipeline stage while translation is running and blocks graph builds', async () => {
  setup({ processing: true })
  open()
  expect(await screen.findByText('Переводит материал')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /Собрать граф/ })).toBeDisabled()
})

it('keeps the master graph viewer for a published graph', async () => {
  setup({ graphReady: true })
  open()
  expect(
    await screen.findByText('Граф знаний готов к экзамену'),
  ).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: /Показать/ }))
  expect(await screen.findByText('Линейная регрессия')).toBeInTheDocument()
})

it('uploads a queued PDF using the master multipart field', async () => {
  const calls = setup()
  open()
  const input = await screen.findByLabelText('Добавить PDF или ZIP')
  fireEvent.change(input, {
    target: {
      files: [
        new File(['%PDF-1.4 lecture'], 'new.pdf', { type: 'application/pdf' }),
      ],
    },
  })
  const send = await screen.findByRole('button', { name: /Отправить всё/ })
  fireEvent.click(send)
  await waitFor(() =>
    expect(calls.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(
      true,
    ),
  )
  const request = calls.mock.calls.find(
    ([, init]) => init?.method === 'POST',
  )?.[1]
  const body = request?.body as FormData
  expect(body.getAll('file')).toHaveLength(1)
  expect(body.has('metadata')).toBe(false)
  await waitFor(() =>
    expect(screen.queryByText('new.pdf')).not.toBeInTheDocument(),
  )
})

it('blocks building the graph while a queued upload is in flight', async () => {
  let finish!: () => void
  const uploadGate = new Promise<void>((resolve) => {
    finish = resolve
  })
  setup({ uploadGate })
  open()
  fireEvent.change(await screen.findByLabelText('Добавить PDF или ZIP'), {
    target: { files: [new File(['%PDF-1.4 content'], 'new.pdf')] },
  })
  fireEvent.click(await screen.findByRole('button', { name: 'Отправить всё' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Собрать граф' })).toBeDisabled(),
  )
  finish()
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Собрать граф' })).toBeEnabled(),
  )
})

it('confirms a graph build before using the master build endpoint', async () => {
  const calls = setup()
  open()
  fireEvent.click(await screen.findByRole('button', { name: 'Собрать граф' }))
  const dialog = await screen.findByRole('alertdialog')
  expect(calls.mock.calls.some(([url]) => url.endsWith('/graph/build'))).toBe(
    false,
  )
  fireEvent.click(within(dialog).getByRole('button', { name: 'Начать сборку' }))
  await waitFor(() =>
    expect(
      calls.mock.calls.some(
        ([url, init]) =>
          url.endsWith('/graph/build') && init?.method === 'POST',
      ),
    ).toBe(true),
  )
  expect(
    await screen.findByText('Построение графа знаний курса…'),
  ).toBeInTheDocument()
})
