import { Link } from 'react-router-dom'
import { useAnalysisRun, useAnomalies } from '@/shared/api/queries'
import type { Anomaly } from '@/shared/api/types'
import { fmtConf, fmtTime } from '@/shared/lib/format'
import { priorityTag, SEVERITY_LABEL, TYPE_META } from '@/shared/lib/labels'
import { Button, EmptyState, Label } from '@/shared/ui/primitives'
import { LifecycleBar, NextActionButton } from '@/features/anomalies/Lifecycle'

function AlarmCard({ a, onFocus }: { a: Anomaly; onFocus: (meterId: string) => void }) {
  const meta = TYPE_META[a.type]
  const closed = a.status === 'RESOLVED'
  const p1 = a.rank === 1 && a.type === 'REAL_ANOMALY' && !closed
  return (
    <article
      aria-label={`Alarma ${a.meter_id}`}
      className="flex flex-col gap-1.5 rounded-md border px-3 py-2.5"
      style={{
        background: p1 ? '#1a0d0b' : 'var(--color-panel)',
        borderColor: closed ? 'var(--color-line)' : meta.color,
        opacity: closed ? 0.6 : 1,
      }}
    >
      <div className="flex items-center gap-2">
        <span className="rounded-[3px] px-[7px] py-0.5 font-mono text-[11px] font-semibold text-bg" style={{ background: meta.color }}>
          {priorityTag(a.rank)}
        </span>
        <span className="font-mono text-sm font-semibold">{a.meter_id}</span>
        <span className="text-xs font-semibold" style={{ color: meta.color }}>
          {meta.label}
        </span>
        <span className="ml-auto font-mono text-[11px] text-muted">conf {fmtConf(a.confidence)}</span>
      </div>
      <p className="m-0 line-clamp-2 text-xs text-ink-2">{a.reason}</p>
      <LifecycleBar type={a.type} status={a.status} />
      <div className="flex gap-1.5">
        <Button size="sm" onClick={() => onFocus(a.meter_id)} className="font-sans tracking-normal">
          Ver foco
        </Button>
        <NextActionButton id={a.id} type={a.type} status={a.status} className="flex-1 font-sans tracking-normal" />
      </div>
    </article>
  )
}

/** Right column of the operations view: AI alarms ordered by priority, report shortcut and run log. */
export function AlarmStack({ onFocus }: { onFocus: (meterId: string) => void }) {
  const { data: anomalies = [] } = useAnomalies()
  const { data: run } = useAnalysisRun(null)
  const counts = (['HIGH', 'MEDIUM', 'LOW'] as const).map((s) => ({ s, n: anomalies.filter((a) => a.severity === s).length }))

  return (
    <aside aria-label="Alarmas IA" className="flex min-h-0 flex-col gap-2.5 overflow-y-auto">
      <div className="flex items-center justify-between">
        <Label className="text-[11px]">ALARMAS IA</Label>
        <span className="flex gap-1.5 font-mono text-[10px]">
          {counts.map(({ s, n }) => (
            <span key={s} className="rounded-[3px] border border-line-2 px-1.5 py-0.5 text-ink-2">
              {n} {SEVERITY_LABEL[s].toUpperCase()}
            </span>
          ))}
        </span>
      </div>

      {anomalies.length > 0 && (
        <Link to="/report" className="flex items-center gap-2.5 rounded-md border border-[#1f3a55] bg-[#0f1a26] px-3 py-2.5 text-ink no-underline">
          <svg width="18" height="18" viewBox="0 0 24 24" aria-hidden className="shrink-0 fill-none stroke-accent" strokeWidth={1.8} strokeLinejoin="round">
            <path d="M6 3h9l4 4v14H6z" />
            <path d="M9 12h7M9 16h7M9 8h3" />
          </svg>
          <span className="flex flex-1 flex-col gap-px">
            <span className="text-[13px] font-semibold">Reporte del análisis listo</span>
            <span className="font-mono text-[10px] text-muted">6 PREGUNTAS · {anomalies.length} FICHAS · PLAN DE ACCIÓN</span>
          </span>
          <span className="font-mono text-xs text-accent">ABRIR →</span>
        </Link>
      )}

      {anomalies.length === 0 && (
        <EmptyState title="Todavía no hay veredictos de la IA.">
          Las reglas marcan los medidores fuera de su banda, pero no dicen qué hacer con ellos. Ejecuta Run AI Analysis.
        </EmptyState>
      )}

      {anomalies.map((a) => (
        <AlarmCard key={a.id} a={a} onFocus={onFocus} />
      ))}

      {run && (
        <div className="mt-auto flex flex-col gap-[3px] rounded-md border border-line bg-panel-2 px-3 py-2.5 font-mono text-[11px] text-muted">
          <span className="tracking-[0.1em]">
            REGISTRO · {run.id} · {fmtTime(run.started_at)}
          </span>
          {run.steps
            .filter((s) => s.result)
            .map((s) => (
              <span key={s.key}>
                <span className="text-ink">{s.label}</span> {s.result}
              </span>
            ))}
        </div>
      )}
    </aside>
  )
}
