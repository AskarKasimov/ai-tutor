import { useState } from 'react'
import { LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../data/use-auth'
import { isMockApi } from '../data/api-fetch'
import styles from './auth-modal.module.scss'
import type { FormEvent } from 'react'

export function AuthForm() {
  const { t } = useTranslation()
  const { checkingSession, authenticate } = useAuth()
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [name, setName] = useState('')
  const pending = checkingSession || authenticate.isPending
  const emptyDemoLogin =
    isMockApi() && mode === 'login' && email === '' && password === ''

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending) return
    try {
      await authenticate.mutateAsync({
        mode,
        email: email.trim(),
        password,
        ...(mode === 'register' && name.trim()
          ? { display_name: name.trim() }
          : {}),
      })
      setPassword('')
    } catch {
      /* The mutation error is displayed in the form. */
    }
  }

  return (
    <section className={styles.modal} aria-labelledby="auth-title">
      <h1 id="auth-title">{t(`auth.${mode}Title`)}</h1>
      <p className={styles.subtitle}>{t('auth.subtitle')}</p>
      <div className={styles.tabs}>
        {(['login', 'register'] as const).map((tab) => (
          <button
            key={tab}
            type="button"
            aria-pressed={mode === tab}
            disabled={pending}
            className={mode === tab ? styles.selected : undefined}
            onClick={() => {
              setMode(tab)
              setPassword('')
              authenticate.reset()
            }}
          >
            {t(tab === 'login' ? 'auth.loginTab' : 'auth.register')}
          </button>
        ))}
      </div>
      <form onSubmit={submit}>
        <fieldset disabled={pending}>
          {mode === 'register' && (
            <label>
              {t('auth.name')}
              <input
                autoComplete="nickname"
                value={name}
                onChange={(event) => setName(event.target.value)}
                maxLength={200}
              />
            </label>
          )}
          <label>
            {t('auth.email')}
            <input
              type="email"
              autoComplete="username"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              required={!emptyDemoLogin}
              maxLength={254}
            />
          </label>
          <label>
            {t('auth.password')}
            <input
              type="password"
              autoComplete={
                mode === 'register' ? 'new-password' : 'current-password'
              }
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              required={!emptyDemoLogin}
              minLength={mode === 'register' ? 15 : 1}
              maxLength={128}
              aria-describedby={
                mode === 'register' ? 'password-help' : undefined
              }
            />
          </label>
          {mode === 'register' && (
            <p id="password-help" className={styles.hint}>
              {t('auth.passwordHelp')}
            </p>
          )}
          {authenticate.isError && (
            <p role="alert" className={styles.error}>
              {authenticate.error.message}
            </p>
          )}
          <button className={styles.submit} type="submit">
            {pending && (
              <LoaderCircle
                size={18}
                className={styles.spinner}
                aria-hidden="true"
              />
            )}
            {t(
              checkingSession
                ? 'auth.checkingSession'
                : pending
                  ? 'auth.pending'
                  : mode === 'register'
                    ? 'auth.create'
                    : 'auth.login',
            )}
          </button>
        </fieldset>
      </form>
    </section>
  )
}
