import { createContext, useContext, useEffect, useState, useCallback } from 'react'
import type { ReactNode } from 'react'
import { auth, totp } from '../api/client'
import type { AuthUser, MFARequiredResponse, TOTPVerifyInput } from '../api/types'

interface AuthContextType {
  user: AuthUser | null
  isLoading: boolean
  isAuthenticated: boolean
  setupRequired: boolean
  mfaEnrollmentRequired: boolean
  pwPolicyNoncompliant: boolean
  // login returns the MFA challenge when TOTP is required, or null for a full login.
  login: (email: string, password: string) => Promise<MFARequiredResponse | null>
  verifyMFA: (input: TOTPVerifyInput) => Promise<void>
  logout: () => Promise<void>
  refreshUser: () => Promise<void>
  clearPwPolicyNoncompliant: () => void
}

const AuthContext = createContext<AuthContextType | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [setupRequired, setSetupRequired] = useState(false)
  const [mfaEnrollmentRequired, setMfaEnrollmentRequired] = useState(false)
  const [pwPolicyNoncompliant, setPwPolicyNoncompliant] = useState(false)

  useEffect(() => {
    const onExpired = () => setUser(null)
    window.addEventListener('nora:session-expired', onExpired)
    return () => window.removeEventListener('nora:session-expired', onExpired)
  }, [])

  useEffect(() => {
    let cancelled = false

    auth.setupRequired()
      .then(res => {
        if (cancelled) return
        if (res.required) {
          setSetupRequired(true)
          setIsLoading(false)
          return
        }
        return auth.me().then(u => {
          if (!cancelled) setUser(u)
        }).catch(() => {
          // 401 — not logged in, user stays null
        })
      })
      .catch(() => {
        // network error — stay in loading=false, user=null
      })
      .finally(() => {
        if (!cancelled) setIsLoading(false)
      })

    return () => { cancelled = true }
  }, [])

  const login = useCallback(async (email: string, password: string): Promise<MFARequiredResponse | null> => {
    const res = await auth.login({ email, password })
    if ('mfa_required' in res && res.mfa_required) {
      return res as MFARequiredResponse
    }
    const loginRes = res as import('../api/types').LoginResponse
    setUser(loginRes.user)
    setSetupRequired(false)
    if (loginRes.mfa_enrollment_required) {
      setMfaEnrollmentRequired(true)
    }
    if (loginRes.pw_policy_noncompliant) {
      setPwPolicyNoncompliant(true)
    }
    return null
  }, [])

  const verifyMFA = useCallback(async (input: TOTPVerifyInput) => {
    const res = await totp.verify(input)
    setUser(res.user)
    setSetupRequired(false)
  }, [])

  const logout = useCallback(async () => {
    await auth.logout()
    setUser(null)
    setMfaEnrollmentRequired(false)
    setPwPolicyNoncompliant(false)
  }, [])

  const refreshUser = useCallback(async () => {
    const u = await auth.me()
    setUser(u)
    if (u.totp_enabled) setMfaEnrollmentRequired(false)
  }, [])

  const clearPwPolicyNoncompliant = useCallback(() => {
    setPwPolicyNoncompliant(false)
  }, [])

  return (
    <AuthContext.Provider value={{
      user,
      isLoading,
      isAuthenticated: user !== null,
      setupRequired,
      mfaEnrollmentRequired,
      pwPolicyNoncompliant,
      login,
      verifyMFA,
      logout,
      refreshUser,
      clearPwPolicyNoncompliant,
    }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth(): AuthContextType {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
