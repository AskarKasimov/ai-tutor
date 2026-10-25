import { isAdminPortal } from '@/shared/api'

const studentAuth = ['auth', 'me', 'student'] as const
const adminAuth = ['auth', 'me', 'admin'] as const
export const userQueryKeys = {
  // Keep the established key as the student portal key for existing consumers.
  auth: studentAuth,
  adminAuth,
  authForPortal: (admin = isAdminPortal()) => (admin ? adminAuth : studentAuth),
  authenticate: ['auth', 'authenticate'] as const,
  logout: ['auth', 'logout'] as const,
}
