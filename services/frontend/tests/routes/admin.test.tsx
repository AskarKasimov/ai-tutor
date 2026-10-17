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
    week: 1 as number | null,
    kind: 'lecture' as 'lecture' | 'lab' | 'tutorial' | null,
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
    subjects?: typeof subjects
    teacherFails?: boolean
    materials?: typeof sampleMaterials
    partialUpload?: boolean
    downloadFails?: boolean
    graph?: {
      version: string
      nodes: Array<{
        id: string
        title: string
        content: string
        prerequisites: string[]
        outcomes: string[]
        questions: string[]
      }>
    }
    build?: {
      id: string
      subject_id: string
      status: string
      stage: string | null
      started_at: number | null
      completed_at: number | null
      error: { code: string; message: string } | null
    }
  } = {},
) {
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
    if (url.endsWith('/admin/material-kinds'))
      return Response.json([
        { code: 'lecture', label: 'Лекция' },
        { code: 'lab', label: 'Лабораторная' },
        { code: 'tutorial', label: 'Практика' },
      ])
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
            results: [
              {
                client_id: JSON.parse(
                  (init.body as FormData).get('items') as string,
                )[0].client_id,
                error: {
                  filename: 'corrupted.pdf',
                  code: 'OCR_FAILED',
                  message: 'Ошибка распознавания',
                },
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
        week: 1,
        kind: 'lecture' as const,
        error_message: null,
        created_at: 1791356400,
        updated_at: 1791356400,
      }
      activeMaterials.push(newMat)
      const clientId = JSON.parse(
        (init.body as FormData).get('items') as string,
      )[0].client_id
      return new Response(
        JSON.stringify({
          uploaded: [newMat],
          failed: [],
          results: [{ client_id: clientId, material: newMat }],
        }),
      )
    }
    if (
      url.includes('/admin/subjects/') &&
      url.endsWith('/graph/build') &&
      init?.method === 'POST'
    )
      return Response.json(
        options.build ?? {
          id: 'build-1',
          subject_id: 'subject-a',
          status: 'queued',
          stage: null,
          started_at: null,
          completed_at: null,
          error: null,
        },
        { status: 202 },
      )
    if (url.includes('/admin/subjects/') && url.endsWith('/graph/build'))
      return options.build
        ? Response.json(options.build)
        : Response.json(
            { code: 'NOT_FOUND', message: 'Not found' },
            { status: 404 },
          )
    if (url.includes('/admin/subjects/') && url.endsWith('/graph'))
      return options.graph
        ? Response.json(options.graph)
        : Response.json(
            { code: 'NOT_FOUND', message: 'Not found' },
            { status: 404 },
          )
    if (
      url.includes('/admin/subjects/') &&
      url.includes('/materials/') &&
      init?.method === 'PATCH'
    ) {
      const material =
        activeMaterials.find((item) => url.includes(item.id)) ??
        activeMaterials[0]
      Object.assign(material, JSON.parse(init.body as string))
      return Response.json(material)
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
  'opens the materials administration by default when the admin panel opens at %s',
  async (path) => {
    backend()
    const router = renderApp(path)
    expect(
      await screen.findByRole('heading', {
        name: 'Подготовка курса',
        level: 1,
      }),
    ).toBeVisible()
    expect(router.state.location.pathname).toBe('/admin/competency-map')
    expect(screen.queryByText('Задания сессии')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Начать запись' }),
    ).not.toBeInTheDocument()
    expect(
      await screen.findByRole('heading', {
        name: 'Машинное обучение',
        level: 2,
      }),
    ).toBeVisible()
    const courseSelect = screen.getByRole('combobox', { name: 'Курс' })
    expect(courseSelect).toBeVisible()
    fireEvent.click(courseSelect)
    expect(
      await screen.findByRole('option', { name: 'Статистика' }),
    ).toBeVisible()
    fireEvent.keyDown(courseSelect, { key: 'Escape' })
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

it('creates a trimmed subject, then switches to its materials', async () => {
  const fetch = backend({ subjects: [], materials: [] })
  renderApp('/admin/competency-map')
  const field = await screen.findByLabelText('Название курса')
  const submit = screen.getByRole('button', { name: 'Подтвердить' })
  expect(submit).toBeDisabled()
  fireEvent.change(field, { target: { value: '  Новый  ' } })
  fireEvent.click(submit)
  expect(
    await screen.findByRole('heading', { name: 'Новый', level: 2 }),
  ).toBeVisible()
  expect(
    await screen.findByText('Материалы курса ещё не загружены.'),
  ).toBeVisible()
  const [, init] = fetch.mock.calls.find(
    ([url, request]) =>
      url.endsWith('/admin/subjects') && request?.method === 'POST',
  )!
  expect(JSON.parse(init?.body as string)).toEqual({ name: 'Новый' })
})

it('creates an additional course without replacing the existing course', async () => {
  const fetch = backend()
  renderApp('/admin/competency-map')
  fireEvent.click(await screen.findByRole('button', { name: 'Создать курс' }))
  const dialog = await screen.findByRole('dialog')
  fireEvent.change(within(dialog).getByLabelText('Название курса'), {
    target: { value: '  Новый курс  ' },
  })
  fireEvent.click(within(dialog).getByRole('button', { name: 'Создать' }))
  expect(
    await screen.findByRole('heading', { name: 'Новый', level: 2 }),
  ).toBeVisible()
  const [, request] = fetch.mock.calls.find(
    ([url, init]) => url.endsWith('/admin/subjects') && init?.method === 'POST',
  )!
  expect(JSON.parse(request?.body as string)).toEqual({ name: 'Новый курс' })
})

it('keeps the student site free of the admin panel even for an admin account', async () => {
  backend()
  const router = renderApp('/')
  expect(await screen.findByRole('heading', { name: 'Курсы' })).toBeVisible()
  expect(router.state.location.pathname).toBe('/')
  expect(
    screen.queryByRole('heading', { name: 'Подготовка курса' }),
  ).not.toBeInTheDocument()
  expect(document.querySelector('a[href^="/admin"]')).toBeNull()
})

it('denies students direct access to the admin screen without fetching materials', async () => {
  const fetch = backend({ user: { ...admin, role: 'student' } })
  renderApp('/admin/competency-map')
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Доступно только администраторам',
  )
  expect(
    screen.queryByLabelText('Загрузить файлы материалов'),
  ).not.toBeInTheDocument()
  expect(fetch.mock.calls.every(([url]) => url.endsWith('/auth/me'))).toBe(true)
})

it('shows the shared account without a logout in the admin panel', async () => {
  backend()
  renderApp()
  await screen.findByRole('heading', { name: 'Подготовка курса' })
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
    screen.queryByLabelText('Загрузить файлы материалов'),
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
      if (url.endsWith('/subjects')) return Response.json(subjects)
      if (url.endsWith('/materials')) return Response.json([])
      throw new Error(`Unexpected request: ${url}`)
    }),
  )
  renderApp('/admin/competency-map')
  expect(
    await screen.findByRole('heading', { name: 'Подготовка курса' }),
  ).toBeVisible()
  expect(screen.queryByText('Открываем админ-панель…')).not.toBeInTheDocument()
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
    await screen.findByRole('heading', { name: 'Подготовка курса' }),
  ).toBeVisible()
})

