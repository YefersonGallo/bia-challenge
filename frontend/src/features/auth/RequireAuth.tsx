import type { ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router-dom'
import { useAuthStore } from './authStore'

/** Redirects to /login when there is no session, remembering where the user wanted to go. */
export function RequireAuth({ children }: { children: ReactNode }) {
  const token = useAuthStore((s) => s.token)
  const notice = useAuthStore((s) => s.notice)
  const location = useLocation()
  // After an explicit logout the next session starts at the home screen;
  // otherwise (expired session, deep link) it returns to where the user was.
  if (!token) return <Navigate to="/login" replace state={notice === 'user' ? undefined : { from: location.pathname + location.search }} />
  return <>{children}</>
}
