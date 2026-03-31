import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { auth } from '../api/client'
import { useAuth } from '../context/AuthContext'
import './Login.css'

export function Setup() {
  const { login } = useAuth()
  const navigate = useNavigate()

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSetup = async () => {
    if (!email || !password) {
      setError('Email and password are required.')
      return
    }
    setLoading(true)
    setError('')
    try {
      await auth.register({ email, password })
      await login(email, password)
      navigate('/', { replace: true })
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Setup failed')
    } finally {
      setLoading(false)
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') handleSetup()
  }

  return (
    <div className="login-page">
      <div className="login-card">
        <div className="login-logo">
          <svg className="login-logo-icon" viewBox="0 0 40 40" fill="none">
            <rect width="40" height="40" rx="8" fill="var(--accent-dim)" />
            <path d="M8 20 L20 8 L32 20 L20 32 Z" stroke="var(--accent)" strokeWidth="2" fill="none" />
            <circle cx="20" cy="20" r="4" fill="var(--accent)" />
          </svg>
        </div>
        <h1 className="login-title">NORA</h1>
        <p className="login-subtitle">Create your admin account</p>

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
              placeholder="Choose a strong password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              onKeyDown={handleKeyDown}
              autoComplete="new-password"
            />
          </div>

          {error && <div className="login-error">{error}</div>}

          <button
            className="login-btn"
            onClick={handleSetup}
            disabled={loading}
          >
            {loading ? 'Creating account…' : 'Create admin account'}
          </button>
        </div>
      </div>
    </div>
  )
}
