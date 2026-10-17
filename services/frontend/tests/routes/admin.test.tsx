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
    error_message: null,
    created_at: 1791356400,
    updated_at: 1791356400,
  },
]

// The admin panel is the /admin site; the browser address decides the portal.
function renderApp(path = '/admin/competency-map/subject-a') {
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
    if (url.endsWith('/graph'))
      return Response.json({
        status: 'not_requested',
        attempts: 0,
        error_message: '',
        graph: null,
        stale: false,
        inputs_ready: true,
        updated_at: 0,
      })
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

it.each(['/admin', '/admin/'])('opens the course list at %s', async (path) => {
  backend()
  const router = renderApp(path)
  expect(
    await screen.findByRole('heading', { name: 'Курсы', level: 1 }),
  ).toBeVisible()
  expect(router.state.location.pathname).toBe('/admin/courses')
  expect(
    await screen.findByRole('heading', { name: 'Машинное обучение', level: 2 }),
  ).toBeVisible()
  expect(
    screen.getByRole('heading', { name: 'Статистика', level: 2 }),
  ).toBeVisible()
  fireEvent.click(screen.getByRole('link', { name: /Машинное обучение/ }))
  expect(await screen.findByText('lecture1.pdf')).toBeVisible()
  expect(router.state.location.pathname).toBe('/admin/competency-map/subject-a')
})

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

it('creates a trimmed course from the list', async () => {
  const fetch = backend({ subjects: [], materials: [] })
  renderApp('/admin/courses')
  fireEvent.click(await screen.findByRole('button', { name: 'Новый курс' }))
  const field = screen.getByLabelText('Название курса')
  const submit = screen.getByRole('button', { name: 'Создать курс' })
  expect(submit).toBeDisabled()
  fireEvent.change(field, { target: { value: '  Новый  ' } })
  fireEvent.click(submit)
  expect(
    await screen.findByRole('heading', { name: 'Новый', level: 2 }),
  ).toBeVisible()
  const [, init] = fetch.mock.calls.find(
    ([url, request]) =>
      url.endsWith('/admin/subjects') && request?.method === 'POST',
  )!
  expect(JSON.parse(init?.body as string)).toEqual({ name: 'Новый' })
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
  renderApp('/admin/competency-map/subject-a')
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Доступно только администраторам',
  )
  expect(
    screen.queryByLabelText('Добавить PDF или ZIP'),
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
  renderApp('/admin/competency-map/subject-a')
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Не удалось открыть админ-панель.',
  )
  expect(
    screen.queryByLabelText('Добавить PDF или ZIP'),
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
      if (url.endsWith('/graph'))
        return Response.json({
          status: 'not_requested',
          attempts: 0,
          error_message: '',
          graph: null,
          stale: false,
          inputs_ready: true,
          updated_at: 0,
        })
      throw new Error(`Unexpected request: ${url}`)
    }),
  )
  renderApp('/admin/competency-map/subject-a')
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
  renderApp('/admin/competency-map/subject-a')
  const input = await screen.findByLabelText('Добавить PDF или ZIP')
  fireEvent.change(input, {
    target: {
      files: [
        new File(['%PDF-1.4 content'], 'new_lecture.pdf', {
          type: 'application/pdf',
        }),
      ],
    },
  })
  expect(await screen.findByText(/new_lecture\.pdf/)).toBeVisible()
  const submitBtn = screen.getByRole('button', {
    name: /Отправить всё/,
  })
  fireEvent.click(submitBtn)
  expect(
    await screen.findByText(
      'Материалы загружены. Обработка продолжится в фоне.',
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
  renderApp('/admin/competency-map/subject-a')
  const previewBtn = await screen.findByRole('button', {
    name: 'Предпросмотр распознанного текста',
  })
  expect(screen.queryByText(/Формула: \$y = Wx \+ b\$/)).not.toBeInTheDocument()
  fireEvent.click(previewBtn)
  expect(await screen.findByText(/Формула: \$y = Wx \+ b\$/)).toBeVisible()
})

it('downloads raw material file using downloadCourseMaterialRaw', async () => {
  const fetch = backend()
  renderApp('/admin/competency-map/subject-a')
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
  renderApp('/admin/competency-map/subject-a')
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
  renderApp('/admin/competency-map/subject-a')
  const input = await screen.findByLabelText('Добавить PDF или ZIP')
  fireEvent.change(input, {
    target: {
      files: [
        new File(['%PDF-1.4 bad'], 'corrupted.pdf', {
          type: 'application/pdf',
        }),
      ],
    },
  })
  expect(await screen.findByText(/corrupted\.pdf/)).toBeVisible()
  const submitBtn = screen.getByRole('button', {
    name: /Отправить всё/,
  })
  fireEvent.click(submitBtn)
  expect(
    await screen.findByText(/corrupted\.pdf: Ошибка распознавания/),
  ).toBeVisible()
})

it('shows download error alert when downloading raw material fails', async () => {
  backend({ downloadFails: true })
  renderApp('/admin/competency-map/subject-a')
  const downloadBtn = await screen.findByRole('button', {
    name: 'Скачать lecture1.pdf',
  })
  fireEvent.click(downloadBtn)
  expect(await screen.findByRole('alert')).toBeVisible()
})
