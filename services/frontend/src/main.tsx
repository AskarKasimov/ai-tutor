import '@radix-ui/themes/styles.css'
import { createRouter, RouterProvider } from '@tanstack/react-router'
import { createRoot } from 'react-dom/client'

import { createQueryClient } from './app/providers'
import './app/global.scss'
import { routeTree } from './routeTree.gen'

export const router = createRouter({
  context: { queryClient: createQueryClient() },
  routeTree,
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

createRoot(document.getElementById('root')!).render(
  <RouterProvider router={router} />,
)
