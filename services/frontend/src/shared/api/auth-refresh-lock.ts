// Cookies are shared by tabs. Rotating the same refresh token twice revokes
// the backend session, so serialize recovery across same-origin tabs.
export function withAuthRefreshLock<T>(
  name: string,
  signal: AbortSignal,
  recover: () => Promise<T>,
): Promise<T> {
  if (navigator.locks) {
    return navigator.locks.request(name, { signal }, recover)
  }
  // Older browsers still get single-flight recovery within this tab.
  return recover()
}
