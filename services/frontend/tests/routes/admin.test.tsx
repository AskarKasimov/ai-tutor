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
import { afterEach, beforeAll, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import { routeTree } from '@/routeTree.gen'

// Load the lazy page outside timed user interactions, as in diagnostic tests.
beforeAll(async () => {
  if (!window.URL.createObjectURL) {
    window.URL.createObjectURL = vi.fn(() => 'blob:mock-url')
  }
  if (!window.URL.revokeObjectURL) {
    window.URL.revokeObjectURL = vi.fn()
  }
  await import('@/pages/competency-map')
}, 30000)

const admin = {
  id: 'admin-1',
  email: 'admin@example.com',
  display_name: 'Администратор',
  role: 'admin',
  created_at: 1791158400,
}
const emptyMap = { revision: 0, imported_at: null, competencies: [] }
const importedMap = {
  revision: 2,
  imported_at: 1791356400,
  competencies: [
    {
      id: 'c1',
      name: 'ML',
      constituents: [
        {
          id: 's1',
          name: 'Модели',
          outcomes: [
            {
              id: 'o1',
              name: 'Классификация',
              tasks: [{ id: 't1' }, { id: 't2' }],
            },
          ],
        },
      ],
    },
  ],
}
const imported = {
  revision: 2,
  imported_at: 1791356400,
  competency_count: 1,
  constituent_count: 1,
  outcome_count: 1,
  task_count: 2,
  unparsed_task_cell_count: 3,
  warnings: [
    {
      row: 8,
      column_index: 12,
      column: 'Задание 2',
      code: 'TASK_INVALID_BLOCK_COUNT',
    },
    {
      row: 8,
      column_index: 11,
      column: 'Задание 1',
      code: 'TASK_INVALID_OPTIONS',
    },
    {
      row: 8,
      column_index: 13,
      column: 'Задание 3',
      code: 'TASK_INVALID_BLOCK_COUNT',
    },
  ],
}
const subjects = [
  { id: 'subject-a', name: 'Машинное обучение', ready: true },
  { id: 'subject-b', name: 'Статистика', ready: false },
]

const sampleMaterials = [
  {
    id: 'mat-1',
    subject_id: 'subject-a',
    filename: 'lecture1.pdf',
    content_type: 'application/pdf',
    file_size: 1048576,
    s3_raw_key: 'subjects/subject-a/raw/mat-1_lecture1.pdf',
    s3_parsed_key: 'subjects/subject-a/parsed/mat-1_lecture1.pdf.md',
    parsed_text: '# Лекция 1\n\nФормула: $y = Wx + b$',
    char_count: 35,
    status: 'parsed',
    error_message: null,
    created_at: 1791356400,
    updated_at: 1791356400,
  },
]

// The admin panel is the /admin site; the browser address decides the portal.
function renderApp(path = '/admin') {
  window.history.replaceState({}, '', path)
  const router = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: [path] }),
    routeTree,
  })
  render(<RouterProvider router={router} />)
  return router
}

