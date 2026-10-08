import { AuthForm } from '@/features/auth'
import styles from './login.module.scss'

export function LoginScreen() {
  return (
    <main className={styles.screen}>
      <AuthForm />
    </main>
  )
}
