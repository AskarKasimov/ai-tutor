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
  parsed_text: '# Тема лекции\n\nФормула: $y = Wx + b$',
  char_count: 12,
  status: 'parsed',
  week: 1,
  kind: 'lecture',
  created_at: 1,
  updated_at: 1,
}
function setup({
  processing = false,
  cleaningFailed = false,
  graphReady = false,
  uploadGate,
}: {
  processing?: boolean
  cleaningFailed?: boolean
  graphReady?: boolean
  uploadGate?: Promise<void>
} = {}) {
  let graphRequested = false
  const calls = vi.fn(async (url: string, init?: RequestInit) => {
    if (url.endsWith('/auth/me')) return Response.json(admin)
    if (url.endsWith('/admin/material-kinds'))
      return Response.json([
        { code: 'lecture', label: 'Лекция' },
        { code: 'lab', label: 'Лабораторная' },
      ])
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
        results: JSON.parse((init.body as FormData).get('items') as string).map(
          (item: { client_id: string }, index: number) => ({
            client_id: item.client_id,
            material: { ...material, filename: files[index].name },
          }),
        ),
      })
    }
    if (url.endsWith('/materials'))
      return Response.json([
        cleaningFailed
          ? {
              ...material,
              cleaning: {
                status: 'failed',
                updated_at: 2,
                attempts: 3,
                error_message: 'Очистка материала не удалась.',
              },
            }
          : processing
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
    if (url.endsWith('/graph') || url.endsWith('/graph/build')) {
      if (init?.method === 'POST') graphRequested = true
      return Response.json({
        status: graphRequested
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
        inputs_ready: !processing && !cleaningFailed,
        updated_at: 1,
      })
    }
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

it('opens the selected course with original metadata controls and master graph API', async () => {
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
  expect(calls.mock.calls.some(([url]) => /graph-builds/.test(url))).toBe(false)
  expect(screen.getByRole('combobox', { name: 'Тип материала' })).toBeVisible()
  expect(screen.getByRole('spinbutton', { name: 'Неделя' })).toHaveValue(1)
})

it('shows the master pipeline stage while translation is running and blocks graph builds', async () => {
  setup({ processing: true })
  open()
  expect(
    await screen.findByText('Дождитесь завершения распознавания материалов.'),
  ).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /Собрать граф/ })).toBeDisabled()
})

it('keeps the master graph viewer for a published graph', async () => {
  setup({ graphReady: true })
  open()
  expect(
    await screen.findByText('Курс готов для студентов. В графе тем: 1.'),
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
  expect(body.has('items')).toBe(true)
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
    expect(
      screen.getByRole('button', { name: 'Собрать граф тем' }),
    ).toBeDisabled(),
  )
  finish()
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Собрать граф тем' }),
    ).toBeEnabled(),
  )
})

it('confirms a graph build before using the master build endpoint', async () => {
  const calls = setup()
  open()
  fireEvent.click(
    await screen.findByRole('button', { name: 'Собрать граф тем' }),
  )
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
  expect(await screen.findByText('В очереди')).toBeInTheDocument()
})

it('preserves the three numbered preparation blocks and arrows in their original order', async () => {
  setup()
  open()
  const pipeline = await screen.findByRole('navigation', {
    name: 'Этапы подготовки курса',
  })
  expect(
    [...pipeline.children].map((element) => element.tagName.toLowerCase()),
  ).toEqual(['div', 'svg', 'div', 'svg', 'div'])
  const blocks = [...pipeline.children].filter(
    (element) => element.tagName === 'DIV',
  )
  expect(blocks.map((element) => element.textContent)).toEqual([
    '1Материалы',
    '2Граф тем',
    '3Готов студентам',
  ])
})

it('uses one card list for queued and saved materials and keeps reorder controls', async () => {
  const calls = setup()
  open()
  fireEvent.change(await screen.findByLabelText('Добавить PDF или ZIP'), {
    target: { files: [new File(['%PDF-1.4 new'], 'queued.pdf')] },
  })
  await screen.findByText('queued.pdf')
  const list = screen.getByRole('list')
  let rows = within(list).getAllByRole('listitem')
  expect(rows).toHaveLength(2)
  expect(within(rows[0]).getByRole('article')).toHaveTextContent('lecture.pdf')
  expect(within(rows[1]).getByRole('article')).toHaveTextContent('queued.pdf')
  fireEvent.click(
    screen.getByRole('button', { name: 'Переместить lecture.pdf ниже' }),
  )
  rows = within(list).getAllByRole('listitem')
  expect(rows[0]).toHaveTextContent('queued.pdf')
  expect(rows[1]).toHaveTextContent('lecture.pdf')
  expect(calls.mock.calls.some(([, init]) => init?.method === 'PATCH')).toBe(
    false,
  )
})

it('opens rendered Markdown and formulas in a closable preview dialog', async () => {
  setup()
  open()
  fireEvent.click(
    await screen.findByRole('button', {
      name: 'Предпросмотр',
    }),
  )
  const dialog = await screen.findByRole('dialog')
  expect(
    within(dialog).getByRole('heading', { name: 'Тема лекции' }),
  ).toBeVisible()
  expect(dialog.querySelector('.katex')).toBeInTheDocument()
  fireEvent.click(
    within(dialog).getByRole('button', { name: 'Закрыть предпросмотр' }),
  )
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument(),
  )
})

it('shows terminal cleaning failure in the original card without a pending blocker', async () => {
  setup({ cleaningFailed: true, graphReady: true })
  open()
  expect(await screen.findByText('Очистка материала не удалась.')).toBeVisible()
  expect(
    screen.getByText(
      'У 1 материалов ошибка распознавания: удалите их или загрузите заново.',
    ),
  ).toBeVisible()
  expect(
    screen.queryByText('Дождитесь завершения распознавания материалов.'),
  ).not.toBeInTheDocument()
})
