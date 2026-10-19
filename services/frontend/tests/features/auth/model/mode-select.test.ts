import { afterEach, expect, it, vi } from 'vitest'

import { modeSelectUrl } from '@/features/auth/model/mode-select'

afterEach(() => {
  vi.unstubAllEnvs()
})

it('has no start page outside the two-mode stand', () => {
  vi.stubEnv('VITE_MODE_SELECT_URL', '')
  expect(modeSelectUrl()).toBe('')
})

it('drops trailing slashes of the start page address', () => {
  vi.stubEnv('VITE_MODE_SELECT_URL', 'https://127-0-0-1.sslip.io//')
  expect(modeSelectUrl()).toBe('https://127-0-0-1.sslip.io')
})
