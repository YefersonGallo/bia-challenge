import { useNavigate, useSearchParams } from 'react-router-dom'
import { useAnalysisRun, useAnomalies } from '@/shared/api/queries'
import type { AnomalyType, Severity } from '@/shared/api/types'
import { fmtConf, fmtTime } from '@/shared/lib/format'
import { confidenceLevel } from '@/shared/lib/series'
import { lifecycleLabels, priorityTag, SEVERITY_LABEL, TYPE_META } from '@/shared/lib/labels'
import { Button, EmptyState, ErrorBox, Label, Loading, Segmented } from '@/shared/ui/primitives'
import { useAnalysis } from '@/features/analysis/useAnalysis'

const COLS = '64px 90px 150px 90px 150px minmax(0,1fr) 120px 200px 24px'

const TYPE_OPTIONS: { value: AnomalyType | 'ALL'; label: string; dot?: string }[] = [
  { value: 'ALL', label: 'TODAS' },
  ...(Object.keys(TYPE_META) as AnomalyType[]).map((t) => ({ value: t, label: TYPE_META[t].short, dot: TYPE_META[t].color })),
]
const SEV_OPTIONS: { value: Severity | 'ALL'; label: string }[] = [
  { value: 'ALL', label: 'TODAS' },
  { value: 'HIGH', label: 'ALTA' },
  { value: 'MEDIUM', label: 'MEDIA' },
  { value: 'LOW', label: 'BAJA' },
]

/** AI anomalies ordered by priority, filterable by type and severity. */
export function AnomaliesPage() {
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const type = (params.get('type') as AnomalyType | null) ?? 'ALL'
  const severity = (params.get('severity') as Severity | null) ?? 'ALL'
  const { data = [], isLoading, error } = useAnomalies({
    type: type === 'ALL' ? undefined : type,
    severity: severity === 'ALL' ? undefined : severity,
  })
  const { data: run } = useAnalysisRun(null)
  const analysis = useAnalysis()

  const setParam = (k: string, v: string) =>
    setParams((p) => {
      if (v === 'ALL') p.delete(k)
      else p.set(k, v)
      return p
    })

  if (isLoading) return <Loading what="anomalías" />
  if (error) return <ErrorBox error={error} />

  if (!run || run.status !== 'COMPLETED') {
    return (
      <div className="flex flex-1 flex-col gap-4 px-6 py-5">
        <h1 className="m-0 font-display text-2xl font-semibold">Anomalías IA</h1>
        <EmptyState
          title="Todavía no hay veredictos."
          action={
            <Button variant="primary" onClick={analysis.start} disabled={analysis.running}>
              {analysis.running ? 'ANALIZANDO…' : 'RUN AI ANALYSIS'}
            </Button>
          }
        >
          Las reglas marcan los medidores fuera de rango. El análisis los clasifica como anomalía real, explicable, falso positivo o problema de datos,
          con confianza, evidencia y acción.
        </EmptyState>
      </div>
    )
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3 px-6 py-4">
      <div className="flex flex-col gap-1">
        <Label className="text-[11px]">
          ANÁLISIS #{run.id} · {fmtTime(run.started_at)} · CONFIANZA MEDIA {fmtConf(run.summary?.avg_confidence)}
        </Label>
        <h1 className="m-0 font-display text-2xl font-semibold tracking-[-0.01em]">{run.summary?.headline ?? 'Anomalías IA'}</h1>
      </div>
      <div className="flex flex-wrap items-center gap-4">
        <Segmented label="Filtrar por tipo" value={type} options={TYPE_OPTIONS} onChange={(v) => setParam('type', v)} />
        <span className="label">SEVERIDAD</span>
        <Segmented label="Filtrar por severidad" value={severity} options={SEV_OPTIONS} onChange={(v) => setParam('severity', v)} />
      </div>
      <div className="min-h-0 flex-1 overflow-auto rounded-md border border-line bg-panel-2">
        <table className="w-full min-w-[1150px] border-collapse">
          <thead>
            <tr className="grid h-[38px] items-center gap-3.5 border-b border-line px-4 text-left font-mono text-[10px] tracking-[0.08em] text-muted" style={{ gridTemplateColumns: COLS }}>
              <th className="font-normal">PRIOR.</th>
              <th className="font-normal">MEDIDOR</th>
              <th className="font-normal">TIPO</th>
              <th className="font-normal">SEVERIDAD</th>
              <th className="font-normal">CONFIANZA</th>
              <th className="font-normal">RAZÓN</th>
              <th className="font-normal">ESTADO</th>
              <th className="font-normal">ACCIÓN</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {data.map((a) => {
              const meta = TYPE_META[a.type]
              return (
                <tr
                  key={a.id}
                  onClick={() => navigate(`/anomalies/${a.id}`)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault()
                      navigate(`/anomalies/${a.id}`)
                    }
                  }}
                  tabIndex={0}
                  aria-label={`Abrir investigación de ${a.meter_id}`}
                  className="grid min-h-[58px] cursor-pointer items-center gap-3.5 border-b border-raise px-4 py-2 hover:bg-panel focus-visible:bg-panel focus-visible:outline-2 focus-visible:outline-accent"
                  style={{ gridTemplateColumns: COLS, opacity: a.status === 'RESOLVED' ? 0.55 : 1 }}
                >
                  <td>
                    <span className="rounded-[3px] px-2 py-0.5 font-mono text-xs font-semibold text-bg" style={{ background: meta.color }}>
                      {priorityTag(a.rank)}
                    </span>
                  </td>
                  <td className="font-mono text-sm font-semibold">{a.meter_id}</td>
                  <td className="text-[13px] font-semibold" style={{ color: meta.color }}>
                    {meta.label}
                  </td>
                  <td className="font-mono text-xs">{SEVERITY_LABEL[a.severity].toUpperCase()}</td>
                  <td className="flex items-center gap-2">
                    <span className="relative block h-1.5 w-[70px] rounded-sm bg-raise">
                      <span className="absolute inset-y-0 left-0 rounded-sm" style={{ width: `${a.confidence * 100}%`, background: meta.color }} />
                    </span>
                    <span className="flex flex-col leading-tight">
                      <span className="text-xs font-semibold">{confidenceLevel(a.confidence)}</span>
                      <span className="font-mono text-[10px] text-muted">{fmtConf(a.confidence)}</span>
                    </span>
                  </td>
                  <td className="line-clamp-2 text-[13px] text-ink-2">{a.reason}</td>
                  <td className="font-mono text-[11px] text-muted uppercase">{lifecycleLabels(a.type)[a.status]}</td>
                  <td className="text-[13px]">{a.recommended_action}</td>
                  <td className="text-right font-mono text-muted" aria-hidden>
                    →
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
        {data.length === 0 && <div className="px-4 py-8 text-sm text-soft">Ninguna anomalía coincide con los filtros.</div>}
      </div>
      <span className="font-mono text-[10px] tracking-[0.06em] text-muted">
        PRIORIDAD = SEVERIDAD × MAGNITUD × PERSISTENCIA × (1 − EXPLICADO POR EVENTO) · TIPO Y SEVERIDAD LOS DECIDE EL MOTOR; CLAUDE REDACTA LA EXPLICACIÓN
      </span>
    </div>
  )
}
