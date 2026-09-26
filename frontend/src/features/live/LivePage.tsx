import { useNavigate } from 'react-router-dom'
import { useAnomalies } from '@/shared/api/queries'
import { Sparkline } from '@/shared/charts/small'
import { fmtConf, fmtDayHour, fmtNum } from '@/shared/lib/format'
import { METER_STATUS_META, SEVERITY_LABEL, TYPE_META } from '@/shared/lib/labels'
import { Button, EmptyState, Label, Lamp, Panel, Segmented } from '@/shared/ui/primitives'
import { CHANGE_LABEL, sortedAlerts, useLiveStore } from './liveStore'
import { useLiveControl } from './useLiveStream'

const SPEEDS = ['1', '2', '4', '8'] as const

function Controls() {
  const state = useLiveStore((s) => s.state)
  const control = useLiveControl()
  if (!state) return null
  const pct = state.total_hours ? (state.hour / state.total_hours) * 100 : 0
  return (
    <Panel className="flex flex-wrap items-center gap-4 px-4 py-3">
      <div className="flex flex-col">
        <Label>RELOJ SIMULADO</Label>
        <span className="font-mono text-2xl font-semibold">{state.clock ? fmtDayHour(state.clock) : '—'}</span>
      </div>
      <div className="flex min-w-[220px] flex-1 flex-col gap-1">
        <span className="flex justify-between font-mono text-[10px] text-muted">
          <span>
            HORA {state.hour} DE {state.total_hours} · DESDE {fmtDayHour(state.start)}
          </span>
          <span>
            {fmtNum(state.step_ms)} ms por hora · último ciclo {fmtNum(state.last_tick_ms, 1)} ms
          </span>
        </span>
        <span className="relative h-1.5 rounded-sm bg-raise" role="progressbar" aria-valuenow={state.hour} aria-valuemax={state.total_hours} aria-label="Avance del replay">
          <span className="absolute inset-y-0 left-0 rounded-sm bg-accent" style={{ width: `${pct}%` }} />
        </span>
      </div>
      <div className="flex items-center gap-1.5">
        {state.running ? (
          <Button variant="primary" onClick={() => control.mutate({ action: 'pause' })} disabled={control.isPending}>
            PAUSAR
          </Button>
        ) : (
          <Button variant="primary" onClick={() => control.mutate({ action: 'start' })} disabled={control.isPending}>
            {state.finished ? 'REPETIR' : state.hour > 0 ? 'REANUDAR' : 'INICIAR'}
          </Button>
        )}
        <Button onClick={() => control.mutate({ action: 'reset' })} disabled={control.isPending}>
          REINICIAR
        </Button>
        <Segmented
          label="Velocidad"
          value={String(state.speed) as (typeof SPEEDS)[number]}
          onChange={(v) => control.mutate({ action: 'speed', speed: Number(v) })}
          options={SPEEDS.map((s) => ({ value: s, label: `${s}×` }))}
        />
      </div>
    </Panel>
  )
}

function MeterGrid() {
  const meters = useLiveStore((s) => s.meters)
  const alerts = useLiveStore((s) => s.alerts)
  const open = new Map(Object.values(alerts).filter((a) => a.state !== 'CLOSED').map((a) => [a.meter_id, a]))
  return (
    <div className="grid grid-cols-2 gap-2 md:grid-cols-3 xl:grid-cols-4">
      {meters.map((m) => {
        const a = open.get(m.id)
        const color = a ? TYPE_META[a.type].color : METER_STATUS_META[m.status].color
        const last = m.recent.at(-1)
        return (
          <div
            key={m.id}
            aria-label={`${m.id} en vivo`}
            className="flex flex-col gap-1 rounded-md border bg-panel px-3 py-2"
            style={{ borderColor: a ? color : 'var(--color-line)', borderStyle: a?.state === 'CANDIDATE' ? 'dashed' : 'solid' }}
          >
            <span className="flex items-center justify-between">
              <span className="font-mono text-[13px] font-semibold">{m.id}</span>
              <Lamp color={color} glow={!!a} />
            </span>
            <span className="truncate text-[11px] text-muted">{m.name}</span>
            <Sparkline values={m.recent.map((p) => p.consumption_kwh)} color={color} width={220} height={34} />
            <span className="flex flex-wrap justify-between gap-x-2 font-mono text-[10px]">
              <span>{last ? `${fmtNum(last.consumption_kwh, 1)} kWh/h · ${fmtNum(last.voltage_v)} V · FP ${fmtNum(last.power_factor, 2)}` : '—'}</span>
              <span style={{ color }}>{a ? `${a.state === 'CANDIDATE' ? 'CANDIDATA' : TYPE_META[a.type].short}` : 'OK'}</span>
            </span>
          </div>
        )
      })}
    </div>
  )
}

