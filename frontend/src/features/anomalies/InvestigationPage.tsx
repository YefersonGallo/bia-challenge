import { useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useAnomalies, useAnomaly, useMeterEvents, useMeterSeries } from '@/shared/api/queries'
import type { AnomalyDetail } from '@/shared/api/types'
import { DailyChart } from '@/shared/charts/DailyChart'
import { HourlyChart } from '@/shared/charts/small'
import { fmtConf, fmtNum, fmtPct } from '@/shared/lib/format'
import { priorityTag, SEVERITY_LABEL, TYPE_META } from '@/shared/lib/labels'
import { eventMarkers } from '@/shared/lib/meterMath'
import { confidenceLevel } from '@/shared/lib/series'
import { Button, ErrorBox, Label, Loading, Panel, Segmented } from '@/shared/ui/primitives'
import { toAiOutput } from './aiOutput'
import { Diagnostics, ElectricalSeries, HourlySeries } from './Diagnostics'
import { ActionsPanel, ConfidenceBreakdown, EventTimeline, ImpactPanel } from './Insights'
import { LifecycleBar } from './Lifecycle'

type View = 'daily' | 'series' | 'hourly' | 'electrical'

function Comparison({ a, color, series }: { a: AnomalyDetail; color: string; series: ReturnType<typeof useMeterSeries> }) {
  const [view, setView] = useState<View>('daily')
  const ev = a.evidence
  const night = ev.night_ratio
  const { readings: rs, baseline: b, forecast: f } = series
  const pending = <span className="py-10 text-center font-mono text-[11px] text-muted">{series.loading ? 'Cargando lecturas…' : 'Lecturas no disponibles.'}</span>
  return (
    <Panel className="flex flex-col gap-2.5 px-4 py-3.5">
      <div className="flex items-center justify-between">
        <Label>COMPARACIÓN CONTRA BASELINE</Label>
        <Segmented
          label="Vista de la comparación"
          value={view}
          onChange={setView}
          options={[
            { value: 'daily', label: 'DIARIO' },
            { value: 'series', label: 'SERIE HORARIA' },
            { value: 'hourly', label: 'HORARIO' },
            { value: 'electrical', label: 'ELÉCTRICO' },
          ]}
        />
      </div>
      {view === 'daily' && (
        <DailyChart
          days={a.days}
          color={color}
          window={ev.onset_day ? [ev.onset_day, ev.end_day ?? a.days.length] : null}
          markers={eventMarkers(ev.related_events, a.days)}
          height={230}
        />
      )}
      {view === 'hourly' && (
        <>
          <HourlyChart baseline={a.hourly_baseline} current={a.hourly_current} color={color} width={940} height={230} />
          <span className="text-[13px] text-ink-2">
            {night >= 1.5
              ? `De noche consume ${fmtNum(night, 1)}× lo del baseline: la carga no se está apagando fuera de turno.`
              : 'El perfil horario conserva la forma del baseline; no aporta evidencia adicional para este caso.'}
          </span>
        </>
      )}
      {view === 'series' && (rs && b ? <HourlySeries a={a} color={color} rs={rs} b={b} f={f} /> : pending)}
      {view === 'electrical' && (rs && b ? <ElectricalSeries a={a} color={color} rs={rs} b={b} /> : pending)}
    </Panel>
  )
}

