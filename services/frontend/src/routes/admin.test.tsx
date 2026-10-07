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
import { createQueryClient } from '../app/providers'
import { routeTree } from '../routeTree.gen'
import * as trainerSource from '../mocks/trainer-session-source'

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
  unparsed_task_cell_count: 1,
  warnings: [
    {
      row: 8,
      column_index: 12,
      column: 'Задание 2',
      code: 'TASK_CELL_FRAGMENT',
    },
  ],
}

function renderApp(path = '/') {
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
  } = {},
) {
  let active: typeof emptyMap | typeof importedMap = emptyMap
  const fetch = vi.fn(async (url: string, init?: RequestInit) => {
    if (url.endsWith('/auth/refresh'))
      return new Response(null, { status: 401 })
    if (url.endsWith('/auth/me'))
      return new Response(
        JSON.stringify(options.user === undefined ? admin : options.user),
        { status: options.user === null ? 401 : 200 },
      )
    if (url.endsWith('/competency-map') && !init?.method)
      return new Response(JSON.stringify(active))
    if (url.endsWith('/admin/competency-map/import')) {
      if (!options.status || options.status === 200) active = importedMap
      return new Response(JSON.stringify(options.result ?? imported), {
        status: options.status ?? 200,
      })
    }
    throw new Error(`Unexpected request: ${url}`)
  })
  vi.stubGlobal('fetch', fetch)
  return fetch
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

it.each(['/', '/login'])(
  'opens the map administration by default for a restored admin session at %s',
  async (path) => {
    const createLesson = vi.spyOn(
      trainerSource,
      'createMockTrainerSessionSource',
    )
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
    expect(createLesson).not.toHaveBeenCalled()
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
  expect(
    await screen.findByRole('heading', { name: 'Вход в AI Tutor', level: 1 }),
  ).toBeVisible()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})

it('routes an admin straight from the login form to the import screen', async () => {
  let authenticated = false
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.endsWith('/auth/refresh'))
        return new Response(null, { status: 401 })
      if (url.endsWith('/auth/me'))
        return new Response(JSON.stringify(admin), {
          status: authenticated ? 200 : 401,
        })
      if (url.endsWith('/auth/login')) {
        authenticated = true
        return new Response(JSON.stringify({ user: admin }))
      }
      return new Response(JSON.stringify(emptyMap))
    }),
  )
  renderApp()
  await screen.findByRole('heading', { name: 'Вход в AI Tutor' })
  expect(screen.queryByText('Машинное обучение')).not.toBeInTheDocument()
  fireEvent.change(screen.getByLabelText('Email'), {
    target: { value: 'admin@example.com' },
  })
  fireEvent.change(screen.getByLabelText('Пароль'), {
    target: { value: 'admin-password' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Войти' }))
  expect(
    await screen.findByRole('heading', { name: 'Карта компетенций' }),
  ).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Начать запись' }),
  ).not.toBeInTheDocument()
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
    fireEvent.click(screen.getByRole('button', { name: 'Загрузить карту' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(dialog).toHaveTextContent('текущую карту и банк заданий')
    expect(fetch.mock.calls.some(([url]) => url.endsWith('/import'))).toBe(
      false,
    )
    fireEvent.click(
      within(dialog).getByRole('button', { name: 'Заменить и загрузить' }),
    )
    expect(await screen.findByRole('status')).toHaveTextContent(
      'Карта загружена',
    )
    expect(screen.getByText('Задание 2')).toBeVisible()
    expect(screen.getByText('8')).toBeVisible()
    expect(
      screen.getByText(
        'Неполное задание: сохранено в источнике, но не включено в банк.',
      ),
    ).toBeVisible()
    await waitFor(() =>
      expect(
        screen.getByRole('region', { name: 'Текущая карта' }),
      ).toHaveTextContent('Версия 2'),
    )
    const [, init] = fetch.mock.calls.find(([url]) => url.endsWith('/import'))!
    expect(init?.credentials).toBe('include')
    expect(init?.headers).toBeUndefined()
    const file = (init?.body as FormData).get('file') as File
    expect(file.name).toBe(`map.${extension}`)
    expect(file.type).toBe(
      extension === 'csv'
        ? 'text/csv'
        : 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    )
  },
)

it('cancels replacement without sending an import request', async () => {
  const fetch = backend()
  renderApp()
  fireEvent.change(await screen.findByLabelText('Файл карты компетенций'), {
    target: { files: [new File(['map'], 'map.csv')] },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Загрузить карту' }))
  fireEvent.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: 'Отмена',
    }),
  )
  expect(fetch.mock.calls.some(([url]) => url.endsWith('/import'))).toBe(false)
  expect(screen.getByRole('button', { name: 'Загрузить карту' })).toBeEnabled()
})

it('rejects unsupported and oversized files before sending them', async () => {
  const fetch = backend()
  renderApp()
  const input = await screen.findByLabelText('Файл карты компетенций')
  fireEvent.change(input, { target: { files: [new File(['map'], 'map.xls')] } })
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Выберите файл CSV или XLSX',
  )
  expect(screen.getByRole('button', { name: 'Загрузить карту' })).toBeDisabled()
  const large = new File(['map'], 'map.csv')
  Object.defineProperty(large, 'size', { value: 25 * 1024 * 1024 + 1 })
  fireEvent.change(input, { target: { files: [large] } })
  expect(await screen.findByRole('alert')).toHaveTextContent('25 МиБ')
  expect(fetch.mock.calls.some(([url]) => url.endsWith('/import'))).toBe(false)
})

it('shows parser details and keeps the file available for retry after a failed import', async () => {
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
  fireEvent.click(screen.getByRole('button', { name: 'Загрузить карту' }))
  fireEvent.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: 'Заменить и загрузить',
    }),
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Строка 7: Важность: ожидается число.',
  )
  expect(screen.getByRole('button', { name: 'Загрузить карту' })).toBeEnabled()
  expect(screen.queryByText('Карта загружена')).not.toBeInTheDocument()
  expect(screen.getByText('Карта ещё не загружена.')).toBeVisible()
})

