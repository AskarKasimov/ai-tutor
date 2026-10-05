import { useState } from 'react'
import Modal from 'react-modal'
import { LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../data/use-auth'
import { isMockApi } from '../data/api-fetch'
import styles from './auth-modal.module.scss'
import type { FormEvent } from 'react'

export function AuthModal() {
  const { t } = useTranslation()
  const [accountElement, setAccountElement] = useState<HTMLDivElement | null>(null)
  const { user, checkingSession, authenticate, logout } = useAuth()
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [name, setName] = useState('')
  const pending = checkingSession || authenticate.isPending
  const emptyDemoLogin = isMockApi() && mode === 'login' && email === '' && password === ''

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending) return
    try {
      await authenticate.mutateAsync({ mode, email: email.trim(), password,
        ...(mode === 'register' && name.trim() ? { display_name: name.trim() } : {}),
      })
      setPassword('')
    } catch { /* The mutation error is displayed in the form. */ }
  }

  return <div ref={setAccountElement} className={styles.account}>
    {user ? <>
      <span className={styles.user}>{user.display_name || user.email}</span>
      <button className={styles.trigger} disabled={logout.isPending} onClick={() => logout.mutate()}>{t('auth.logout')}</button>
      {logout.isError && <span role="alert" className={styles.error}>{t('auth.networkError')}</span>}
    </> : null}
    <Modal isOpen={!!accountElement && !user} appElement={accountElement?.closest<HTMLDivElement>('[data-trainer-app]') ?? undefined}
      className={styles.modal} overlayClassName={styles.overlay} bodyOpenClassName={styles.bodyOpen}
      contentLabel={t(`auth.${mode}Title`)} shouldCloseOnEsc={false} shouldCloseOnOverlayClick={false}>
      <h2>{t(`auth.${mode}Title`)}</h2>
      <p className={styles.subtitle}>{t('auth.subtitle')}</p>
      <div className={styles.tabs}>
        {(['login', 'register'] as const).map((tab) => <button key={tab} type="button" aria-pressed={mode === tab} disabled={pending}
          className={mode === tab ? styles.selected : undefined}
          onClick={() => { setMode(tab); setPassword(''); authenticate.reset() }}>{t(tab === 'login' ? 'auth.loginTab' : 'auth.register')}</button>)}
      </div>
      <form onSubmit={submit}>
        <fieldset disabled={pending}>
          {mode === 'register' && <label>{t('auth.name')}<input autoComplete="nickname" value={name} onChange={(event) => setName(event.target.value)} maxLength={200} /></label>}
          <label>{t('auth.email')}<input type="email" autoComplete="username" value={email} onChange={(event) => setEmail(event.target.value)} required={!emptyDemoLogin} maxLength={254} /></label>
          <label>{t('auth.password')}<input type="password" autoComplete={mode === 'register' ? 'new-password' : 'current-password'}
            value={password} onChange={(event) => setPassword(event.target.value)} required={!emptyDemoLogin} minLength={mode === 'register' ? 15 : 1} maxLength={128}
            aria-describedby={mode === 'register' ? 'password-help' : undefined} /></label>
          {mode === 'register' && <p id="password-help" className={styles.hint}>{t('auth.passwordHelp')}</p>}
          {authenticate.isError && <p role="alert" className={styles.error}>{authenticate.error.message}</p>}
          <button className={styles.submit} type="submit">{pending && <LoaderCircle size={18} className={styles.spinner} />}{t(checkingSession ? 'auth.checkingSession' : pending ? 'auth.pending' : mode === 'register' ? 'auth.create' : 'auth.login')}</button>
        </fieldset>
      </form>
    </Modal>
  </div>
}
