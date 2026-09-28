import { act, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { mockApi, renderWithProviders } from '@/test/utils'
import { useAuthStore } from '@/features/auth/authStore'
import { applyAlert, applySnapshot, applyTick, initialLive, RECENT, sortedAlerts, useLiveStore } from './liveStore'
import { LivePage } from './LivePage'
import { LiveIndicator, LiveToasts } from './LiveWidgets'
import { useLiveStream } from './useLiveStream'
import type { AlertChange, LiveAlert, LivePoint, LiveState, Snapshot, Tick } from './types'

const state = (over: Partial<LiveState> = {}): LiveState => ({
  clock: '2026-09-12T19:00:00Z',
  start: '2026-09-07T00:00:00Z',
  end: '2026-09-14T23:00:00Z',
  hour: 140,
  total_hours: 192,
  running: true,
  finished: false,
  speed: 1,
  step_ms: 500,
  last_tick_ms: 4.8,
  ...over,
})
const pt = (meter: string, h: number, kwh = 40): LivePoint => ({
  meter_id: meter,
  timestamp: new Date(Date.UTC(2026, 8, 12, h)).toISOString(),
  consumption_kwh: kwh,
  voltage_v: 220,
  current_a: 180,
  power_factor: 0.93,
})
const alert = (over: Partial<LiveAlert> = {}): LiveAlert => ({
  id: 'L-M-109-1',
  meter_id: 'M-109',
  state: 'CANDIDATE',
  type: 'REAL_ANOMALY',
  severity: 'HIGH',
  confidence: 0.9,
  priority_score: 2.7,
  reason: 'Consumo +110% sin evento conocido',
  opened_at: '2026-09-12T19:00:00Z',
  updated_at: '2026-09-12T19:00:00Z',
  ...over,
})
const snapshot: Snapshot = {
  state: state({ running: false, hour: 0, clock: '2026-09-06T23:00:00Z' }),
  meters: [
    { id: 'M-109', name: 'Compresor línea 3', status: 'OK', recent: [pt('M-109', 1)] },
    { id: 'M-104', name: 'Línea de ensamble 2', status: 'OK', recent: [] },
  ],
  alerts: [],
}

describe('live store reducers', () => {
  it('appends readings, keeps the last points and updates statuses', () => {
    let d = applySnapshot(initialLive, snapshot)
    const tick: Tick = { state: state(), readings: [pt('M-109', 2, 90), pt('M-104', 2)], status: { 'M-109': 'ALERT', 'M-104': 'OK' } }
    d = applyTick(d, tick)
    expect(d.meters[0].recent.map((p) => p.consumption_kwh)).toEqual([40, 90])
    expect(d.meters[0].status).toBe('ALERT')
    for (let i = 0; i < RECENT + 5; i++) d = applyTick(d, { ...tick, readings: [pt('M-109', 3)] })
    expect(d.meters[0].recent).toHaveLength(RECENT)
  })

  it('notifies confirmed alerts but not candidates, and sorts open alerts first', () => {
    let d = applySnapshot(initialLive, snapshot)
    d = applyAlert(d, { change: 'candidate', alert: alert() })
    expect(d.toasts).toHaveLength(0)
    d = applyAlert(d, { change: 'confirmed', alert: alert({ state: 'CONFIRMED' }) })
    d = applyAlert(d, { change: 'closed', alert: alert({ id: 'L-M-106-2', meter_id: 'M-106', state: 'CLOSED', type: 'FALSE_POSITIVE' }) })
    expect(d.toasts.map((t) => t.change.change)).toEqual(['confirmed', 'closed'])
    expect(d.feed[0].change).toBe('closed')
    expect(sortedAlerts(d.alerts).map((a) => a.state)).toEqual(['CONFIRMED', 'CLOSED'])
  })
})

describe('live UI', () => {
  beforeEach(() => useLiveStore.setState({ ...applySnapshot(initialLive, snapshot), connection: 'open' }))

  it('shows the clock, the meters and sends controls', async () => {
    const { calls } = mockApi({ 'POST /stream/control': state({ running: true }), 'GET /anomalies': [] })
    renderWithProviders(<LivePage />)
    expect(screen.getByText('D6 · 23:00')).toBeInTheDocument()
    expect(screen.getByLabelText('M-109 en vivo')).toBeInTheDocument()
    expect(screen.getByText('Sin alertas todavía.')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'INICIAR' }))
    await waitFor(() => expect(calls).toContainEqual({ method: 'POST', path: '/stream/control', body: { action: 'start' } }))
    await userEvent.click(screen.getByRole('button', { name: '4×' }))
    await waitFor(() => expect(calls).toContainEqual({ method: 'POST', path: '/stream/control', body: { action: 'speed', speed: 4 } }))
  })

  it('lists alerts with their lifecycle and notifies confirmations', () => {
    mockApi({ 'GET /anomalies': [] })
    act(() => {
      useLiveStore.getState().alert({ change: 'candidate', alert: alert() })
      useLiveStore.getState().alert({ change: 'confirmed', alert: alert({ state: 'CONFIRMED', confirmed_at: '2026-09-12T22:00:00Z' }) })
    })
    renderWithProviders(
      <>
        <LiveIndicator />
        <LivePage />
        <LiveToasts />
      </>,
    )
    expect(screen.getByRole('button', { name: 'Replay en vivo: PAUSA' })).toBeInTheDocument()
    expect(screen.getByText(/confirmada D12 · 22:00/)).toBeInTheDocument()
    const toasts = screen.getByRole('list', { name: 'Notificaciones en vivo' })
    expect(toasts).toHaveTextContent('CONFIRMADA')
    expect(toasts).toHaveTextContent('M-109')
  })
})

