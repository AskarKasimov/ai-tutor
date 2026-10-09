import { z } from 'zod'
import { i18n } from '@/shared/i18n'
import type { User } from '../model/user'
import { apiFetch } from '@/shared/api'

const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(
  /\/$/,
  '',
)
const userSchema = z.object({
  id: z.string(),
  email: z.string(),
  display_name: z.string().nullable().optional(),
  role: z.enum(['student', 'admin']).optional(),
})
import type { AuthInput } from '../model/auth-input'
export type { AuthInput } from '../model/auth-input'

export async function readCurrentUser(
  signal: AbortSignal,
): Promise<User | null> {
  const response = await apiFetch(`${apiBase}/auth/me`, {
    credentials: 'include',
    signal: AbortSignal.any([signal, AbortSignal.timeout(30_000)]),
  })
  if (response.status === 401) return null
  if (!response.ok) throw new Error(i18n.t('auth.networkError'))
  const user = userSchema.safeParse(await response.json())
  if (!user.success) throw new Error(i18n.t('auth.networkError'))
  return {
    id: user.data.id,
    email: user.data.email,
    display_name: user.data.display_name ?? null,
    role: user.data.role ?? 'student',
  }
}
export async function authenticate(
  input: AuthInput,
  signal: AbortSignal,
): Promise<User> {
  let response: Response
  try {
    const { mode, ...body } = input
    response = await apiFetch(`${apiBase}/auth/${mode}`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      ...(mode === 'teacher' ? {} : { body: JSON.stringify(body) }),
      signal: AbortSignal.any([signal, AbortSignal.timeout(30_000)]),
    })
  } catch {
    throw new Error(i18n.t('auth.networkError'))
  }
  const data = await response.json().catch(() => null)
  if (!response.ok) {
    const message =
      data?.details
        ?.map((detail: { message: string }) => detail.message)
        .join(' ') || data?.message
    throw new Error(
      typeof message === 'string' ? message : i18n.t('auth.networkError'),
    )
  }
  const user = userSchema.safeParse(data?.user)
  if (!user.success) throw new Error(i18n.t('auth.networkError'))
  return {
    id: user.data.id,
    email: user.data.email,
    display_name: user.data.display_name ?? null,
    role: user.data.role ?? 'student',
  }
}
export async function logout(signal: AbortSignal): Promise<void> {
  const response = await apiFetch(`${apiBase}/auth/logout`, {
    method: 'POST',
    credentials: 'include',
    signal: AbortSignal.any([signal, AbortSignal.timeout(30_000)]),
  })
  if (!response.ok) throw new Error(i18n.t('auth.networkError'))
}