function AlertList() {
  const alerts = useLiveStore((s) => s.alerts)
  const navigate = useNavigate()
  const { data: batch = [] } = useAnomalies()
  const list = sortedAlerts(alerts)
  if (list.length === 0) return <EmptyState title="Sin alertas todavía.">Inicia el replay: las alertas aparecen como candidatas y se confirman tras 3 h simuladas.</EmptyState>
  return (
    <ol className="m-0 flex list-none flex-col gap-2 p-0" aria-label="Alertas en vivo">
      {list.map((a) => {
        const meta = TYPE_META[a.type]
        const inBatch = batch.find((b) => b.meter_id === a.meter_id)
        return (
          <li key={a.id} className="flex flex-col gap-1 rounded-md border bg-panel px-3 py-2" style={{ borderColor: a.state === 'CLOSED' ? 'var(--color-line)' : meta.color, opacity: a.state === 'CLOSED' ? 0.6 : 1 }}>
            <span className="flex flex-wrap items-center gap-x-2 font-mono text-[11px] whitespace-nowrap">
              <span className="font-semibold">{a.meter_id}</span>
              <span style={{ color: meta.color }}>{meta.label}</span>
              <span className="text-muted">
                · {SEVERITY_LABEL[a.severity].toLowerCase()} · conf {fmtConf(a.confidence)}
              </span>
              <span className="ml-auto rounded-[3px] border border-line-2 px-1.5 text-[9px]">{a.state}</span>
            </span>
            <span className="text-[13px] text-ink-2">{a.reason}</span>
            <span className="font-mono text-[10px] text-muted">
              abierta {fmtDayHour(a.opened_at)}
              {a.confirmed_at && ` · confirmada ${fmtDayHour(a.confirmed_at)}`}
              {a.closed_at && ` · cerrada ${fmtDayHour(a.closed_at)} (${a.closure})`}
            </span>
            {inBatch && (
              <Button size="sm" variant="link" className="self-start" onClick={() => navigate(`/anomalies/${inBatch.id}`)}>
                VER INVESTIGACIÓN DEL ÚLTIMO ANÁLISIS →
              </Button>
            )}
          </li>
        )
      })}
    </ol>
  )
}

function Feed() {
  const feed = useLiveStore((s) => s.feed)
  return (
    <ol className="m-0 flex list-none flex-col gap-1 p-0" aria-label="Registro de cambios">
      {feed.length === 0 && <li className="text-[12px] text-muted">Sin cambios desde que te conectaste.</li>}
      {feed.map((c) => (
        <li key={`${c.alert.id}-${c.change}-${c.alert.updated_at}`} className="flex gap-2 font-mono text-[11px]">
          <span className="text-muted">{fmtDayHour(c.alert.updated_at)}</span>
          <span className="font-semibold" style={{ color: c.change === 'closed' ? 'var(--color-ok)' : TYPE_META[c.alert.type].color }}>
            {CHANGE_LABEL[c.change]}
          </span>
          <span>
            {c.alert.meter_id} · {TYPE_META[c.alert.type].label}
          </span>
        </li>
      ))}
    </ol>
  )
}

/** Live replay: controls, meters hour by hour and the alert lifecycle. */
export function LivePage() {
  const connection = useLiveStore((s) => s.connection)
  const state = useLiveStore((s) => s.state)
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-6 pt-3.5 pb-6">
      <div className="flex items-baseline gap-3">
        <h1 className="m-0 font-display text-xl font-semibold">Replay en vivo</h1>
        <span className="text-[13px] text-muted">
          Los días 1–6 se precargan; desde el día 7 cada hora llega como un evento y el motor vuelve a analizar la ventana completa.
        </span>
      </div>
      {!state ? (
        <EmptyState title={connection === 'reconnecting' ? 'Reconectando con el stream…' : 'Conectando con el stream…'} />
      ) : (
        <>
          <Controls />
          <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_400px]">
            <MeterGrid />
            <div className="flex flex-col gap-3">
              <Panel className="flex flex-col gap-2 px-4 py-3.5">
                <Label>ALERTAS</Label>
                <AlertList />
              </Panel>
              <Panel className="flex flex-col gap-2 px-4 py-3.5">
                <Label>REGISTRO</Label>
                <Feed />
              </Panel>
            </div>
          </div>
        </>
      )}
    </div>
  )
}
