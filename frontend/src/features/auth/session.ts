import type { QueryClient } from '@tanstack/react-query'
import { useAnalysisUi } from '@/features/analysis/analysisStore'
import { initialLive, useLiveStore } from '@/features/live/liveStore'
import { useAuthStore } from './authStore'

/**
 * When a session ends (logout or expired token) nothing of it stays in memory:
 * the query cache, the analysis strip and the live replay view are cleared. The analyses themselves
 * live on the server, so the next login shows the latest one again.
 */
export function onSessionEnd(queryClient: QueryClient) {
  return useAuthStore.subscribe((s, prev) => {
    if (prev.token && !s.token) {
      queryClient.clear()
      useAnalysisUi.getState().reset()
      useLiveStore.setState(initialLive) // the next snapshot rebuilds it
    }
  })
}
