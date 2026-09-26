import { useEffect } from 'react'
import { useMutation } from '@tanstack/react-query'
import { api, API_BASE } from '@/shared/api/client'
import { useAuthStore } from '@/features/auth/authStore'
import { useLiveStore } from './liveStore'
import type { AlertChange, LiveControl, LiveState, Snapshot, Tick } from './types'

/** Delay before reopening the stream after it drops (ms). */
export const RETRY_MS = 2000

/**
 * Keeps one EventSource on /api/stream while the user is signed in.
 *
 * The URL carries a short-lived stream token (POST /api/stream/token), never the
 * session token, so each (re)connection asks for a fresh one and reopens the
 * stream with `last_event_id`: the server replays only what was missed, or sends
 * a new snapshot when that is no longer possible.
 */
export function useLiveStream() {
  const token = useAuthStore((s) => s.token)
  useEffect(() => {
    if (!token || typeof EventSource === 'undefined') return
    let es: EventSource | null = null
    let retry: ReturnType<typeof setTimeout> | undefined
    let lastId = ''
    let stopped = false

    const connect = async () => {
      const live = useLiveStore.getState()
      live.setConnection(lastId ? 'reconnecting' : 'connecting')
      let streamToken: string
      try {
        streamToken = (await api<{ token: string }>('/stream/token', { method: 'POST' })).token
      } catch {
        if (!stopped) retry = setTimeout(connect, RETRY_MS)
        return
      }
      if (stopped) return
      const qs = new URLSearchParams({ token: streamToken })
      if (lastId) qs.set('last_event_id', lastId)
      es = new EventSource(`${API_BASE}/stream?${qs}`)
      const on = <T>(name: string, fn: (data: T) => void) =>
        es!.addEventListener(name, (e) => {
          const m = e as MessageEvent<string>
          if (m.lastEventId) lastId = m.lastEventId
          try {
            fn(JSON.parse(m.data) as T)
          } catch {
            // a malformed frame is ignored; the next snapshot or tick repairs the state
          }
        })
      es.onopen = () => useLiveStore.getState().setConnection('open')
      es.onerror = () => {
        // The stream token is single-use in practice (it expires in a minute):
        // close and reconnect with a new one instead of letting EventSource retry.
        es?.close()
        if (stopped) return
        useLiveStore.getState().setConnection('reconnecting')
        retry = setTimeout(connect, RETRY_MS)
      }
      on<Snapshot>('snapshot', (s) => useLiveStore.getState().snapshot(s))
      on<Tick>('tick', (t) => useLiveStore.getState().tick(t))
      on<AlertChange>('alert', (c) => useLiveStore.getState().alert(c))
      on<LiveState>('control', (s) => useLiveStore.getState().control(s))
    }

    void connect()
    return () => {
      stopped = true
      clearTimeout(retry)
      es?.close()
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
