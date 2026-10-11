import { afterEach, expect, it, vi } from 'vitest'
import { apiFetch } from '@/shared/api'
import { createAppDependencies } from '@/bootstrap/dependencies'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.unstubAllEnvs()
})

it.each([undefined, '', 'mock', 'legacy-demo', 'preview', 'real'])(
  'uses the backend even with obsolete API mode %s',
  async (mode) => {
    vi.stubEnv('VITE_API_MODE', mode)
    createAppDependencies()
    const response = Response.json({ user: { id: 'backend-user' } })
    const fetch = vi.fn().mockResolvedValue(response)
    vi.stubGlobal('fetch', fetch)
    const options = { method: 'POST', credentials: 'include' as const }
    expect(await apiFetch('/api/v1/auth/login', options)).toBe(response)
    expect(fetch).toHaveBeenCalledWith('/api/v1/auth/login', options)
  },
)