class FakeEventSource {
  static last: FakeEventSource
  static CLOSED = 2
  readyState = 1
  url: string
  listeners: Record<string, ((e: MessageEvent) => void)[]> = {}
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  closed = false
  constructor(url: string) {
    this.url = url
    FakeEventSource.last = this
  }
  addEventListener(name: string, fn: (e: MessageEvent) => void) {
    ;(this.listeners[name] ??= []).push(fn)
  }
  emit(name: string, data: unknown, id = '') {
    for (const fn of this.listeners[name] ?? []) fn(new MessageEvent(name, { data: JSON.stringify(data), lastEventId: id }))
  }
  close() {
    this.closed = true
  }
}

function Harness() {
  useLiveStream()
  return null
}

describe('useLiveStream', () => {
  it('opens the stream with a short-lived stream token and feeds the store', async () => {
    const { calls } = mockApi({ 'POST /stream/token': { token: 'st 1', expires_at: '2026-09-26T10:01:00Z' } })
    vi.stubGlobal('EventSource', FakeEventSource)
    useLiveStore.setState(initialLive)
    useAuthStore.setState({ token: 'session', user: { email: 'u', name: 'U' } })
    const { unmount } = renderWithProviders(<Harness />)
    await waitFor(() => expect(FakeEventSource.last?.url).toBe('/api/stream?token=st+1'))
    expect(calls[0]).toMatchObject({ method: 'POST', path: '/stream/token' })
    const es = FakeEventSource.last
    act(() => {
      es.onopen?.()
      es.emit('snapshot', snapshot, '100')
      es.emit('tick', { state: state(), readings: [pt('M-109', 2, 95)], status: { 'M-109': 'ALERT' } } satisfies Tick, '101')
      es.emit('alert', { change: 'confirmed', alert: alert({ state: 'CONFIRMED' }) } satisfies AlertChange, '102')
    })
    const s = useLiveStore.getState()
    expect(s.connection).toBe('open')
    expect(s.meters[0].recent.at(-1)?.consumption_kwh).toBe(95)
    expect(Object.values(s.alerts)[0].state).toBe('CONFIRMED')

    // The stream drops: a new stream token and the last id seen, never the session token.
    act(() => es.onerror?.())
    expect(useLiveStore.getState().connection).toBe('reconnecting')
    await waitFor(() => expect(FakeEventSource.last).not.toBe(es), { timeout: 4000 })
    expect(FakeEventSource.last.url).toBe('/api/stream?token=st+1&last_event_id=102')
    expect(FakeEventSource.last.url).not.toContain('session')
    unmount()
    expect(FakeEventSource.last.closed).toBe(true)
    vi.unstubAllGlobals()
    useAuthStore.setState({ token: null, user: null })
  })
})
