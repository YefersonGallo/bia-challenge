import { useState } from 'react'

interface ReviewState {
  done: string[]
  reviewed: boolean
}

const EMPTY: ReviewState = { done: [], reviewed: false }
const storageKey = (runId: string) => `vatio-report-review:${runId}`

function load(runId: string): ReviewState {
  try {
    const raw = localStorage.getItem(storageKey(runId))
    return raw ? { ...EMPTY, ...(JSON.parse(raw) as ReviewState) } : EMPTY
  } catch {
    return EMPTY
  }
}

/**
 * Per-viewer review state of a report (plan checkboxes and "reviewed" mark), kept in
 * localStorage per analysis run. Purely a convenience: the source of truth of the
 * anomaly lifecycle stays in the API.
 */
export function useReportReview(runId: string | undefined) {
  // Overrides written in this session, per run; anything else is read from storage.
  const [byRun, setByRun] = useState<Record<string, ReviewState>>({})
  const state = runId ? (byRun[runId] ?? load(runId)) : EMPTY

  const save = (next: ReviewState) => {
    if (!runId) return
    setByRun((m) => ({ ...m, [runId]: next }))
    try {
      localStorage.setItem(storageKey(runId), JSON.stringify(next))
    } catch {
      /* storage unavailable: keep in memory */
    }
  }

  return {
    done: new Set(state.done),
    reviewed: state.reviewed,
    toggle: (key: string) =>
      save({ ...state, done: state.done.includes(key) ? state.done.filter((k) => k !== key) : [...state.done, key] }),
    setReviewed: (reviewed: boolean) => save({ ...state, reviewed }),
  }
}
