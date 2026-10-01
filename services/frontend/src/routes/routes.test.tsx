import { render, screen } from '@testing-library/react'
import { createMemoryHistory, createRouter, RouterProvider } from '@tanstack/react-router'
import { afterEach, expect, it } from 'vitest'

import { createQueryClient } from '../app/providers'
import { i18n } from '../i18n/i18n'
import { routeTree } from '../routeTree.gen'

function renderHome() {
  const router = createRouter({
    context: { queryClient: createQueryClient() },
    history: createMemoryHistory({ initialEntries: ['/'] }),
    routeTree,
  })
  return render(<RouterProvider router={router} />)
}

afterEach(async () => {
  await i18n.changeLanguage('ru')
  i18n.removeResourceBundle('test', 'translation')
})

it('opens the placeholder at the root route', async () => {
  renderHome()
  expect(await screen.findByRole('heading', { level: 1, name: 'AI Tutor' })).toBeVisible()
  expect(screen.getByText('Здесь скоро появится AI-репетитор.')).toBeVisible()
})

it('renders the page strings through i18n', async () => {
  i18n.addResourceBundle('test', 'translation', {
    routes: { home: { title: 'Tutor', description: 'Coming soon.' } },
  })
  await i18n.changeLanguage('test')
  renderHome()
  expect(await screen.findByRole('heading', { name: 'Tutor' })).toBeVisible()
  expect(screen.getByText('Coming soon.')).toBeVisible()
})
