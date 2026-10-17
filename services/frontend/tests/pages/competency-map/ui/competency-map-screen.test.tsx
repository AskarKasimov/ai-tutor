import { render, screen } from '@testing-library/react'
import {
  createMemoryHistory,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { afterEach, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import { routeTree } from '@/routeTree.gen'

afterEach(() => vi.unstubAllGlobals())

it.each([
  ['parsed', 'status', 'Предупреждение при распознавании'],
  ['failed', 'alert', 'Ошибка обработки материала'],
] as const)(
  'shows an API processing message for a %s material',
  async (status, role, label) => {
    const message =
      status === 'parsed'
        ? 'Часть формул не распознана; проверьте исходные слайды.'
        : 'Не удалось извлечь текст из PDF.'
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.endsWith('/auth/me'))
          return Response.json({
            id: 'teacher',
            email: 'teacher@example.com',
            display_name: 'Преподаватель',
            role: 'admin',
          })
        if (url.endsWith('/subjects'))
          return Response.json([
            { id: 'course', name: 'Искусственный интеллект', ready: true },
          ])
        if (url.endsWith('/competency-map'))
          return Response.json({
            revision: 0,
            imported_at: null,
            competencies: [],
          })
        if (url.endsWith('/materials'))
          return Response.json([
            {
              id: 'lecture',
              subject_id: 'course',
              filename: 'lecture.pdf',
              content_type: 'application/pdf',
              file_size: 128,
              s3_raw_key: 'raw/lecture.pdf',
              s3_parsed_key: 'parsed/lecture.md',
              parsed_text: 'Текст лекции',
              char_count: 12,
              status,
              error_message: message,
              created_at: 1792180000,
              updated_at: 1792180000,
            },
          ])
        if (url.endsWith('/graph'))
          return Response.json({
            status: 'waiting_materials',
            attempts: 0,
            error_message: '',
            graph: null,
            stale: false,
            updated_at: 0,
          })
        throw new Error(`Unexpected request: ${url}`)
      }),
    )
    const router = createRouter({
      context: { queryClient: createQueryClient() },
      history: createMemoryHistory({
        initialEntries: ['/admin/competency-map'],
      }),
      routeTree,
    })
    render(<RouterProvider router={router} />)

    expect(await screen.findByRole(role, { name: label })).toHaveTextContent(
      message,
    )
    expect(screen.getByText('lecture.pdf')).toBeVisible()
  },
)