function backend(
  options: {
    user?: typeof admin | null
    result?: unknown
    status?: number
    subjects?: typeof subjects
    teacherFails?: boolean
    materials?: typeof sampleMaterials
    partialUpload?: boolean
    downloadFails?: boolean
  } = {},
) {
  let active: typeof emptyMap | typeof importedMap = emptyMap
  const activeSubjects = [...(options.subjects ?? subjects)]
  const activeMaterials = [...(options.materials ?? sampleMaterials)]
  let signedIn = options.user !== null
  const fetch = vi.fn(async (url: string, init?: RequestInit) => {
    if (url.endsWith('/auth/refresh'))
      return new Response(null, { status: 401 })
    if (url.endsWith('/auth/me'))
      return signedIn
        ? new Response(JSON.stringify(options.user ?? admin))
        : new Response(null, { status: 401 })
    // The admin panel enters the shared teacher account by itself.
    if (url.endsWith('/auth/teacher')) {
      if (options.teacherFails)
        return Response.json({ message: 'Сервер недоступен.' }, { status: 503 })
      signedIn = true
      return Response.json({ user: admin })
    }
    if (url.endsWith('/auth/logout')) {
      signedIn = false
      return new Response(null, { status: 204 })
    }
    if (url.endsWith('/subjects') && !init?.method)
      return new Response(JSON.stringify(activeSubjects))
    if (url.endsWith('/admin/subjects') && init?.method === 'POST') {
      const subject = { id: 'subject-c', name: 'Новый', ready: false }
      activeSubjects.push(subject)
      return new Response(JSON.stringify(subject))
    }
    if (url.endsWith('/subjects/subject-a/learning-state'))
      return new Response(
        JSON.stringify({
          subject_id: 'subject-a',
          subject_name: 'Машинное обучение',
          diagnostic_status: 'not_started',
          diagnostic_completed: false,
          training_available: false,
        }),
      )
    if (
      url.includes('/subjects/') &&
      url.endsWith('/competency-map') &&
      !init?.method
    )
      return new Response(JSON.stringify(active))
    if (
      url.includes('/admin/subjects/') &&
      url.endsWith('/competency-map/import')
    ) {
      if (!options.status || options.status === 200) active = importedMap
      return new Response(JSON.stringify(options.result ?? imported), {
        status: options.status ?? 200,
      })
    }
    if (
      url.includes('/admin/subjects/') &&
      url.endsWith('/materials') &&
      !init?.method
    ) {
      return new Response(JSON.stringify(activeMaterials))
    }
    if (
      url.includes('/admin/subjects/') &&
      url.endsWith('/materials') &&
      init?.method === 'POST'
    ) {
      if (options.partialUpload) {
        return new Response(
          JSON.stringify({
            uploaded: [],
            failed: [
              {
                filename: 'corrupted.pdf',
                code: 'OCR_FAILED',
                message: 'Ошибка распознавания',
              },
            ],
          }),
        )
      }
      const newMat = {
        id: 'mat-uploaded',
        subject_id: 'subject-a',
        filename: 'new_lecture.pdf',
        content_type: 'application/pdf',
        file_size: 2048,
        s3_raw_key: 'subjects/subject-a/raw/mat-uploaded_new_lecture.pdf',
        s3_parsed_key:
          'subjects/subject-a/parsed/mat-uploaded_new_lecture.pdf.md',
        parsed_text: '# Новая лекция',
        char_count: 14,
        status: 'parsed',
        error_message: null,
        created_at: 1791356400,
        updated_at: 1791356400,
      }
      activeMaterials.push(newMat)
      return new Response(JSON.stringify({ uploaded: [newMat], failed: [] }))
    }
    if (
      url.includes('/admin/subjects/') &&
      url.includes('/materials/') &&
      url.endsWith('/raw')
    ) {
      if (options.downloadFails) {
        return new Response('File not found', { status: 404 })
      }
      return new Response('fake raw bytes', {
        headers: { 'Content-Type': 'application/pdf' },
      })
    }
    if (
      url.includes('/admin/subjects/') &&
      url.includes('/materials/') &&
      init?.method === 'DELETE'
    ) {
      return new Response(null, { status: 204 })
    }
    throw new Error(`Unexpected request: ${url}`)
  })
  vi.stubGlobal('fetch', fetch)
  return fetch
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  window.history.replaceState({}, '', '/')
})

it.each(['/admin', '/admin/'])(
  'opens the map administration by default when the admin panel opens at %s',
  async (path) => {
    backend()
    const router = renderApp(path)
    expect(
      await screen.findByRole('heading', {
        name: 'Карта компетенций',
        level: 1,
      }),
    ).toBeVisible()
    expect(router.state.location.pathname).toBe('/admin/competency-map')
    expect(screen.queryByText('Задания сессии')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Начать запись' }),
    ).not.toBeInTheDocument()
    expect(await screen.findByText('Карта ещё не загружена.')).toBeVisible()
    expect(
      screen.getByRole('heading', { name: 'Машинное обучение', level: 2 }),
    ).toBeVisible()
    expect(screen.queryByText('Статистика')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('combobox', { name: 'Предмет' }),
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('heading', {
        name: 'Загруженные лекции и лабораторные',
        level: 2,
      }),
    ).toBeVisible()
    expect(await screen.findByText('lecture1.pdf')).toBeVisible()
    expect(screen.getByText('Распознан')).toBeVisible()
  },
)

