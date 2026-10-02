import { fireEvent, render, screen } from '@testing-library/react'
import { createMemoryHistory, createRouter, RouterProvider } from '@tanstack/react-router'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { createQueryClient } from '../app/providers'
import { i18n } from '../i18n/i18n'
import { routeTree } from '../routeTree.gen'

function renderTraining() {
  const router = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: ['/training'] }),
    routeTree,
  })
  return render(<RouterProvider router={router} />)
}

beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn().mockImplementation(async (url: string, init?: RequestInit) => {
    if (url.endsWith('/auth/me')) return { ok: true, status: 200, json: async () => ({ id: 'u-1', email: 'student@example.com', display_name: 'Студент' }) }
    if (url.endsWith('/outcomes')) return { ok: true, json: async () => ({ outcomes: [{ id: 'o-1', name: 'Дроби', constituent_name: 'Составляющая', competency_name: 'ПК-1', task_count: 1 }] }) }
    if (init?.method === 'POST' && url.endsWith('/training-tasks')) {
      return { ok: true, json: async () => ({ outcome_id: 'o-1', outcome_name: 'Дроби', tasks: [{ question: 'Новый вопрос?', criteria: 'Критерий', voice_instruction: 'Ответьте голосом.', options: [] }] }) }
    }
    return { ok: false, status: 404 }
  }))
})

afterEach(async () => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  await i18n.changeLanguage('ru')
})

it('generates analogous tasks for the selected topic', async () => {
  renderTraining()
  expect(await screen.findByRole('option', { name: /Дроби/ })).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Сгенерировать похожие задания' }))
  expect(await screen.findByRole('heading', { name: 'Новый вопрос?' })).toBeVisible()
  expect(screen.getByText('Задание 1 из 1')).toBeVisible()
})

it('does not allow generating from a topic without sample tasks', async () => {
  vi.stubGlobal('fetch', vi.fn().mockImplementation(async (url: string) => {
    if (url.endsWith('/auth/me')) return { ok: true, status: 200, json: async () => ({ id: 'u-1', email: 'student@example.com', display_name: 'Студент' }) }
    if (url.endsWith('/outcomes')) return { ok: true, json: async () => ({ outcomes: [{ id: 'o-1', name: 'Тема без образцов', constituent_name: 'Составляющая', competency_name: 'ПК-1', task_count: 0 }] }) }
    return { ok: false, status: 404 }
  }))
  renderTraining()
  expect(await screen.findByText('Для этой темы пока нет заданий-образцов. Выберите другую тему.')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Сгенерировать похожие задания' })).toBeDisabled()
})
