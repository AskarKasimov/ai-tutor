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
  competencyMap,
  materials,
  uploadGate,
  assessmentWarnings = [],
}: {
  processing?: boolean
  pending?: boolean
  cleaning?: boolean
  cleaningFailed?: boolean
  graphReady?: boolean
  materials?: (typeof material)[]
  // The import of a competency map: a bad map names its cells; a good one is
  // built from its first sheet and the build publishes the graph of the map.
  competencyMap?: 'valid' | 'invalid'
  uploadGate?: Promise<void>
  assessmentWarnings?: { row: number; column: string; message: string }[]
} = {}) {
  let graphRequested = false
  let mapImported = false
  const courseMaterials = materials ?? [material]
  const calls = vi.fn(async (url: string, init?: RequestInit) => {
    if (url.endsWith('/auth/me')) return Response.json(admin)
    if (url.endsWith('/admin/material-kinds'))
      return Response.json([
        { code: 'lecture', label: 'Лекция' },
        { code: 'lab', label: 'Лабораторная' },
      ])
    if (url.endsWith('/subjects'))
      return Response.json([
        {
          id: 'subject-a',
          name: 'Первый курс',
          ready: false,
          learning_ready: false,
          exam_ready: false,
        },
        {
          id: 'subject-b',
          name: 'Второй курс',
          ready: graphReady || mapImported,
          learning_ready: false,
          exam_ready: graphReady || mapImported,
        },
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
      return Response.json(
        courseMaterials.length === 0
          ? []
          : [
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
            ],
      )
    if (url.endsWith('/graph/competency-map')) {
      if (competencyMap === 'invalid')
        return Response.json(
          {
            code: 'COMPETENCY_MAP_INVALID',
            message: 'Карта на листе «Карта» содержит ошибки.',
            details: [
              {
                path: 'file',
                code: 'COMPETENCY_MAP_CELL',
                message:
                  'Строка 3, «Уровень по Блуму»: Неизвестный уровень по Блуму.',
              },
            ],
          },
          { status: 422 },
        )
      mapImported = true
      return Response.json(
        {
          status: 'pending',
          attempts: 0,
          error_message: '',
          graph: null,
          graph_source: 'none',
          build_source: 'competency_map',
          map: { filename: 'map.xlsx', sheet: 'Карта компетенций v2' },
          stale: false,
          inputs_ready: true,
          updated_at: 0,
        },
        { status: 202 },
      )
    }
    if (mapImported && url.endsWith('/graph'))
      return Response.json({
        status: 'ready',
        attempts: 0,
        error_message: '',
        graph: {
          assessment_warnings: assessmentWarnings,
          topics: [
            { id: 'basic', title: 'Называет части агента' },
            { id: 'main', title: 'Выбирает тип агента' },
          ],
          prerequisites: [{ source: 'basic', target: 'main' }],
          hierarchy: [{ source: 'main', target: 'basic' }],
        },
        graph_source: 'competency_map',
        build_source: 'competency_map',
        map: { filename: 'map.xlsx', sheet: 'Карта компетенций v2' },
        stale: false,
        inputs_ready: true,
        updated_at: 2,
      })
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
              assessment_warnings: assessmentWarnings,
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
function open(path = '/admin/courses/subject-b?direction=exam') {
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

it('shows which imported tasks will use default assessment criteria', async () => {
  setup({
    graphReady: true,
    assessmentWarnings: [
      { row: 3, column: 'Задание 2', message: 'Используются общие критерии.' },
    ],
  })
  open()
  expect(
    await screen.findByText(/Для части заданий критерии в карте не заполнены/),
  ).toBeVisible()
  expect(
    screen.getByText('Строка 3, Задание 2: Используются общие критерии.'),
  ).toBeVisible()
  expect(screen.getByText('Общие критерии — максимум 4 балла:')).toBeVisible()
  expect(screen.getByText(/Предметная корректность \(0–2\): 2 —/)).toBeVisible()
  expect(screen.getByText(/Покрытие вопроса \(0–2\): 2 —/)).toBeVisible()
  expect(
    screen.getByText(/Объяснение, пример, расчёт или сравнение обязательны/),
  ).toBeVisible()
})

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
  expect(screen.queryByText('По карте компетенций')).not.toBeInTheDocument()
  expect(
    screen.queryByText('Общие критерии — максимум 4 балла:'),
  ).not.toBeInTheDocument()
})

it('keeps the competency map import inside the selected course graph card', async () => {
  setup()
  open('/admin/courses/subject-b?direction=exam')
  const graphSection = await screen.findByRole('region', {
    name: 'Граф тем курса',
  })
  expect(
    within(graphSection).getByText(
      /Вместо материалов загрузите карту компетенций/,
    ),
  ).toBeVisible()
  expect(
    within(graphSection).getByRole('button', {
      name: 'Импортировать карту компетенций',
    }),
  ).toBeVisible()
  expect(
    screen.queryByRole('heading', {
      name: 'Карта для экзаменационного графа',
    }),
  ).not.toBeInTheDocument()
})

function chooseMap() {
  return screen
    .findByLabelText('Импортировать карту компетенций', { selector: 'input' })
    .then((input) =>
      fireEvent.change(input, {
        target: {
          files: [new File(['PK'], 'map.xlsx', { type: 'application/zip' })],
        },
      }),
    )
}

it('builds the graph from the first sheet of a competency map', async () => {
  const calls = setup({ competencyMap: 'valid' })
  open()
  await chooseMap()
  const dialog = await screen.findByRole('alertdialog')
  expect(within(dialog).getByText(/по карте «map\.xlsx»/)).toBeVisible()
  fireEvent.click(within(dialog).getByRole('button', { name: 'Импортировать' }))
  expect(
    await screen.findByText('По карте компетенций', undefined, {
      timeout: 8000,
    }),
  ).toBeVisible()
  expect(
    screen.getByText('Последняя карта: map.xlsx, лист «Карта компетенций v2».'),
  ).toBeVisible()
  expect(
    screen.getByText('Курс готов для студентов. В графе тем: 2.'),
  ).toBeInTheDocument()
  const imports = calls.mock.calls.filter(([url]) =>
    String(url).endsWith('/graph/competency-map'),
  )
  // One send, with the file only: the server reads the first sheet.
  expect(imports).toHaveLength(1)
  expect((imports[0][1]?.body as FormData).has('sheet')).toBe(false)
  expect(screen.queryByLabelText('Лист карты')).not.toBeInTheDocument()
}, 15000)

it('imports a map through the shared course ID and makes a map-only course ready', async () => {
  const calls = setup({ competencyMap: 'valid', materials: [] })
  const router = open('/admin/courses/subject-b?direction=exam')
  expect(
    await screen.findByRole('heading', { name: 'Второй курс', level: 1 }),
  ).toBeVisible()
  expect(await screen.findByText(/Материалов: 0/)).toBeVisible()
  await chooseMap()
  const dialog = await screen.findByRole('alertdialog')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Импортировать' }))
  await screen.findByText('По карте компетенций', undefined, { timeout: 8000 })
  expect(await screen.findByText('Экзамен готов')).toBeVisible()
  expect(router.state.location.pathname).toBe('/admin/courses/subject-b')
  expect(router.state.location.search).toEqual({ direction: 'exam' })
  expect(
    calls.mock.calls.some(([url]) =>
      String(url).endsWith('/admin/subjects/subject-b/graph/competency-map'),
    ),
  ).toBe(true)
  expect(
    calls.mock.calls.some(([url]) =>
      String(url).endsWith('/admin/subjects/subject-b/materials'),
    ),
  ).toBe(true)
}, 15000)

it('keeps the common course heading and exactly one selected direction tab', async () => {
  setup()
  const router = open('/admin/courses/subject-b?direction=exam')
  expect(
    await screen.findByRole('heading', { name: 'Второй курс', level: 1 }),
  ).toBeVisible()
  const navigation = screen.getByRole('navigation', {
    name: 'Разделы администрирования',
  })
  const selected = () =>
    within(navigation)
      .getAllByRole('link')
      .filter((tab) => tab.getAttribute('aria-current') === 'page')
  expect(selected()).toHaveLength(1)
  expect(selected()[0]).toHaveAccessibleName('Материалы для экзамена')
  fireEvent.click(
    within(navigation).getByRole('link', { name: 'Материалы для обучения' }),
  )
  expect(
    await screen.findByRole('heading', { name: 'Материалы для обучения' }),
  ).toBeVisible()
  expect(router.state.location.search).toEqual({ direction: 'learn' })
  const navigationAfterSwitch = screen.getByRole('navigation', {
    name: 'Разделы администрирования',
  })
  const selectedAfterSwitch = within(navigationAfterSwitch)
    .getAllByRole('link')
    .filter((tab) => tab.getAttribute('aria-current') === 'page')
  expect(selectedAfterSwitch).toHaveLength(1)
  expect(selectedAfterSwitch[0]).toHaveAccessibleName('Материалы для обучения')
  expect(
    await screen.findByRole('heading', { name: 'Второй курс', level: 1 }),
  ).toBeVisible()
})

it('names the bad cells of a rejected competency map', async () => {
  setup({ competencyMap: 'invalid' })
  open()
  await chooseMap()
  const dialog = await screen.findByRole('alertdialog')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Импортировать' }))
  const alert = await screen.findByText(
    'Строка 3, «Уровень по Блуму»: Неизвестный уровень по Блуму.',
  )
  expect(alert).toBeVisible()
  expect(
    screen.getByText('Карта на листе «Карта» содержит ошибки.'),
  ).toBeVisible()
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
  await waitFor(() => {
    expect(router.state.location.pathname).toBe('/admin/courses/subject-b')
    expect(router.state.location.search).toEqual({ direction: 'exam' })
  })
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