it('does not mount the lesson while the current session is being checked', async () => {
  let finish!: (response: Response) => void
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(new Response('{}', { status: 401 }))
      .mockImplementationOnce(
        () =>
          new Promise<Response>((resolve) => {
            finish = resolve
          }),
      ),
  )
  renderApp()
  expect(await screen.findByRole('status')).toHaveTextContent(
    'Проверяем сессию',
  )
  expect(screen.queryByText('Машинное обучение')).not.toBeInTheDocument()
  await act(async () => finish(new Response('{}', { status: 401 })))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Не удалось открыть админ-панель.',
  )
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})

it('creates a trimmed subject with its map in one step, then switches to editing', async () => {
  const fetch = backend({ subjects: [] })
  renderApp('/admin/competency-map')
  const field = await screen.findByLabelText('Название предмета')
  const submit = screen.getByRole('button', { name: 'Подтвердить' })
  fireEvent.change(field, { target: { value: '  Новый  ' } })
  expect(submit).toBeDisabled()
  fireEvent.change(screen.getByLabelText('Файл карты компетенций'), {
    target: { files: [new File(['map'], 'map.csv')] },
  })
  expect(screen.getByText('map.csv')).toBeVisible()
  fireEvent.click(submit)
  expect(await screen.findByText('Предупреждения импорта')).toBeVisible()
  expect(screen.getByRole('heading', { name: 'Новый', level: 2 })).toBeVisible()
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  const [, init] = fetch.mock.calls.find(
    ([url, request]) =>
      url.endsWith('/admin/subjects') && request?.method === 'POST',
  )!
  expect(JSON.parse(init?.body as string)).toEqual({ name: 'Новый' })
  expect(
    fetch.mock.calls.some(([url]) =>
      url.endsWith('/admin/subjects/subject-c/competency-map/import'),
    ),
  ).toBe(true)
})

it('switches to editing with the error when the first map import fails', async () => {
  backend({
    subjects: [],
    status: 400,
    result: { code: 'INVALID', message: 'Файл повреждён.' },
  })
  renderApp('/admin/competency-map')
  const field = await screen.findByLabelText('Название предмета')
  fireEvent.change(field, { target: { value: 'Новый' } })
  fireEvent.change(screen.getByLabelText('Файл карты компетенций'), {
    target: { files: [new File(['corrupt'], 'corrupt.csv')] },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Подтвердить' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Файл повреждён.')
  expect(screen.getByRole('heading', { name: 'Новый', level: 2 })).toBeVisible()
  expect(screen.queryByText('corrupt.csv')).not.toBeInTheDocument()
})

it('keeps the student site free of the admin panel even for an admin account', async () => {
  backend()
  const router = renderApp('/')
  expect(await screen.findByRole('heading', { name: 'Курсы' })).toBeVisible()
  expect(router.state.location.pathname).toBe('/')
  expect(
    screen.queryByRole('heading', { name: 'Карта компетенций' }),
  ).not.toBeInTheDocument()
  expect(document.querySelector('a[href^="/admin"]')).toBeNull()
})

it('denies students direct access to the import screen without fetching the map', async () => {
  const fetch = backend({ user: { ...admin, role: 'student' } })
  renderApp('/admin/competency-map')
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Доступно только администраторам',
  )
  expect(
    screen.queryByLabelText('Файл карты компетенций'),
  ).not.toBeInTheDocument()
  expect(fetch.mock.calls.every(([url]) => url.endsWith('/auth/me'))).toBe(true)
})

