import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { fmtConf, fmtDayHour } from '@/shared/lib/format'
import { TYPE_META } from '@/shared/lib/labels'
import { CHANGE_LABEL, useLiveStore, type Toast } from './liveStore'

/** Header lamp: LIVE while the replay runs, the simulated clock and the connection. */
export function LiveIndicator() {
  const navigate = useNavigate()
  const connection = useLiveStore((s) => s.connection)
  const state = useLiveStore((s) => s.state)
  if (connection === 'idle' || !state) return null
  const color = connection !== 'open' ? 'var(--color-dim)' : state.running ? 'var(--color-real)' : 'var(--color-expl)'
  const label = connection !== 'open' ? 'RECONECTANDO' : state.running ? 'LIVE' : state.finished ? 'FIN' : 'PAUSA'
  return (
    <button
      type="button"
      onClick={() => navigate('/live')}
      aria-label={`Replay en vivo: ${label}`}
      className="flex items-center gap-1.5 rounded border border-line-2 px-2 py-1 font-mono text-[10px] font-semibold"
    >
      <span className={state.running && connection === 'open' ? 'live-pulse' : undefined} style={{ width: 8, height: 8, borderRadius: 999, background: color }} />
      <span style={{ color }}>{label}</span>
      {state.clock && <span className="text-muted">{fmtDayHour(state.clock)}</span>}
    </button>
  )
}

function ToastItem({ t }: { t: Toast }) {
  const navigate = useNavigate()
  const dismiss = useLiveStore((s) => s.dismiss)
  const { change, alert: a } = t.change
  const meta = TYPE_META[a.type]
  useEffect(() => {
    const id = setTimeout(() => dismiss(t.key), 7000)
    return () => clearTimeout(id)
  }, [t.key, dismiss])
  return (
    <li
      className="pointer-events-auto flex w-[340px] cursor-pointer flex-col gap-0.5 rounded-md border bg-panel px-3 py-2 shadow-lg"
      style={{ borderColor: change === 'closed' ? 'var(--color-line-2)' : meta.color }}
      onClick={() => {
        dismiss(t.key)
        navigate('/live')
      }}
    >
      <span className="flex items-center gap-2 font-mono text-[10px]">
        <span className="font-semibold" style={{ color: change === 'closed' ? 'var(--color-ok)' : meta.color }}>
          {CHANGE_LABEL[change]}
        </span>
        <span className="text-muted">{fmtDayHour(a.updated_at)}</span>
        <span className="ml-auto text-muted">conf {fmtConf(a.confidence)}</span>
      </span>
      <span className="text-[13px]">
        <span className="font-mono font-semibold">{a.meter_id}</span> · {meta.label}
        {change === 'closed' && a.closure ? ` · ${a.closure}` : ''}
      </span>
    </li>
  )
}

/** Notifications for confirmed, updated and closed alerts. */
export function LiveToasts() {
  const toasts = useLiveStore((s) => s.toasts)
  if (toasts.length === 0) return null
  return (
    <ol aria-label="Notificaciones en vivo" aria-live="polite" className="pointer-events-none fixed right-4 bottom-4 z-50 m-0 flex list-none flex-col gap-2 p-0">
      {toasts.map((t) => (
        <ToastItem key={t.key} t={t} />
      ))}
    </ol>
  )
}
