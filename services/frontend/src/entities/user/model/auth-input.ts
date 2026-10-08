export type AuthInput = {
  mode: 'login' | 'register'
  email: string
  password: string
  display_name?: string
}
