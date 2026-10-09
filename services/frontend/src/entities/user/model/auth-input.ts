export type AuthInput =
  | { mode: 'teacher' }
  | {
      mode: 'login' | 'register'
      email: string
      password: string
      display_name?: string
    }
