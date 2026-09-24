import { useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useAnomalies, useAnomaly } from '@/shared/api/queries'
import type { AnomalyDetail } from '@/shared/api/types'
import { DailyChart } from '@/shared/charts/DailyChart'
import { HourlyChart } from '@/shared/charts/small'
import { fmtConf, fmtDayHour, fmtNum, fmtPct } from '@/shared/lib/format'
import { eventLabel, priorityTag, SEVERITY_LABEL, TYPE_META } from '@/shared/lib/labels'
import { eventMarkers } from '@/shared/lib/meterMath'
import { Button, ErrorBox, Label, Loading, Panel, Segmented } from '@/shared/ui/primitives'
import { toAiOutput } from './aiOutput'
import { LifecycleBar, NextActionButton } from './Lifecycle'

type View = 'daily' | 'hourly' | 'electrical'

function Comparison({ a, color }: { a: AnomalyDetail; color: string }) {
  const [view, setView] = useState<View>('daily')
  const ev = a.evidence
  const night = ev.night_ratio
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
      {view === 'electrical' && (
        <div className="grid grid-cols-3 gap-3">
          {(['voltage_v', 'current_a', 'power_factor'] as const).map((k) => {
            const label = { voltage_v: 'VOLTAJE · V', current_a: 'CORRIENTE · A', power_factor: 'FACTOR DE POTENCIA' }[k]
            const dec = k === 'power_factor' ? 2 : 1
            return (
              <div key={k} className="flex flex-col gap-1 rounded border border-line bg-panel-2 p-3">
                <Label>{label}</Label>
                <div className="flex h-24 items-end gap-[3px]" aria-label={`${label} por día`}>
                  {a.days.map((d) => {
                    const max = Math.max(...a.days.map((x) => x[k])) || 1
                    const inWin = ev.onset_day != null && d.day >= ev.onset_day
                    return (
                      <span
                        key={d.day}
                        title={`D${d.day}: ${fmtNum(d[k], dec)}`}
                        className="flex-1 rounded-t-sm"
                        style={{ height: `${(d[k] / max) * 100}%`, background: inWin ? color : 'var(--color-line-2)' }}
                      />
                    )
                  })}
                </div>
              </div>
            )
          })}
        </div>
      )}
    </Panel>
  )
}

/** Investigation of one anomaly: what the AI found, the evidence behind it and the recommended action. */
export function InvestigationPage() {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const { data: a, isLoading, error } = useAnomaly(id)
  const { data: all = [] } = useAnomalies()
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
            {meta.label} · severidad {SEVERITY_LABEL[a.severity].toLowerCase()} · confianza {fmtConf(a.confidence)} · prioridad {a.rank} de {all.length || a.rank}
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
            {!a.anomaly && <span className="font-mono text-[11px] text-fp">anomaly: false · no requiere escalamiento</span>}
          </Panel>

          <Comparison a={a} color={meta.color} />

          <Panel className="flex flex-col gap-1 px-4 py-3.5">
            <Label>VARIABLES QUE CAMBIARON · DÍAS 1–7 → DÍAS 8–14</Label>
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
            <NextActionButton id={a.id} type={a.type} status={a.status} size="md" />
          </Panel>

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
                  {ev.persistent_hours} h fuera de banda · coherencia física {fmtNum(ev.physical_coherence * 100)}% · {ev.invalid_readings} lecturas inválidas
                </li>
              </ul>
            )}
          </Panel>

          <Panel className="flex flex-col gap-2 px-4 py-3.5">
            <Label>EVENTOS RELACIONADOS</Label>
            {(ev.related_events ?? []).length === 0 ? (
              <span className="text-[13px] text-muted">Ninguno en ±24 h del cambio. Por eso no se puede explicar por la operación.</span>
            ) : (
              (ev.related_events ?? []).map((e) => (
                <div key={e.id} className="flex flex-col gap-0.5 rounded border border-line bg-panel-2 px-3 py-2">
                  <span className="font-mono text-[11px] text-expl">
                    {fmtDayHour(e.timestamp)} · {eventLabel(e.type).toUpperCase()}
                  </span>
                  <span className="text-[13px]">{e.description}</span>
                  <span className="font-mono text-[10px] text-muted">
                    {ev.event_explains_shift ? '✓ coherente con la dirección del cambio' : '✗ no explica la dirección del cambio'}
                  </span>
                </div>
              ))
            )}
          </Panel>
        </div>
      </div>
    </div>
  )
}
