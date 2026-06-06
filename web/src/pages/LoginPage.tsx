import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../context/authContext.ts'
import { authApi } from '../api/client.ts'
import styles from './LoginPage.module.css'

const PROVIDER_LABELS: Record<string, string> = { github: 'GitHub', google: 'Google' }

export function LoginPage() {
  const { login, verify2fa } = useAuth()
  const navigate = useNavigate()

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [twoFaCode, setTwoFaCode] = useState('')
  const [step, setStep] = useState<'login' | '2fa'>('login')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [providers, setProviders] = useState<string[]>([])

  useEffect(() => {
    let active = true
    authApi
      .providers()
      .then((r) => {
        if (active) setProviders(r.providers ?? [])
      })
      .catch(() => {
        /* OAuth optional; ignore when unavailable */
      })
    return () => {
      active = false
    }
  }, [])

  async function handleLogin(e: React.FormEvent) {
    e.preventDefault()
    if (!username.trim() || !password.trim()) return
    setError(null)
    setLoading(true)
    try {
      const result = await login(username, password)
      if (result.twofa_required) {
        setStep('2fa')
      } else {
        navigate('/browser', { replace: true })
      }
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : '登入失敗')
    } finally {
      setLoading(false)
    }
  }

  async function handleVerify2fa(e: React.FormEvent) {
    e.preventDefault()
    if (!twoFaCode.trim()) return
    setError(null)
    setLoading(true)
    try {
      await verify2fa(twoFaCode)
      navigate('/browser', { replace: true })
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : '驗證失敗')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className={styles.container}>
      <div className={styles.card}>
        <h1 className={styles.title}>FileTag 登入</h1>

        {step === 'login' ? (
          <form onSubmit={(e) => void handleLogin(e)} className={styles.form}>
            <label className={styles.label}>
              帳號
              <input
                className={styles.input}
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                autoComplete="username"
                required
                disabled={loading}
              />
            </label>

            <label className={styles.label}>
              密碼
              <input
                className={styles.input}
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="current-password"
                required
                disabled={loading}
              />
            </label>

            {error && <p className={styles.error}>{error}</p>}

            <button className={styles.submit} type="submit" disabled={loading}>
              {loading ? '登入中…' : '登入'}
            </button>
          </form>
        ) : (
          <form onSubmit={(e) => void handleVerify2fa(e)} className={styles.form}>
            <p className={styles.hint}>請輸入您的兩步驟驗證碼</p>

            <label className={styles.label}>
              驗證碼
              <input
                className={styles.input}
                type="text"
                value={twoFaCode}
                onChange={(e) => setTwoFaCode(e.target.value)}
                autoComplete="one-time-code"
                inputMode="numeric"
                pattern="[0-9]*"
                maxLength={8}
                required
                disabled={loading}
                autoFocus
              />
            </label>

            {error && <p className={styles.error}>{error}</p>}

            <button className={styles.submit} type="submit" disabled={loading}>
              {loading ? '驗證中…' : '驗證'}
            </button>

            <button
              type="button"
              className={styles.back}
              onClick={() => {
                setStep('login')
                setError(null)
                setTwoFaCode('')
              }}
            >
              ← 返回登入
            </button>
          </form>
        )}

        {step === 'login' && providers.length > 0 && (
          <div className={styles.oauth}>
            <div className={styles.divider}>或</div>
            {providers.map((p) => (
              <a key={p} className={styles.oauthButton} href={authApi.oauthUrl(p)}>
                使用 {PROVIDER_LABELS[p] ?? p} 登入
              </a>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
