import { useEffect, useRef } from 'react'
import { useAnalysisRun, useInvalidateDerived, useStartAnalysis } from '@/shared/api/queries'
import type { AnalysisRun } from '@/shared/api/types'
import { useAnalysisUi } from './analysisStore'

export const isRunning = (r: AnalysisRun | null | undefined) => r?.status === 'RUNNING' || r?.status === 'PENDING'

/** Percentage of completed steps of a run. */
export function runProgress(r: AnalysisRun | null | undefined): number {
  if (!r || r.steps.length === 0) return 0
  if (r.status === 'COMPLETED') return 100
  const done = r.steps.filter((s) => s.status === 'COMPLETED').length
  return Math.round((done / r.steps.length) * 100)
}

/**
 * Orchestrates the analysis flow: starts a run, follows it with polling and, when it
 * finishes, invalidates every derived query (meters, anomalies, report…).
 */
export function useAnalysis() {
  const { runId, follow } = useAnalysisUi()
  const run = useAnalysisRun(runId)
  const start = useStartAnalysis()
  const invalidate = useInvalidateDerived()
  const prev = useRef<string | undefined>(undefined)
  // Two clicks can land before React re-renders with isPending: guard synchronously.
  const starting = useRef(false)

  const current = run.data ?? null
  useEffect(() => {
    const was = prev.current
    prev.current = current?.status
    if ((was === 'RUNNING' || was === 'PENDING') && (current?.status === 'COMPLETED' || current?.status === 'FAILED')) {
      void invalidate()
    }
  }, [current?.status, invalidate])

  // After a reload the store forgets the run it followed: if the latest run is
  // still in progress, follow it again so the strip comes back.
  useEffect(() => {
    if (!runId && current && isRunning(current)) follow(current.id)
  }, [runId, current, follow])

  const busy = isRunning(current) || start.isPending
  return {
    run: current,
    running: busy,
    progress: runProgress(current),
    // A double click must not send two requests. `onStarted` runs once the run
    // exists (e.g. to navigate away: callbacks of an unmounted caller never run).
    start: (onStarted?: () => void) => {
      if (busy || starting.current) return
      starting.current = true
      start.mutate(undefined, {
        onSuccess: (r) => {
          follow(r.id)
          if (typeof onStarted === 'function') onStarted()
        },
        onSettled: () => {
          starting.current = false
        },
      })
    },
    startError: start.error,
  }
}
