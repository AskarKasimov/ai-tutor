export type User = {
  id: string
  email: string
  display_name: string | null
  role: 'student' | 'admin'
}
