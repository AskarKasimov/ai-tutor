import {
  act,
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
  pending = false,
  cleaning = false,
  cleaningFailed = false,
  graphReady = false,
  fixture = false,
  uploadGate,
}: {
  processing?: boolean
  pending?: boolean
  cleaning?: boolean
  cleaningFailed?: boolean
  graphReady?: boolean
  // Before the first build the backend serves the default (fixture) graph;
  // a requested build then publishes the graph from the materials.
  fixture?: boolean
  uploadGate?: Promise<void>
} = {}) {
  let graphRequested = false
  let graphReads = 0
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
          : pending
            ? { ...material, status: 'pending' }
            : cleaning
              ? {
                  ...material,
                  cleaning: {
                    status: 'processing',
                    updated_at: 1,
                    attempts: 1,
                    error_message: '',
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
    if (fixture && (url.endsWith('/graph') || url.endsWith('/graph/build'))) {
      if (init?.method === 'POST') graphRequested = true
      else if (graphRequested) graphReads += 1
      const built = graphRequested && graphReads > 0
      return Response.json({
        status: built ? 'ready' : graphRequested ? 'pending' : 'not_requested',
        attempts: 0,
        error_message: '',
        graph: {
          topics: [
            built
              ? { id: 'ml', title: 'Тема из лекции' }
              : { id: 'AI-01-01', title: 'Подходы к определению ИИ' },
          ],
          prerequisites: [],
          hierarchy: [],
        },
        graph_source: built ? 'built' : 'fixture',
        stale: false,
        inputs_ready: true,
        updated_at: built ? 2 : 0,
      })
    }
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
    await screen.findByText(
      'Дождитесь завершения обработки материалов: распознавания, перевода и очистки текста.',
    ),
  ).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /Собрать граф/ })).toBeDisabled()
})

it('keeps the master graph viewer for a published graph', async () => {
  setup({ graphReady: true })
  open()
  expect(
    await screen.findByText('Курс готов для студентов. В графе тем: 1.'),
  ).toBeInTheDocument()
  // The graph opens as a map without an extra click.
  const canvas = await screen.findByRole('region', {
    name: 'Граф знаний курса',
  })
  expect(within(canvas).getByText('Линейная регрессия')).toBeInTheDocument()
  expect(screen.queryByText('Демо-граф')).not.toBeInTheDocument()
})

it('draws the default graph without treating the course as built', async () => {
  setup({ fixture: true })
  open()
  const canvas = await screen.findByRole('region', {
    name: 'Граф знаний курса',
  })
  expect(
    within(canvas).getByText('Подходы к определению ИИ'),
  ).toBeInTheDocument()
  expect(screen.getByText('Демо-граф')).toBeVisible()
  // Everything else still waits for a graph built from the materials.
  expect(screen.getByText('Не готов')).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'Собрать граф тем' }),
  ).toBeInTheDocument()
  expect(screen.queryByText(/Курс готов для студентов/)).not.toBeInTheDocument()
})

it('replaces the default graph once a build from the materials succeeds', async () => {
  setup({ fixture: true })
  open()
  fireEvent.click(
    await screen.findByRole('button', { name: 'Собрать граф тем' }),
  )
  const dialog = await screen.findByRole('alertdialog')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Начать сборку' }))
  expect(
    await screen.findByText('Тема из лекции', undefined, { timeout: 8000 }),
  ).toBeInTheDocument()
  expect(screen.queryByText('Подходы к определению ИИ')).not.toBeInTheDocument()
  expect(screen.queryByText('Демо-граф')).not.toBeInTheDocument()
  expect(
    screen.getByText('Курс готов для студентов. В графе тем: 1.'),
  ).toBeInTheDocument()
}, 15000)

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

it('asks before leaving a course with an unsent material in the queue', async () => {
  setup()
  const router = open()
  fireEvent.change(await screen.findByLabelText('Добавить PDF или ZIP'), {
    target: { files: [new File(['%PDF-1.4 content'], 'unsent.pdf')] },
  })
  expect(await screen.findByText('unsent.pdf')).toBeVisible()

  act(() => {
    void router.navigate({ to: '/admin/courses' })
  })

  expect(
    await screen.findByRole('alertdialog', {
      name: 'Материалы ещё не отправлены',
    }),
  ).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Остаться' }))
  await waitFor(() =>
    expect(router.state.location.pathname).toBe(
      '/admin/competency-map/subject-b',
    ),
  )
  expect(await screen.findByText('unsent.pdf')).toBeVisible()
  expect(screen.getByRole('heading', { name: 'Второй курс' })).toBeVisible()
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
  fireEvent.click(await screen.findByRole('button', { name: 'Понятно' }))
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
  const list = screen
    .getAllByRole('list')
    .find((candidate) => within(candidate).queryByText('lecture.pdf'))
  expect(list).toBeDefined()
  let rows = within(list!).getAllByRole('listitem')
  expect(rows).toHaveLength(2)
  expect(within(rows[0]).getByRole('article')).toHaveTextContent('lecture.pdf')
  expect(within(rows[1]).getByRole('article')).toHaveTextContent('queued.pdf')
  fireEvent.click(
    screen.getByRole('button', { name: 'Переместить lecture.pdf ниже' }),
  )
  rows = within(list!).getAllByRole('listitem')
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
      'У 1 материалов ошибка обработки. Подробности — в карточках; замените файл или удалите его.',
    ),
  ).toBeVisible()
  expect(
    screen.queryByText(
      'Дождитесь завершения обработки материалов: распознавания, перевода и очистки текста.',
    ),
  ).not.toBeInTheDocument()
})

it.each([{ pending: true }, { processing: true }, { cleaning: true }])(
  'hides replacement and deletion while a document is being processed (%j)',
  async (state) => {
    setup(state)
    open()
    expect(await screen.findByText('lecture.pdf')).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Заменить PDF' }),
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Удалить' }),
    ).not.toBeInTheDocument()
  },
)

it.each([{}, { cleaningFailed: true }])(
  'keeps replacement and deletion available after processing (%j)',
  async (state) => {
    setup(state)
    open()
    expect(
      await screen.findByRole('button', { name: 'Заменить PDF' }),
    ).toBeVisible()
    expect(screen.getByRole('button', { name: 'Удалить' })).toBeVisible()
  },
)

it.each([
  [{ processing: true }, 'Переводится на русский'],
  [{ cleaning: true }, 'Очищается текст'],
  [{ pending: true }, 'В очереди на распознавание'],
] as const)(
  'shows the active processing stage and counts only ready materials (%j)',
  async (state, label) => {
    setup(state)
    open()
    expect(await screen.findByText(label)).toBeVisible()
    expect(
      screen.getByText(
        'Материалов: 1 · готово к графу: 0 · в обработке: 1 · с ошибкой: 0',
      ),
    ).toBeVisible()
    expect(screen.queryByText('Готов к графу')).not.toBeInTheDocument()
  },
)

it('marks completed materials ready for the graph', async () => {
  setup()
  open()
  expect(await screen.findByText('Готов к графу')).toBeVisible()
  expect(
    screen.getByText(
      'Материалов: 1 · готово к графу: 1 · в обработке: 0 · с ошибкой: 0',
    ),
  ).toBeVisible()
})
