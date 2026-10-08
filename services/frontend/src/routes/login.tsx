import { createFileRoute } from '@tanstack/react-router'
import { AuthForm } from './-auth-form'
import styles from './auth-modal.module.scss'

export const Route = createFileRoute('/login')({ component: Login })

function Login() {
  return (
    <main className={styles.screen}>
      <AuthForm />
    </main>
  )
}
