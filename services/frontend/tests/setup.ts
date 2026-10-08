import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach, beforeEach, vi } from 'vitest'

afterEach(cleanup)
// Contract/UI tests deliberately exercise the real adapter, irrespective of
// the developer's local demo env. Mock-mode tests override this per test.
beforeEach(() => vi.stubEnv('VITE_API_MODE', 'real'))

Object.defineProperty(window, 'matchMedia', {
  configurable: true,
  value: (query: string): MediaQueryList => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    dispatchEvent: () => false,
    addListener: () => undefined,
    removeListener: () => undefined,
  }),
})

Object.defineProperty(window, 'scrollTo', {
  configurable: true,
  value: () => undefined,
})

// jsdom has <dialog> but not its modal API; opening and closing is all the tests need.
Object.assign(HTMLDialogElement.prototype, {
  showModal(this: HTMLDialogElement) {
    this.open = true
  },
  close(this: HTMLDialogElement) {
    this.open = false
  },
})
