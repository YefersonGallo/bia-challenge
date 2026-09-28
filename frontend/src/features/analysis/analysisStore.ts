import { create } from 'zustand'

/** UI state of the "Run AI Analysis" strip: which run it follows and whether it is visible. */
interface AnalysisUiState {
  runId: string | null
  stripOpen: boolean
  follow: (runId: string) => void
  close: () => void
  reset: () => void
}

export const useAnalysisUi = create<AnalysisUiState>()((set) => ({
  runId: null,
  stripOpen: false,
  follow: (runId) => set({ runId, stripOpen: true }),
  close: () => set({ stripOpen: false }),
  reset: () => set({ runId: null, stripOpen: false }),
}))
