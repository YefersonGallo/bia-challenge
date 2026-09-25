import { useNavigate, useParams } from 'react-router-dom'
import { useMeter, useMeters } from '@/shared/api/queries'
import type { MeterDetail } from '@/shared/api/types'
import { DailyChart } from '@/shared/charts/DailyChart'
import { HourlyChart } from '@/shared/charts/small'
import { fmtConf, fmtDayHour, fmtNum, fmtPct } from '@/shared/lib/format'
import { eventLabel, METER_STATUS_META, SEVERITY_LABEL, TYPE_META, zoneOf } from '@/shared/lib/labels'
import { electricalRows, eventMarkers } from '@/shared/lib/meterMath'
import { meterVerdict } from '@/shared/lib/verdict'
import { Button, ErrorBox, KpiStrip, Label, Loading, Panel, Tag } from '@/shared/ui/primitives'
import { useAnalysis } from '@/features/analysis/useAnalysis'
import { toMeterQuery, useMeterFilters } from './filtersStore'

/** Human summary of the kinds of suspicious readings of a meter. */
function qualityBreakdown(s: MeterDetail['stats']): string {
  const parts = [
    s.voltage_anomalies && `${s.voltage_anomalies} con voltaje anómalo`,
    s.pf_jumps && `${s.pf_jumps} saltos de factor de potencia`,
    s.incoherent_readings && `${s.incoherent_readings} donde kWh no cuadra con V·I·PF`,
    s.pf_out_of_range && `${s.pf_out_of_range} con FP fuera de [0,1]`,
    s.zero_voltage && `${s.zero_voltage} con 0 V`,
  ].filter(Boolean)
  return parts.join(' · ')
}

