import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import './Login.css'

export function Login() {
  const { login, verifyMFA } = useAuth()
  const navigate = useNavigate()

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  // MFA second-step state
  const [mfaToken, setMfaToken] = useState<string | null>(null)
  const [totpCode, setTotpCode] = useState('')

  const handleLogin = async () => {
    if (!email || !password) {
      setError('Email and password are required.')
      return
    }
    setLoading(true)
    setError('')
    try {
      const mfaChallenge = await login(email, password)
      if (mfaChallenge) {
        setMfaToken(mfaChallenge.mfa_token)
      } else {
        navigate('/', { replace: true })
      }
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Login failed')
    } finally {
      setLoading(false)
    }
  }

  const handleVerifyMFA = async () => {
    if (!mfaToken || !totpCode) return
    setLoading(true)
    setError('')
    try {
      await verifyMFA({ mfa_token: mfaToken, code: totpCode })
      navigate('/', { replace: true })
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Invalid code')
      setTotpCode('')
    } finally {
      setLoading(false)
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      if (mfaToken) handleVerifyMFA()
      else handleLogin()
    }
  }

  return (
    <div className="login-page">
      <div className="login-card">
        <div className="login-logo">
          <img src="/favicon.svg" alt="NORA" className="login-logo-icon" />
        </div>
        <h1 className="login-title">NORA</h1>
        <p className="login-subtitle">Nexus Operations Recon &amp; Alerts</p>

        {mfaToken ? (
          <div className="login-fields">
            <p className="login-mfa-hint">Enter the 6-digit code from your authenticator app.</p>
            <div className="login-field">
              <label className="login-label">Authenticator Code</label>
              <input
                className="login-input login-input--code"
                type="text"
                inputMode="numeric"
                pattern="[0-9]*"
                maxLength={6}
                placeholder="000000"
                value={totpCode}
                onChange={e => setTotpCode(e.target.value.replace(/\D/g, ''))}
                onKeyDown={handleKeyDown}
                autoFocus
                autoComplete="one-time-code"
              />
            </div>
            {error && <div className="login-error">{error}</div>}
            <button className="login-btn" onClick={handleVerifyMFA} disabled={loading || totpCode.length !== 6}>
              {loading ? 'Verifying…' : 'Verify'}
            </button>
            <button
              className="login-btn login-btn--secondary"
              onClick={() => { setMfaToken(null); setTotpCode(''); setError('') }}
            >
              Back
            </button>
          </div>
        ) : (
          <div className="login-fields">
            <div className="login-field">
              <label className="login-label">Email</label>
              <input
                className="login-input"
                type="email"
                placeholder="admin@example.com"
                value={email}
                onChange={e => setEmail(e.target.value)}
                onKeyDown={handleKeyDown}
                autoFocus
                autoComplete="email"
              />
            </div>
            <div className="login-field">
              <label className="login-label">Password</label>
              <input
                className="login-input"
                type="password"
                placeholder="••••••••"
                value={password}
                onChange={e => setPassword(e.target.value)}
                onKeyDown={handleKeyDown}
                autoComplete="current-password"
              />
            </div>

            {error && <div className="login-error">{error}</div>}

            <button
              className="login-btn"
              onClick={handleLogin}
              disabled={loading}
            >
              {loading ? 'Signing in…' : 'Sign in'}
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