it.each(['csv', 'xlsx'])(
  'confirms and uploads a %s file, then refreshes the active map and displays warnings',
  async (extension) => {
    const fetch = backend()
    renderApp()
    const input = await screen.findByLabelText('Файл карты компетенций')
    fireEvent.change(input, {
      target: { files: [new File(['map'], `map.${extension}`, { type: '' })] },
    })
    fireEvent.click(
      screen.getByRole('button', { name: 'Подтвердить изменения' }),
    )
    const dialog = await screen.findByRole('alertdialog')
    expect(dialog).toHaveTextContent(
      'Текущая карта и задания предмета будут заменены',
    )
    expect(fetch.mock.calls.some(([url]) => url.endsWith('/import'))).toBe(
      false,
    )
    fireEvent.click(
      within(dialog).getByRole('button', { name: 'Перезаписать' }),
    )
    expect(await screen.findByText('Предупреждения импорта')).toBeVisible()
    // Fragments of one spreadsheet row read as a single entry.
    const rows = screen.getAllByRole('row')
    expect(rows).toHaveLength(2)
    expect(rows[1]).toHaveTextContent(
      '8Задание 1: варианты должны быть нумерованным Markdown-списком или [] для задания без выбора.' +
        'Задание 2, Задание 3: нужны четыре блока: вопрос, варианты, голосовая инструкция и эталон. Разделитель — две пустые строки.',
    )
    expect(screen.getByText(/строк: 1/)).toBeVisible()
    await waitFor(() =>
      expect(
        screen.getByRole('region', { name: 'Машинное обучение' }),
      ).toHaveTextContent(/Версия карты\s*2/),
    )
    const [, init] = fetch.mock.calls.find(([url]) => url.endsWith('/import'))!
    const [importUrl] = fetch.mock.calls.find(([url]) =>
      url.endsWith('/import'),
    )!
    expect(importUrl).toBe(
      '/api/v1/admin/subjects/subject-a/competency-map/import',
    )
    expect(init?.credentials).toBe('include')
    // The browser sets the multipart boundary; only the portal header is ours.
    const headers = new Headers(init?.headers)
    expect(headers.get('Content-Type')).toBeNull()
    expect(headers.get('X-AI-Tutor-Portal')).toBe('admin')
    const file = (init?.body as FormData).get('file') as File
    expect(file.name).toBe(`map.${extension}`)
    expect(file.type).toBe(
      extension === 'csv'
        ? 'text/csv'
        : 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    )
  },
)

it('shows no import block after a clean import', async () => {
  backend({
    result: { ...imported, unparsed_task_cell_count: 0, warnings: [] },
  })
  renderApp()
  fireEvent.change(await screen.findByLabelText('Файл карты компетенций'), {
    target: { files: [new File(['map'], 'map.csv')] },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Подтвердить изменения' }))
  fireEvent.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: 'Перезаписать',
    }),
  )
  await waitFor(() =>
    expect(
      screen.getByRole('region', { name: 'Машинное обучение' }),
    ).toHaveTextContent(/Версия карты\s*2/),
  )
  expect(screen.queryByText('Предупреждения импорта')).not.toBeInTheDocument()
  expect(
    within(screen.getByRole('region', { name: 'Машинное обучение' })).getByRole(
      'status',
    ),
  ).toHaveTextContent('Карта компетенций успешно загружена')
})

it('cancels replacement without sending an import request', async () => {
  const fetch = backend()
  renderApp()
  fireEvent.change(await screen.findByLabelText('Файл карты компетенций'), {
    target: { files: [new File(['map'], 'map.csv')] },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Подтвердить изменения' }))
  fireEvent.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: 'Отмена',
    }),
  )
  expect(fetch.mock.calls.some(([url]) => url.endsWith('/import'))).toBe(false)
  expect(
    screen.getByRole('button', { name: 'Подтвердить изменения' }),
  ).toBeEnabled()
})

it('discards a chosen replacement file with the cancel button', async () => {
  const fetch = backend()
  renderApp()
  expect(
    await screen.findByRole('heading', { name: 'Машинное обучение' }),
  ).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Отменить' }),
  ).not.toBeInTheDocument()
  fireEvent.change(screen.getByLabelText('Файл карты компетенций'), {
    target: { files: [new File(['map'], 'map.csv')] },
  })
  expect(screen.getByText('map.csv')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Отменить' }))
  expect(screen.queryByText('map.csv')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Подтвердить изменения' }),
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Отменить' }),
  ).not.toBeInTheDocument()
  expect(fetch.mock.calls.some(([url]) => url.endsWith('/import'))).toBe(false)
})