it('returns to the separate login screen after logout', async () => {
  const fetch = backend()
  fetch.mockImplementationOnce(async () => new Response(JSON.stringify(admin)))
  renderApp()
  await screen.findByRole('heading', { name: 'Карта компетенций' })
  fetch.mockImplementation(async () => new Response(null, { status: 204 }))
  fireEvent.click(screen.getByRole('button', { name: 'Выйти' }))
  expect(
    await screen.findByRole('heading', { name: 'Вход в AI Tutor' }),
  ).toBeVisible()
  expect(
    screen.queryByLabelText('Файл карты компетенций'),
  ).not.toBeInTheDocument()
})

it('does not expose the upload form to guests opening the admin URL directly', async () => {
  const fetch = backend({ user: null })
  renderApp('/admin/competency-map')
  expect(
    await screen.findByRole('heading', { name: 'Вход в AI Tutor' }),
  ).toBeVisible()
  expect(
    screen.queryByLabelText('Файл карты компетенций'),
  ).not.toBeInTheDocument()
  expect(
    fetch.mock.calls.every(([url]) => /\/auth\/(me|refresh)$/.test(url)),
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
      if (url.endsWith('/competency-map')) return Response.json(emptyMap)
      throw new Error(`Unexpected request: ${url}`)
    }),
  )
  renderApp('/admin/competency-map')
  expect(
    await screen.findByRole('heading', { name: 'Карта компетенций' }),
  ).toBeVisible()
  expect(
    screen.queryByRole('heading', { name: 'Вход в AI Tutor' }),
  ).not.toBeInTheDocument()
})

it('blocks a second upload and changing the file while import is pending', async () => {
  const fetch = backend()
  let finish!: (response: Response) => void
  renderApp()
  const input = await screen.findByLabelText('Файл карты компетенций')
  fetch.mockImplementation(async (url: string) =>
    url.endsWith('/import')
      ? new Promise<Response>((resolve) => {
          finish = resolve
        })
      : new Response(JSON.stringify(emptyMap)),
  )
  fireEvent.change(input, { target: { files: [new File(['map'], 'map.csv')] } })
  fireEvent.click(screen.getByRole('button', { name: 'Загрузить карту' }))
  fireEvent.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: 'Заменить и загрузить',
    }),
  )
  expect(await screen.findByText('Загружаем и проверяем карту…')).toBeVisible()
  expect(input).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Загрузить карту' })).toBeDisabled()
  await act(async () => finish(new Response(JSON.stringify(imported))))
  expect(await screen.findByText('Карта загружена.')).toBeVisible()
})

it('returns to login when the backend rejects an import with an expired session', async () => {
  backend({
    status: 401,
    result: { code: 'UNAUTHORIZED', message: 'Сессия завершилась.' },
  })
  renderApp()
  fireEvent.change(await screen.findByLabelText('Файл карты компетенций'), {
    target: { files: [new File(['map'], 'map.csv')] },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Загрузить карту' }))
  fireEvent.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: 'Заменить и загрузить',
    }),
  )
  expect(
    await screen.findByRole('heading', { name: 'Вход в AI Tutor' }),
  ).toBeVisible()
  expect(
    screen.queryByLabelText('Файл карты компетенций'),
  ).not.toBeInTheDocument()
})

it('does not claim success for an unreadable import result', async () => {
  backend({ result: { revision: 2 } })
  renderApp()
  fireEvent.change(await screen.findByLabelText('Файл карты компетенций'), {
    target: { files: [new File(['map'], 'map.csv')] },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Загрузить карту' }))
  fireEvent.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: 'Заменить и загрузить',
    }),
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Проверьте текущую карту перед повторной загрузкой',
  )
  expect(screen.queryByText('Карта загружена.')).not.toBeInTheDocument()
  await waitFor(() =>
    expect(
      screen.getByRole('region', { name: 'Текущая карта' }),
    ).toHaveTextContent('Версия 2'),
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

it('can log in again after an expired map refresh without reusing the cached 401', async () => {
  const fetch = backend()
  const router = renderApp()
  await screen.findByText('Карта ещё не загружена.')
  fetch.mockImplementation(async (url: string) =>
    url.endsWith('/competency-map')
      ? new Response(JSON.stringify({ message: 'Сессия завершилась.' }), {
          status: 401,
        })
      : new Response(JSON.stringify(admin)),
  )
  await act(async () => {
    await router.options.context!.queryClient.invalidateQueries({
      queryKey: ['competency-map'],
    })
  })
  await screen.findByRole('heading', { name: 'Вход в AI Tutor' })
  let finish!: (response: Response) => void
  fetch.mockImplementation(async (url: string) =>
    url.endsWith('/auth/login')
      ? new Response(JSON.stringify({ user: admin }))
      : new Promise<Response>((resolve) => {
          finish = resolve
        }),
  )
  fireEvent.change(screen.getByLabelText('Email'), {
    target: { value: 'admin@example.com' },
  })
  fireEvent.change(screen.getByLabelText('Пароль'), {
    target: { value: 'admin-password' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Войти' }))
  await waitFor(() => expect(finish).toBeDefined())
  await act(async () => finish(new Response(JSON.stringify(emptyMap))))
  expect(
    await screen.findByRole('heading', { name: 'Карта компетенций' }),
  ).toBeVisible()
  expect(
    screen.queryByRole('heading', { name: 'Вход в AI Tutor' }),
  ).not.toBeInTheDocument()
})
