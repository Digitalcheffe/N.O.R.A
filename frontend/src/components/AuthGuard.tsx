import { Navigate, useLocation } from 'react-router-dom'
import type { ReactNode } from 'react'
import { useAuth } from '../context/AuthContext'

interface AuthGuardProps {
  children: ReactNode
}

export function AuthGuard({ children }: AuthGuardProps) {
  const { isLoading, isAuthenticated, setupRequired, mfaEnrollmentRequired, pwPolicyNoncompliant } = useAuth()
  const location = useLocation()

  if (isLoading) {
    return null
  }

  if (setupRequired) {
    return <Navigate to="/setup" replace />
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />
  }

  // Force user to profile page until they address warnings.
  const needsProfileAction = mfaEnrollmentRequired || pwPolicyNoncompliant
  if (needsProfileAction && location.pathname !== '/profile') {
    return <Navigate to="/profile" replace />
  }

  return <>{children}</>
}
