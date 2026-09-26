import { create } from 'zustand'
import type { AlertChange, AlertChangeKind, LiveAlert, LivePoint, LiveState, MeterLive, Snapshot, Tick } from './types'

export type Connection = 'idle' | 'connecting' | 'open' | 'reconnecting'

export interface Toast {
  key: number
  change: AlertChange
}

export interface LiveData {
  connection: Connection
  state: LiveState | null
  meters: MeterLive[]
  alerts: Record<string, LiveAlert>
  /** Alert transitions, newest first. */
  feed: AlertChange[]
  toasts: Toast[]
}

export const CHANGE_LABEL: Record<AlertChangeKind, string> = {
  candidate: 'CANDIDATA',
  confirmed: 'CONFIRMADA',
  updated: 'ACTUALIZADA',
  closed: 'CERRADA',
}

/** Points kept per meter for the sparklines (two simulated days). */
export const RECENT = 48
const FEED = 60
let toastSeq = 0

export const initialLive: LiveData = { connection: 'idle', state: null, meters: [], alerts: {}, feed: [], toasts: [] }

// Pure reducers: easy to test and shared by the store.

export function applySnapshot(d: LiveData, s: Snapshot): LiveData {
  const alerts: Record<string, LiveAlert> = {}
  for (const a of s.alerts) alerts[a.id] = a
  return { ...d, state: s.state, meters: s.meters.map((m) => ({ ...m, recent: m.recent ?? [] })), alerts, feed: [], toasts: [] }
}

export function applyTick(d: LiveData, t: Tick): LiveData {
  const byMeter = new Map<string, LivePoint[]>()
  for (const p of t.readings) byMeter.set(p.meter_id, [...(byMeter.get(p.meter_id) ?? []), p])
  const meters = d.meters.map((m) => {
    const add = byMeter.get(m.id) ?? []
    return { ...m, status: t.status[m.id] ?? m.status, recent: [...m.recent, ...add].slice(-RECENT) }
  })
  return { ...d, state: t.state, meters }
}

/** Candidates only update the list; confirmed, updated and closed alerts also notify. */
export function applyAlert(d: LiveData, c: AlertChange): LiveData {
  const toasts = c.change === 'candidate' ? d.toasts : [...d.toasts, { key: ++toastSeq, change: c }].slice(-4)
  return { ...d, alerts: { ...d.alerts, [c.alert.id]: c.alert }, feed: [c, ...d.feed].slice(0, FEED), toasts }
}

export function applyControl(d: LiveData, s: LiveState): LiveData {
  return { ...d, state: s }
}

/** Open alerts first (confirmed before candidates, by priority), then closed ones. */
export function sortedAlerts(alerts: Record<string, LiveAlert>): LiveAlert[] {
  const rank = { CONFIRMED: 0, CANDIDATE: 1, CLOSED: 2 } as const
  return Object.values(alerts).sort(
    (a, b) => rank[a.state] - rank[b.state] || b.priority_score - a.priority_score || b.updated_at.localeCompare(a.updated_at),
  )
}

interface LiveStore extends LiveData {
  setConnection: (c: Connection) => void
  snapshot: (s: Snapshot) => void
  tick: (t: Tick) => void
  alert: (c: AlertChange) => void
  control: (s: LiveState) => void
  dismiss: (key: number) => void
}

export const useLiveStore = create<LiveStore>()((set) => ({
  ...initialLive,
  setConnection: (connection) => set({ connection }),
  snapshot: (s) => set((d) => applySnapshot(d, s)),
  tick: (t) => set((d) => applyTick(d, t)),
  alert: (c) => set((d) => applyAlert(d, c)),
  control: (s) => set((d) => applyControl(d, s)),
  dismiss: (key) => set((d) => ({ toasts: d.toasts.filter((t) => t.key !== key) })),
}))
