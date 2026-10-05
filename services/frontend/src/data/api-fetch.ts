export function isMockApi() {
  return import.meta.env.VITE_API_MODE !== 'real'
}

// All backend requests cross this boundary. Mocks never fall through to fetch.
export async function apiFetch(url: string, options?: RequestInit): Promise<Response> {
  if (isMockApi()) {
    const { mockApiFetch } = await import('../mocks/api')
    return mockApiFetch(url, options)
  }
  return fetch(url, options)
}
