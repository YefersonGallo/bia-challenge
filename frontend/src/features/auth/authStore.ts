import { create } from 'zustand'
import { persist } from 'zustand/middleware'

/** Why the last session ended, shown once on the login screen. */
export type LogoutReason = 'user' | 'expired'

interface AuthState {
  token: string | null
  user: { email: string; name: string } | null
  notice: LogoutReason | null
  login: (token: string, user: { email: string; name: string }) => void
  logout: (reason?: LogoutReason) => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      notice: null,
      login: (token, user) => set({ token, user, notice: null }),
      logout: (reason = 'user') => set({ token: null, user: null, notice: reason }),
    }),
    // Only the session survives a reload; the notice belongs to this tab.
    { name: 'vatio-auth', partialize: (s) => ({ token: s.token, user: s.user }) },
  ),
)
