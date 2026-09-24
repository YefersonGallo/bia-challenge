import { useNavigate } from 'react-router-dom'
import { useMeter } from '@/shared/api/queries'
import { DailyChart } from '@/shared/charts/DailyChart'
import { lifecycleLabels } from '@/shared/lib/labels'
import { electricalRows, eventMarkers } from '@/shared/lib/meterMath'
import { meterVerdict } from '@/shared/lib/verdict'
import { Button, ErrorBox, Label, Loading, Tag } from '@/shared/ui/primitives'
import { NextActionButton } from '@/features/anomalies/Lifecycle'

/** Focus on one meter inside the operations view: chart, AI reason, electrical variables and next action. */
export function FocusPanel({ meterId, onClose }: { meterId: string; onClose: () => void }) {
  const navigate = useNavigate()
  const { data: m, isLoading, error } = useMeter(meterId)
  if (isLoading) return <Loading what={meterId} />
  if (error || !m) return <ErrorBox error={error} />

  const v = meterVerdict(m)
  const f = m.finding
  const ep = m.stats.episode
  const signals = f?.evidence.signals ?? []

  return (
    <section aria-label={`Foco ${m.id}`} className="flex min-h-0 flex-1 flex-col rounded-md border bg-panel" style={{ borderColor: v.color }}>
      <header className="flex flex-wrap items-center gap-3 border-b border-line px-4 py-2.5">
        <Label>FOCO</Label>
        <span className="font-mono text-base font-semibold">{m.id}</span>
        <span className="text-[13px] text-ink-2">{m.name}</span>
        <Tag color={v.color}>{v.text}</Tag>
        {f && <span className="font-mono text-[11px] text-muted uppercase">{lifecycleLabels(f.type)[f.status]}</span>}
        <span className="ml-auto flex gap-1.5">
          <Button size="sm" onClick={() => navigate(`/meters/${m.id}`)}>
            VER MEDIDOR
          </Button>
          {f && (
            <Button size="sm" variant="primary" onClick={() => navigate(`/anomalies/${f.id}`)}>
              INVESTIGAR →
            </Button>
          )}
        </span>
        <button type="button" aria-label="Cerrar foco" onClick={onClose} className="h-9 w-9 rounded border border-line-2 text-ink hover:border-muted">
          ×
        </button>
      </header>
      <div className="grid flex-1 gap-5 px-4 py-3.5 xl:grid-cols-[minmax(0,430px)_minmax(0,1fr)_220px]">
        <div className="flex flex-col gap-1.5">
          <Label>KWH/DÍA · 14 DÍAS · BANDA = BASELINE ±10%</Label>
          <DailyChart
            days={m.stats.days}
            color={v.color}
            window={ep ? [ep.onset_day, ep.end_day] : null}
            markers={eventMarkers(m.events, m.stats.days)}
            width={430}
            height={160}
          />
        </div>
        <div className="flex min-w-0 flex-col gap-2">
          <Label>{f ? `POR QUÉ · ${f.explained_by === 'claude' ? 'REDACTADO POR CLAUDE' : 'MOTOR DE ANÁLISIS'}` : 'REGLA'}</Label>
          <p className="m-0 text-[15px] text-white">{f?.reason ?? (m.status === 'OK' ? 'Consumo dentro de la banda del baseline.' : `Regla activada: ${m.status_reason}.`)}</p>
          {signals.slice(0, 4).map((s) => (
            <span key={s.code} className="flex gap-2 text-[13px] text-ink-2">
              <span className="font-mono" style={{ color: v.color }}>
                ▸
              </span>
              {s.description}
            </span>
          ))}
          {!f && m.status !== 'OK' && <span className="text-xs text-muted">Ejecuta el análisis IA para saber si es real, explicable o un problema de datos.</span>}
        </div>
        <div className="flex flex-col gap-2">
          {electricalRows(m.stats).map((e) => (
            <div key={e.label} className="flex items-baseline justify-between border-b border-line pb-1.5">
              <span className="font-mono text-[10px] text-muted">{e.label}</span>
              <span className="font-mono text-[15px] font-semibold">
                {e.value} <span className="text-[11px]" style={{ color: e.alarm ? v.color : 'var(--color-muted)' }}>{e.delta}</span>
              </span>
            </div>
          ))}
          {f && <NextActionButton id={f.id} type={f.type} status={f.status} size="md" className="mt-auto" colored />}
        </div>
      </div>
    </section>
  )
}
