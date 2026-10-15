import { fireEvent, render, screen } from '@testing-library/react'
import {
  createMemoryHistory,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { afterEach, beforeAll, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import { routeTree } from '@/routeTree.gen'

beforeAll(async () => {
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

const testMaterials = [
  {
    id: 'mat-1',
    subject_id: 'subject-a',
    filename: 'lecture1.pdf',
    content_type: 'application/pdf',
    file_size: 1024 * 1024,
    s3_raw_key: 'subjects/subject-a/raw/mat-1_lecture1.pdf',
    s3_parsed_key: 'subjects/subject-a/parsed/mat-1_lecture1.pdf.md',
    parsed_text: '## Лекция 1\n\nФормула: $y = Wx + b$',
    char_count: 30,
    status: 'parsed' as const,
    created_at: 1791356400,
    updated_at: 1791356400,
  },
]

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
    materials?: typeof testMaterials
    teacherFails?: boolean
  } = {},
) {
  const activeSubjects = [...(options.subjects ?? subjects)]
  let materialsList = [...(options.materials ?? testMaterials)]
  let signedIn = options.user !== null

  const fetch = vi.fn(async (url: string, init?: RequestInit) => {
    if (url.endsWith('/auth/refresh'))
      return new Response(null, { status: 401 })
    if (url.endsWith('/auth/me'))
      return signedIn
        ? new Response(JSON.stringify(options.user ?? admin))
        : new Response(null, { status: 401 })
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
      return new Response(JSON.stringify(materialsList))
    }
    if (
      url.includes('/admin/subjects/') &&
      url.endsWith('/materials') &&
      init?.method === 'POST'
    ) {
      const added = {
        id: 'mat-new',
        subject_id: 'subject-a',
        filename: 'new_lecture.pdf',
        content_type: 'application/pdf',
        file_size: 500000,
        s3_raw_key: 'subjects/subject-a/raw/mat-new_new_lecture.pdf',
        s3_parsed_key: 'subjects/subject-a/parsed/mat-new_new_lecture.pdf.md',
        parsed_text: '# Новая лекция',
        char_count: 14,
        status: 'parsed' as const,
        created_at: 1791356400,
        updated_at: 1791356400,
      }
      materialsList = [added, ...materialsList]
      return new Response(JSON.stringify([added]))
    }
    if (
      url.includes('/admin/subjects/') &&
      url.includes('/materials/') &&
      init?.method === 'DELETE'
    ) {
      materialsList = []
      return new Response(JSON.stringify({ status: 'deleted' }))
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
        name: 'Карта компетенций',
        level: 1,
      }),
    ).toBeVisible()
    expect(router.state.location.pathname).toBe('/admin/competency-map')
    expect(
      await screen.findByRole('heading', {
        name: 'Загруженные лекции и лабораторные',
        level: 2,
      }),
    ).toBeVisible()
    expect(
      screen.getByRole('heading', { name: 'Машинное обучение', level: 2 }),
    ).toBeVisible()
    expect(await screen.findByText('lecture1.pdf')).toBeVisible()
    expect(screen.getByText('Распознан')).toBeVisible()
  },
)

it('creates a subject and switches to materials view', async () => {
  const fetch = backend({ subjects: [] })
  renderApp('/admin/competency-map')
  const field = await screen.findByLabelText('Название предмета')
  const submit = screen.getByRole('button', { name: 'Подтвердить' })
  fireEvent.change(field, { target: { value: '  Новый  ' } })
  expect(submit).not.toBeDisabled()
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

it('denies students direct access to admin materials screen', async () => {
  const fetch = backend({ user: { ...admin, role: 'student' } })
  renderApp('/admin/competency-map')
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Доступно только администраторам',
  )
  expect(fetch.mock.calls.every(([url]) => url.endsWith('/auth/me'))).toBe(true)
})

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
