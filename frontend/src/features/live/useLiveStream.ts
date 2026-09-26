import { useEffect } from 'react'
import { useMutation } from '@tanstack/react-query'
import { api, API_BASE } from '@/shared/api/client'
import { useAuthStore } from '@/features/auth/authStore'
import { useLiveStore } from './liveStore'
import type { AlertChange, LiveControl, LiveState, Snapshot, Tick } from './types'

/**
 * Keeps one EventSource on /api/stream while the user is signed in. The browser
 * reconnects by itself and sends Last-Event-ID, so the server only replays what was
 * missed; a "snapshot" resets the store (first connection or after a reset).
 */
export function useLiveStream() {
  const token = useAuthStore((s) => s.token)
  useEffect(() => {
    if (!token || typeof EventSource === 'undefined') return
    const live = useLiveStore.getState()
    live.setConnection('connecting')
    const es = new EventSource(`${API_BASE}/stream?token=${encodeURIComponent(token)}`)
    const on = <T>(name: string, fn: (data: T) => void) =>
      es.addEventListener(name, (e) => {
        try {
          fn(JSON.parse((e as MessageEvent<string>).data) as T)
        } catch {
          // a malformed frame is ignored; the next snapshot or tick repairs the state
        }
      })
    es.onopen = () => useLiveStore.getState().setConnection('open')
    es.onerror = () => useLiveStore.getState().setConnection(es.readyState === EventSource.CLOSED ? 'idle' : 'reconnecting')
    on<Snapshot>('snapshot', (s) => useLiveStore.getState().snapshot(s))
    on<Tick>('tick', (t) => useLiveStore.getState().tick(t))
    on<AlertChange>('alert', (c) => useLiveStore.getState().alert(c))
    on<LiveState>('control', (s) => useLiveStore.getState().control(s))
    return () => {
      es.close()
      useLiveStore.getState().setConnection('idle')
    }
  }, [token])
}

/** start | pause | reset | speed. The new state also arrives through the stream. */
export function useLiveControl() {
  const control = useLiveStore((s) => s.control)
  return useMutation({
    mutationFn: ({ action, speed }: { action: LiveControl; speed?: number }) =>
      api<LiveState>('/stream/control', { method: 'POST', body: JSON.stringify({ action, speed }) }),
    onSuccess: (s) => control(s),
  })
}