/** Investigation of one anomaly: what the AI found, the evidence behind it and the recommended action. */
export function InvestigationPage() {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const { data: a, isLoading, error } = useAnomaly(id)
  const { data: all = [] } = useAnomalies()
  const { data: meterEvents = [] } = useMeterEvents(a?.meter_id)
  const series = useMeterSeries(a?.meter_id ?? '')
  const [showJson, setShowJson] = useState(false)

  if (isLoading) return <Loading what="la investigación" />
  if (error || !a) return <ErrorBox error={error} />

  const meta = TYPE_META[a.type]
  const ev = a.evidence
  const idx = all.findIndex((x) => x.id === a.id)
  const go = (d: number) => all.length && navigate(`/anomalies/${all[(idx + d + all.length) % all.length].id}`)

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3.5 overflow-y-auto px-6 pt-3.5 pb-6">
      <div className="flex flex-wrap items-center gap-3">
        <Button onClick={() => navigate('/anomalies')} className="text-soft">
          ← ANOMALÍAS
        </Button>
        <span className="rounded-[3px] px-2.5 py-1 font-mono text-base font-semibold text-bg" style={{ background: meta.color }}>
          {priorityTag(a.rank)}
        </span>
        <h1 className="m-0 font-mono text-[28px] font-semibold tracking-[-0.02em]">{a.meter_id}</h1>
        <span className="flex flex-col">
          <span className="text-base font-semibold">{a.meter.name}</span>
          <span className="text-xs" style={{ color: meta.color }}>
            {meta.label} · severidad {SEVERITY_LABEL[a.severity].toLowerCase()} · confianza {confidenceLevel(a.confidence).toLowerCase()} ({fmtConf(a.confidence)}) · prioridad {a.rank} de {all.length || a.rank}
          </span>
        </span>
        <span className="ml-auto flex items-center gap-1.5">
          <Button size="sm" onClick={() => navigate(`/meters/${a.meter_id}`)}>
            VER MEDIDOR
          </Button>
          {idx >= 0 && (
            <>
              <button type="button" aria-label="Anomalía anterior" onClick={() => go(-1)} className="h-[30px] w-[30px] rounded border border-line-2 text-ink">
                ‹
              </button>
              <button type="button" aria-label="Anomalía siguiente" onClick={() => go(1)} className="h-[30px] w-[30px] rounded border border-line-2 text-ink">
                ›
              </button>
            </>
          )}
        </span>
      </div>

      <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_380px]">
        <div className="flex min-w-0 flex-col gap-3">
          <Panel accent={meta.color} className="flex flex-col gap-2 px-5 py-4">
            <Label>QUÉ ENCONTRÓ LA IA · {a.explained_by === 'claude' ? 'REDACTADO POR CLAUDE' : 'PLANTILLA DEL MOTOR'}</Label>
            <p className="m-0 font-display text-xl leading-snug font-medium text-white">{a.reason}</p>
            {a.evidence_summary && <p className="m-0 text-[13px] text-ink-2">Evidencia: {a.evidence_summary}</p>}
            {!a.anomaly && <span className="font-mono text-[11px] text-fp">anomaly: false · no requiere escalamiento</span>}
          </Panel>

          <Comparison a={a} color={meta.color} series={series} />

          <Panel className="flex flex-col gap-1 px-4 py-3.5">
            <Label>VARIABLES QUE CAMBIARON · BASELINE (DÍAS 1–7) → {ev.onset_day ? `DESDE EL DÍA ${ev.onset_day}` : 'ÚLTIMAS 24 H'}</Label>
            <table className="w-full border-collapse text-left">
              <tbody>
                {(ev.variables ?? []).map((v) => (
                  <tr key={v.name} className="border-b border-line last:border-0">
                    <td className="py-2 text-[13px] text-ink-2">{v.name}</td>
                    <td className="py-2 text-right font-mono text-[13px] text-muted">
                      {fmtNum(v.baseline, v.baseline < 10 ? 2 : 1)} {v.unit}
                    </td>
                    <td className="px-2 py-2 text-center font-mono text-muted">→</td>
                    <td className="py-2 font-mono text-[13px]">
                      {fmtNum(v.current, v.current < 10 ? 2 : 1)} {v.unit}
                    </td>
                    <td className="py-2 text-right font-mono text-[13px] font-semibold" style={{ color: Math.abs(v.delta_pct) >= 5 ? meta.color : 'var(--color-muted)' }}>
                      {fmtPct(v.delta_pct)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </Panel>

          {series.readings && series.baseline && <Diagnostics a={a} color={meta.color} rs={series.readings} b={series.baseline} />}
        </div>

        <div className="flex flex-col gap-3">
          <Panel className="flex flex-col gap-2.5 px-4 py-3.5">
            <Label>ACCIÓN RECOMENDADA</Label>
            <p className="m-0 text-base font-semibold">{a.recommended_action}</p>
            <ol className="m-0 flex list-decimal flex-col gap-1 pl-5 text-[13px] text-ink-2">
              {(a.next_steps ?? []).map((s) => (
                <li key={s}>{s}</li>
              ))}
            </ol>
            <LifecycleBar type={a.type} status={a.status} />
            <ActionsPanel a={a} />
          </Panel>

          <ImpactPanel a={a} color={meta.color} />
          <ConfidenceBreakdown a={a} />

          <Panel className="flex flex-col gap-2 px-4 py-3.5">
            <div className="flex items-center justify-between">
              <Label>EVIDENCIA · CONFIANZA {fmtConf(a.confidence)}</Label>
              <Button size="sm" variant="link" aria-pressed={showJson} onClick={() => setShowJson((s) => !s)}>
                {showJson ? 'VER EVIDENCIA' : 'VER JSON'}
              </Button>
            </div>
            {showJson ? (
              <pre className="m-0 overflow-x-auto rounded bg-bg p-3 font-mono text-[11px] leading-relaxed text-ink-2" aria-label="Salida IA en JSON">
                {JSON.stringify(toAiOutput(a), null, 2)}
              </pre>
            ) : (
              <ul className="m-0 flex list-none flex-col gap-1.5 p-0">
                {(ev.signals ?? []).map((s) => (
                  <li key={s.code} className="flex gap-2 text-[13px] text-ink-2">
                    <span className="font-mono" style={{ color: meta.color }}>
                      ▸
                    </span>
                    {s.description}
                  </li>
                ))}
                <li className="pt-1 font-mono text-[11px] text-muted">
                  {ev.persistent_hours} h de duración · relación kWh / V·I·PF coherente en el {fmtNum(ev.physical_coherence * 100)}% · {ev.invalid_readings} lecturas inconsistentes
                </li>
              </ul>
            )}
          </Panel>

          <EventTimeline a={a} events={meterEvents} />
        </div>
      </div>
    </div>
  )
}