it('uploads lecture files and shows success message', async () => {
  const fetch = backend()
  renderApp('/admin/competency-map')
  const input = await screen.findByLabelText('Добавить PDF или ZIP')
  fireEvent.change(input, {
    target: {
      files: [
        new File(['%PDF-1.7 pdfcontent'], 'new_lecture.pdf', {
          type: 'application/pdf',
        }),
      ],
    },
  })
  expect(await screen.findByText(/new_lecture\.pdf/)).toBeVisible()
  const submitBtn = screen.getByRole('button', { name: 'Отправить всё' })
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
  const [, request] = fetch.mock.calls.find(
    ([url, init]) => url.endsWith('/materials') && init?.method === 'POST',
  )!
  expect(request?.body).toBeInstanceOf(FormData)
  expect((request?.body as FormData).get('items')).toBeTruthy()
})

it('blocks graph generation for legacy materials until their metadata is fixed', async () => {
  backend({ materials: [{ ...sampleMaterials[0], week: null, kind: null }] })
  renderApp('/admin/competency-map')
  expect(
    await screen.findByText('У 1 материалов не указаны неделя или тип.'),
  ).toBeVisible()
  expect(
    screen.getByRole('button', { name: /Собрать граф тем/ }),
  ).toBeDisabled()
})

it('persists corrected metadata without uploading the source again', async () => {
  const fetch = backend({
    materials: [{ ...sampleMaterials[0], week: null, kind: null }],
  })
  renderApp('/admin/competency-map')
  const typeSelect = await screen.findByRole('combobox', {
    name: 'Тип материала: lecture1.pdf',
  })
  fireEvent.click(typeSelect)
  fireEvent.click(await screen.findByRole('option', { name: 'Лабораторная' }))
  await waitFor(() =>
    expect(
      fetch.mock.calls.some(
        ([url, request]) =>
          url.endsWith('/materials/mat-1') && request?.method === 'PATCH',
      ),
    ).toBe(true),
  )
  expect(
    fetch.mock.calls.some(
      ([url, request]) =>
        url.endsWith('/materials') && request?.method === 'POST',
    ),
  ).toBe(false)
})