/** Meter detail: KPIs, daily and hourly charts, AI verdict, events and data quality. */
export function MeterDetailPage() {
  const { meterId = '' } = useParams()
  const navigate = useNavigate()
  const filters = useMeterFilters()
  const { data: list = [] } = useMeters(toMeterQuery(filters))
  const { data: m, isLoading, error } = useMeter(meterId)
  const analysis = useAnalysis()

  if (isLoading) return <Loading what={meterId} />
  if (error || !m) return <ErrorBox error={error} />

  const v = meterVerdict(m)
  const st = METER_STATUS_META[m.status]
  const f = m.finding
  const ep = m.stats.episode
  const idx = list.findIndex((x) => x.id === m.id)
  const go = (d: number) => list.length && navigate(`/meters/${list[(idx + d + list.length) % list.length].id}`)
  const coherence = m.stats.physical_coherence

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3.5 overflow-y-auto px-6 pt-3.5 pb-6">
      <div className="flex flex-wrap items-center gap-3.5">
        <Button onClick={() => navigate('/meters')} className="text-soft">
          ← MEDIDORES
        </Button>
        <h1 className="m-0 font-mono text-3xl font-semibold tracking-[-0.02em]">{m.id}</h1>
        <span className="flex flex-col">
          <span className="text-base font-semibold">{m.name}</span>
          <span className="text-xs text-muted">
            {m.location === zoneOf(m.location) ? m.location : `${m.location} · ${zoneOf(m.location)}`}
          </span>
        </span>
        <Tag color={st.color} outline>
          {st.label.toUpperCase()}
        </Tag>
        {v.classified && <Tag color={v.color}>{v.text}</Tag>}
        {idx >= 0 && (
          <span className="ml-auto flex items-center gap-1.5">
            <button type="button" aria-label="Medidor anterior" onClick={() => go(-1)} className="h-[34px] w-[34px] rounded border border-line-2 text-ink">
              ‹
            </button>
            <span className="font-mono text-[11px] text-muted">
              {idx + 1} / {list.length}
            </span>
            <button type="button" aria-label="Medidor siguiente" onClick={() => go(1)} className="h-[34px] w-[34px] rounded border border-line-2 text-ink">
              ›
            </button>
          </span>
        )}
      </div>

      <KpiStrip
        items={[
          { label: 'KWH ÚLTIMAS 24 H', value: fmtNum(m.current_kwh), sub: `baseline ${fmtNum(m.baseline_kwh)} kWh/día · periodo ${fmtNum(m.period_kwh)} kWh` },
          { label: 'VARIACIÓN', value: fmtPct(m.variation_pct), color: Math.abs(m.variation_pct) >= 25 ? v.color : undefined, sub: m.status_reason },
          { label: 'CONSUMO NOCTURNO', value: `${fmtNum(m.stats.night_ratio, 1)}×`, sub: 'últimas 24 h frente al baseline (22–06 h)' },
          {
            label: 'LECTURAS INCONSISTENTES',
            value: fmtNum(m.invalid_readings),
            color: m.invalid_readings > 0 ? 'var(--color-data)' : undefined,
            sub: qualityBreakdown(m.stats) || 'ninguna',
          },
          {
            label: 'COHERENCIA FÍSICA',
            value: `${fmtNum(coherence * 100)}%`,
            color: coherence < 0.9 ? 'var(--color-data)' : undefined,
            sub: 'relación kWh / V·I·PF estable',
          },
        ]}
      />

      <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_380px]">
        <div className="flex min-w-0 flex-col gap-3">
          <Panel className="flex flex-col gap-2 px-4 py-3.5">
            <span className="flex justify-between">
              <Label>CONSUMO DIARIO VS BASELINE · KWH · 14 DÍAS</Label>
              <Label>BANDA = BASELINE ±10%</Label>
            </span>
            <DailyChart days={m.stats.days} color={v.color} window={ep ? [ep.onset_day, ep.end_day] : null} markers={eventMarkers(m.events, m.stats.days)} />
          </Panel>
          <div className="grid gap-3 lg:grid-cols-2">
            <Panel className="flex flex-col gap-2 px-4 py-3.5">
              <Label>PERFIL HORARIO · KWH/H · BASELINE (--) VS ÚLTIMAS 24 H</Label>
              <HourlyChart baseline={m.stats.hourly_baseline} current={m.stats.hourly_current} color={v.color} />
            </Panel>
            <Panel className="flex flex-col gap-2 px-4 py-3.5">
              <Label>VARIABLES ELÉCTRICAS · DÍAS 1–7 → {ep ? 'VENTANA DEL CAMBIO' : 'ÚLTIMAS 24 H'}</Label>
              {electricalRows(m.stats).map((e) => (
                <div key={e.label} className="flex items-baseline justify-between border-b border-line pb-2">
                  <span className="font-mono text-[10px] text-muted">{e.label}</span>
                  <span className="font-mono text-base font-semibold">
                    {e.value} <span className="text-xs" style={{ color: e.alarm ? v.color : 'var(--color-muted)' }}>{e.delta}</span>
                  </span>
                </div>
              ))}
            </Panel>
          </div>
        </div>

        <div className="flex flex-col gap-3">
          <Panel accent={f ? TYPE_META[f.type].color : undefined} className="flex flex-col gap-2.5 px-4 py-3.5">
            <Label>VEREDICTO IA</Label>
            {f ? (
              <>
                <span className="flex flex-wrap items-center gap-2">
                  <Tag color={TYPE_META[f.type].color}>{TYPE_META[f.type].label.toUpperCase()}</Tag>
                  <span className="font-mono text-[11px] text-muted">
                    severidad {SEVERITY_LABEL[f.severity].toLowerCase()} · conf {fmtConf(f.confidence)}
                  </span>
                </span>
                <p className="m-0 text-sm text-ink">{f.reason}</p>
                <Label>ACCIÓN RECOMENDADA</Label>
                <p className="m-0 text-sm font-semibold">{f.recommended_action}</p>
                <Button variant="primary" onClick={() => navigate(`/anomalies/${f.id}`)}>
                  ABRIR INVESTIGACIÓN →
                </Button>
              </>
            ) : (
              <>
                <p className="m-0 text-sm text-soft">
                  {m.status === 'OK'
                    ? 'Sin anomalías: el consumo se mantiene en la banda del baseline.'
                    : 'Las reglas marcan este medidor, pero aún no hay veredicto de la IA.'}
                </p>
                {m.status !== 'OK' && (
                  <Button variant="primary" onClick={analysis.start} disabled={analysis.running}>
                    RUN AI ANALYSIS
                  </Button>
                )}
              </>
            )}
          </Panel>

          <Panel className="flex flex-col gap-2 px-4 py-3.5">
            <Label>EVENTOS OPERATIVOS</Label>
            {m.events.length === 0 && <span className="text-[13px] text-muted">Ninguno registrado en los 14 días.</span>}
            {m.events.map((e) => (
              <div key={e.id} className="flex flex-col gap-0.5 rounded border border-line bg-panel-2 px-3 py-2">
                <span className="font-mono text-[11px] text-expl">
                  {fmtDayHour(e.timestamp)} · {eventLabel(e.type).toUpperCase()}
                </span>
                <span className="text-[13px]">{e.description}</span>
              </div>
            ))}
          </Panel>

          <Panel className="flex flex-col gap-1.5 px-4 py-3.5">
            <Label>CALIDAD DE DATOS</Label>
            <span className="text-[13px] text-ink-2">
              {m.invalid_readings === 0
                ? 'Todas las lecturas son físicamente posibles.'
                : `${m.invalid_readings} lecturas físicamente inconsistentes${m.stats.issue_onset ? ` desde ${fmtDayHour(m.stats.issue_onset)}` : ''}: ${qualityBreakdown(m.stats)}.`}
            </span>
            <span className="font-mono text-[11px] text-muted">relación kWh / V·I·PF coherente en el {fmtNum(coherence * 100)}% de las lecturas</span>
          </Panel>
        </div>
      </div>
    </div>
  )
}
