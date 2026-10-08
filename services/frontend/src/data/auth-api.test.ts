import { afterEach, expect, it, vi } from 'vitest'
import { readCurrentUser } from './auth-api'

afterEach(() => vi.unstubAllGlobals())

it('restores the current user after reload when access expired but refresh is valid', async () => {
  const user = {
    id: 'student',
    email: 'student@example.com',
    display_name: null,
    role: 'student',
  }
  let accessValid = false
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.endsWith('/auth/refresh')) {
        accessValid = true
        return new Response('{}')
      }
      return accessValid
        ? Response.json(user)
        : Response.json({ code: 'UNAUTHORIZED' }, { status: 401 })
    }),
  )

  expect(await readCurrentUser(new AbortController().signal)).toEqual(user)
})