it('rejects unsupported and oversized files before sending them', async () => {
  const fetch = backend()
  renderApp()
  const input = await screen.findByLabelText('Файл карты компетенций')
  fireEvent.change(input, { target: { files: [new File(['map'], 'map.xls')] } })
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Выберите файл CSV или XLSX',
  )
  expect(
    screen.queryByRole('button', { name: 'Подтвердить изменения' }),
  ).not.toBeInTheDocument()
  const large = new File(['map'], 'map.csv')
  Object.defineProperty(large, 'size', { value: 25 * 1024 * 1024 + 1 })
  fireEvent.change(input, { target: { files: [large] } })
  expect(await screen.findByRole('alert')).toHaveTextContent('25 МиБ')
  expect(fetch.mock.calls.some(([url]) => url.endsWith('/import'))).toBe(false)
})

it('shows parser details and drops the rejected file without confirm actions', async () => {
  backend({
    status: 422,
    result: {
      code: 'CSV_INVALID',
      message: 'Некорректная карта.',
      details: [
        {
          path: 'row:7',
          code: 'CSV_INVALID',
          message: 'Важность: ожидается число.',
        },
      ],
    },
  })
  renderApp()
  fireEvent.change(await screen.findByLabelText('Файл карты компетенций'), {
    target: { files: [new File(['map'], 'map.csv')] },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Подтвердить изменения' }))
  fireEvent.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: 'Перезаписать',
    }),
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Строка 7: Важность: ожидается число.',
  )
  expect(
    screen.queryByRole('button', { name: 'Подтвердить изменения' }),
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Отменить' }),
  ).not.toBeInTheDocument()
  // A new file clears the rejection and brings the actions back.
  fireEvent.change(screen.getByLabelText('Файл карты компетенций'), {
    target: { files: [new File(['fixed'], 'fixed.csv')] },
  })
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: 'Подтвердить изменения' }),
  ).toBeEnabled()
})

it('keeps the file for a retry after a technical import failure', async () => {
  backend({
    status: 503,
    result: { code: 'UNAVAILABLE', message: 'Сервер временно недоступен.' },
  })
  renderApp()
  fireEvent.change(await screen.findByLabelText('Файл карты компетенций'), {
    target: { files: [new File(['map'], 'map.csv')] },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Подтвердить изменения' }))
  fireEvent.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: 'Перезаписать',
    }),
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Сервер временно недоступен.',
  )
  expect(screen.getByText('map.csv')).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'Подтвердить изменения' }),
  ).toBeEnabled()
})

it('shows the shared account without a logout in the admin panel', async () => {
  backend()
  renderApp()
  await screen.findByRole('heading', { name: 'Карта компетенций' })
  expect(screen.getByLabelText('Профиль: Администратор')).toBeVisible()
  expect(
    screen.queryByRole('button', { name: /Меню профиля/ }),
  ).not.toBeInTheDocument()
  expect(screen.queryByText('Выйти')).not.toBeInTheDocument()
})

it('does not expose the upload form when the admin panel cannot sign in', async () => {
  const fetch = backend({ user: null, teacherFails: true })
  renderApp('/admin/competency-map')
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Не удалось открыть админ-панель.',
  )
  expect(
    screen.queryByLabelText('Файл карты компетенций'),
  ).not.toBeInTheDocument()
  expect(
    fetch.mock.calls.every(([url]) =>
      /\/auth\/(me|refresh|teacher)$/.test(url),
    ),
  ).toBe(true)
})

it('restores the admin screen after reload with an expired access cookie', async () => {
  let valid = false
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.endsWith('/auth/refresh')) {
        valid = true
        return Response.json({})
      }
      if (url.endsWith('/auth/me'))
        return valid
          ? Response.json(admin)
          : new Response(null, { status: 401 })
      if (url.endsWith('/subjects/subject-a/competency-map'))
        return Response.json(emptyMap)
      if (url.endsWith('/materials')) return Response.json([])
      throw new Error(`Unexpected request: ${url}`)
    }),
  )
  renderApp('/admin/competency-map')
  expect(
    await screen.findByRole('heading', { name: 'Карта компетенций' }),
  ).toBeVisible()
  expect(screen.queryByText('Открываем админ-панель…')).not.toBeInTheDocument()
})