it('shows the durable active build stage recovered from the server', async () => {
  backend({
    build: {
      id: 'build-1',
      subject_id: 'subject-a',
      status: 'running',
      stage: 'dedup',
      started_at: 1791356400,
      completed_at: null,
      error: null,
    },
  })
  renderApp('/admin/competency-map')
  expect(await screen.findByText('Объединение повторов')).toBeVisible()
  expect(screen.getByText('Собираем граф тем')).toBeVisible()
  expect(
    screen.getByRole('button', { name: /Собрать граф тем/ }),
  ).toBeDisabled()
})

it('requires confirmation before replacing an existing graph', async () => {
  const fetch = backend({
    graph: {
      version: 'v1',
      nodes: [
        {
          id: 'topic-1',
          title: 'Матрицы',
          content: 'Основы матриц',
          prerequisites: [],
          outcomes: [],
          questions: [],
        },
      ],
    },
  })
  renderApp('/admin/competency-map')
  expect(
    await screen.findByText('Курс готов для студентов. В графе тем: 1.'),
  ).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Пересобрать граф' }))
  const dialog = await screen.findByRole('alertdialog')
  expect(dialog).toHaveTextContent(
    'Активные диагностики по курсу будут отменены.',
  )
  expect(
    fetch.mock.calls.some(
      ([url, request]) =>
        url.endsWith('/graph/build') && request?.method === 'POST',
    ),
  ).toBe(false)
  fireEvent.click(within(dialog).getByRole('button', { name: 'Начать сборку' }))
  await waitFor(() =>
    expect(
      fetch.mock.calls.some(
        ([url, request]) =>
          url.endsWith('/graph/build') && request?.method === 'POST',
      ),
    ).toBe(true),
  )
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
      name: 'Подтвердить',
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
  const input = await screen.findByLabelText('Добавить PDF или ZIP')
  fireEvent.change(input, {
    target: {
      files: [
        new File(['%PDF-1.7 bad'], 'corrupted.pdf', {
          type: 'application/pdf',
        }),
      ],
    },
  })
  expect(await screen.findByText(/corrupted\.pdf/)).toBeVisible()
  const submitBtn = screen.getByRole('button', { name: 'Отправить всё' })
  fireEvent.click(submitBtn)
  expect(await screen.findByText(/Ошибка распознавания/)).toBeVisible()
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

it('expands a PDF-only ZIP into independently editable queue rows', async () => {
  const { zipSync, strToU8 } = await import('fflate')
  backend()
  renderApp('/admin/competency-map')
  const input = await screen.findByLabelText('Добавить PDF или ZIP')
  const archive = new File(
    [
      zipSync({
        'week-1.pdf': strToU8('%PDF-1.7 one'),
        'week-2.pdf': strToU8('%PDF-1.7 two'),
      }),
    ],
    'materials.zip',
  )
  fireEvent.change(input, { target: { files: [archive] } })
  expect(await screen.findByText('week-1.pdf')).toBeVisible()
  expect(await screen.findByText('week-2.pdf')).toBeVisible()
  expect(screen.getAllByRole('spinbutton', { name: 'Неделя' })).toHaveLength(2)
})

it('rejects a mixed ZIP without adding any PDFs to the queue', async () => {
  const { zipSync, strToU8 } = await import('fflate')
  backend()
  renderApp('/admin/competency-map')
  const input = await screen.findByLabelText('Добавить PDF или ZIP')
  const archive = new File(
    [
      zipSync({
        'week.pdf': strToU8('%PDF-1.7'),
        'notes.txt': strToU8('not a PDF'),
      }),
    ],
    'materials.zip',
  )
  fireEvent.change(input, { target: { files: [archive] } })
  expect(await screen.findByRole('alert')).toHaveTextContent('notes.txt')
  expect(screen.queryByText('week.pdf')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Отправить всё' }),
  ).not.toBeInTheDocument()
})
