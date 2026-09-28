import type { QueryClient } from '@tanstack/react-query'
import { useAnalysisUi } from '@/features/analysis/analysisStore'
import { useAuthStore } from './authStore'

/**
 * When a session ends (logout or expired token) nothing of it stays in memory:
 * the query cache and the analysis strip are cleared. The analyses themselves
 * live on the server, so the next login shows the latest one again.
 */
export function onSessionEnd(queryClient: QueryClient) {
  return useAuthStore.subscribe((s, prev) => {
    if (prev.token && !s.token) {
      queryClient.clear()
      useAnalysisUi.getState().reset()
    }
  })
}