it('blocks a second upload and changing the file while import is pending', async () => {
  const fetch = backend()
  let finish!: (response: Response) => void
  renderApp()
  const input = await screen.findByLabelText('Файл карты компетенций')
  fetch.mockImplementation(async (url: string) => {
    if (url.endsWith('/subjects')) return Response.json(subjects)
    if (url.endsWith('/materials')) return Response.json([])
    return url.endsWith('/import')
      ? new Promise<Response>((resolve) => {
          finish = resolve
        })
      : new Response(JSON.stringify(emptyMap))
  })
  fireEvent.change(input, { target: { files: [new File(['map'], 'map.csv')] } })
  fireEvent.click(screen.getByRole('button', { name: 'Подтвердить изменения' }))
  fireEvent.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: 'Перезаписать',
    }),
  )
  expect(await screen.findByText('Загружаем и проверяем карту…')).toBeVisible()
  expect(input).toBeDisabled()
  expect(
    screen.getByRole('button', { name: 'Подтвердить изменения' }),
  ).toBeDisabled()
  await act(async () => finish(new Response(JSON.stringify(imported))))
  expect(await screen.findByText('Предупреждения импорта')).toBeVisible()
})

it('enters the shared account again when an import meets an expired session', async () => {
  backend({
    status: 401,
    result: { code: 'UNAUTHORIZED', message: 'Сессия завершилась.' },
  })
  renderApp()
  fireEvent.change(await screen.findByLabelText('Файл карты компетенций'), {
    target: { files: [new File(['map'], 'map.csv')] },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Подтвердить изменения' }))
  fireEvent.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: 'Перезаписать',
    }),
  )
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Подтвердить изменения' }),
    ).toBeVisible(),
  )
  expect(
    await screen.findByLabelText('Файл карты компетенций'),
  ).toBeInTheDocument()
})

it('does not claim success for an unreadable import result', async () => {
  backend({ result: { revision: 2 } })
  renderApp()
  fireEvent.change(await screen.findByLabelText('Файл карты компетенций'), {
    target: { files: [new File(['map'], 'map.csv')] },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Подтвердить изменения' }))
  fireEvent.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: 'Перезаписать',
    }),
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Проверьте текущую карту перед повторной загрузкой',
  )
  expect(screen.queryByText('Предупреждения импорта')).not.toBeInTheDocument()
  await waitFor(() =>
    expect(
      screen.getByRole('region', { name: 'Машинное обучение' }),
    ).toHaveTextContent(/Версия карты\s*2/),
  )
})

it('retries a failed session check without exposing the lesson', async () => {
  const fetch = backend()
  fetch.mockRejectedValueOnce(new Error('offline'))
  renderApp()
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Не удалось связаться с сервером',
  )
  expect(screen.queryByText('Машинное обучение')).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Попробовать снова' }))
  expect(
    await screen.findByRole('heading', { name: 'Карта компетенций' }),
  ).toBeVisible()
})

it('enters again after an expired map refresh without reusing the cached 401', async () => {
  const fetch = backend()
  const router = renderApp()
  await screen.findByText('Карта ещё не загружена.')
  let signedIn = true
  let expired = false
  let finish!: (response: Response) => void
  fetch.mockImplementation(async (url: string) => {
    if (url.endsWith('/auth/refresh'))
      return new Response(null, { status: 401 })
    if (url.endsWith('/auth/me'))
      return signedIn
        ? Response.json(admin)
        : new Response(null, { status: 401 })
    if (url.endsWith('/auth/teacher')) {
      signedIn = true
      return Response.json({ user: admin })
    }
    if (url.endsWith('/subjects')) return Response.json(subjects)
    if (url.endsWith('/materials')) return Response.json([])
    // The first map read finds the session expired; after entering again
    // the map is read afresh.
    if (!expired) {
      expired = true
      signedIn = false
      return Response.json({ message: 'Сессия завершилась.' }, { status: 401 })
    }
    return new Promise<Response>((resolve) => {
      finish = resolve
    })
  })
  await act(async () => {
    await router.options.context!.queryClient.invalidateQueries({
      queryKey: ['competency-map'],
    })
  })
  await waitFor(() =>
    expect(
      fetch.mock.calls.some(([url]) => url.endsWith('/auth/teacher')),
    ).toBe(true),
  )
  await waitFor(() => expect(finish).toBeDefined())
  await act(async () => finish(Response.json(emptyMap)))
  expect(
    await screen.findByRole('heading', { name: 'Карта компетенций' }),
  ).toBeVisible()
  expect(await screen.findByText('Карта ещё не загружена.')).toBeVisible()
})

// Course Materials Tests

it('uploads lecture files and shows success message', async () => {
  const fetch = backend()
  renderApp('/admin/competency-map')
  const input = await screen.findByLabelText('Загрузить файлы материалов')
  fireEvent.change(input, {
    target: {
      files: [
        new File(['pdfcontent'], 'new_lecture.pdf', {
          type: 'application/pdf',
        }),
      ],
    },
  })
  expect(await screen.findByText(/new_lecture\.pdf/)).toBeVisible()
  const submitBtn = screen.getByRole('button', {
    name: /Загрузить и распознать материалы/,
  })
  fireEvent.click(submitBtn)
  expect(
    await screen.findByText(
      'Материалы успешно загружены, распознаны и сохранены в S3.',
    ),
  ).toBeVisible()
  expect(
    fetch.mock.calls.some(
      ([url, req]) => url.endsWith('/materials') && req?.method === 'POST',
    ),
  ).toBe(true)
})

it('toggles markdown text preview for an uploaded material', async () => {
  backend()
  renderApp('/admin/competency-map')
  const previewBtn = await screen.findByRole('button', {
    name: 'Предпросмотр распознанного текста',
  })
  expect(screen.queryByText(/Формула: \$y = Wx \+ b\$/)).not.toBeInTheDocument()
  fireEvent.click(previewBtn)
  expect(await screen.findByText(/Формула: \$y = Wx \+ b\$/)).toBeVisible()
})

it('downloads raw material file using downloadCourseMaterialRaw', async () => {
  const fetch = backend()
  renderApp('/admin/competency-map')
  const downloadBtn = await screen.findByRole('button', {
    name: 'Скачать lecture1.pdf',
  })
  fireEvent.click(downloadBtn)
  await waitFor(() => {
    expect(
      fetch.mock.calls.some(([url]) => url.endsWith('/materials/mat-1/raw')),
    ).toBe(true)
  })
})

it('deletes an uploaded material after confirmation', async () => {
  const fetch = backend()
  renderApp('/admin/competency-map')
  const deleteBtn = await screen.findByRole('button', {
    name: 'Удалить',
  })
  fireEvent.click(deleteBtn)
  const dialog = await screen.findByRole('alertdialog')
  expect(dialog).toHaveTextContent(
    'Удалить этот файл и его распознанный текст?',
  )
  fireEvent.click(
    within(dialog).getByRole('button', {
      name: 'Перезаписать',
    }),
  )
  await waitFor(() => {
    expect(
      fetch.mock.calls.some(
        ([url, req]) =>
          url.endsWith('/materials/mat-1') && req?.method === 'DELETE',
      ),
    ).toBe(true)
  })
})

it('shows warning alert when some files fail during upload', async () => {
  backend({ partialUpload: true })
  renderApp('/admin/competency-map')
  const input = await screen.findByLabelText('Загрузить файлы материалов')
  fireEvent.change(input, {
    target: {
      files: [
        new File(['bad'], 'corrupted.pdf', {
          type: 'application/pdf',
        }),
      ],
    },
  })
  expect(await screen.findByText(/corrupted\.pdf/)).toBeVisible()
  const submitBtn = screen.getByRole('button', {
    name: /Загрузить и распознать материалы/,
  })
  fireEvent.click(submitBtn)
  expect(
    await screen.findByText(
      /Часть файлов не удалось загрузить: corrupted\.pdf: Ошибка распознавания/,
    ),
  ).toBeVisible()
})

it('shows download error alert when downloading raw material fails', async () => {
  backend({ downloadFails: true })
  renderApp('/admin/competency-map')
  const downloadBtn = await screen.findByRole('button', {
    name: 'Скачать lecture1.pdf',
  })
  fireEvent.click(downloadBtn)
  expect(await screen.findByRole('alert')).toBeVisible()
})
