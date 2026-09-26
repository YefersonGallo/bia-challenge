import { useMemo } from 'react'
import type { useForecast } from '@/shared/api/queries'
import type { AnomalyDetail, Baseline, Reading } from '@/shared/api/types'
import { ScatterChart } from '@/shared/charts/ScatterChart'
import { TimeSeriesChart, type Series, type TimeMarker } from '@/shared/charts/TimeSeriesChart'
import { fmtNum, fmtPct } from '@/shared/lib/format'
import { eventLabel } from '@/shared/lib/labels'
import { type Pt, baselineBand, cumulativeEnergy, flaggedSet, forecastSeries, kSeries, scatterSplit, variableSeries } from '@/shared/lib/series'
import { Label, Panel } from '@/shared/ui/primitives'

const ms = (iso?: string | null) => (iso ? new Date(iso).getTime() : null)

/** Before the change in neutral, from the change on in the anomaly color. */
function splitSeries(key: string, points: Pt[], cp: number | null, color: string, width = 1.2): Series[] {
  if (cp == null) return [{ key, points, color, width }]
  const i = points.findIndex(([t]) => t >= cp)
  if (i <= 0) return [{ key, points, color, width }]
  return [
    { key: `${key}-before`, points: points.slice(0, i + 1), color: 'var(--color-soft)', width },
    { key: `${key}-after`, points: points.slice(i), color, width: width + 0.3 },
  ]
}

function eventMarkers(a: AnomalyDetail): TimeMarker[] {
  return (a.evidence.related_events ?? []).map((e) => ({ t: new Date(e.timestamp).getTime(), label: eventLabel(e.type) }))
}

/** 14 days hour by hour against the p10–p90 band of the baseline, plus the 24 h projection. */
export function HourlySeries({ a, color, rs, b, f }: { a: AnomalyDetail; color: string; rs: Reading[]; b: Baseline; f?: ReturnType<typeof useForecast>['data'] }) {
  const fc = f ? forecastSeries(f) : null
  const band = [...baselineBand(rs, b), ...(fc?.band ?? [])]
  const last = rs.length ? new Date(rs[rs.length - 1].timestamp).getTime() : 0
  return (
    <div className="flex flex-col gap-1.5">
      <TimeSeriesChart
        title="Consumo horario frente a la banda p10–p90 del baseline"
        series={[
          ...splitSeries('kwh', variableSeries(rs, 'consumption_kwh'), ms(a.change_point_at), color, 1.2),
          ...(fc ? [{ key: 'proj', points: fc.projected, color: 'var(--color-accent)', width: 1.6, dashed: true, label: 'proyección 24 h' }] : []),
        ]}
        band={band}
        bandColor="rgba(174, 184, 196, 0.16)"
        changePoint={ms(a.change_point_at)}
        markers={eventMarkers(a)}
        future={fc && fc.projected.length ? [last, fc.projected[fc.projected.length - 1][0]] : null}
        decimals={1}
        zero
        height={240}
      />
      <span className="text-[12px] text-muted">
        Banda gris: p10–p90 de cada hora en los días 1–7. Línea vertical: inicio del cambio.
        {f && ` Punteado: próximas 24 h, ${fmtNum(f.projected_kwh)} kWh frente a ${fmtNum(f.expected_kwh)} esperados (${f.method}).`}
      </span>
    </div>
  )
}

/** Voltage, current and power factor over time with their reference bands. */
export function ElectricalSeries({ a, color, rs, b }: { a: AnomalyDetail; color: string; rs: Reading[]; b: Baseline }) {
  const flagged = flaggedSet(a.flagged_readings ?? b.flagged_readings)
  const cp = ms(a.change_point_at)
  return (
    <div className="grid gap-3">
      {(
        [
          { key: 'voltage_v', label: 'VOLTAJE · V (BANDA 209–231 V)', refBand: b.voltage_band, dec: 0 },
          { key: 'current_a', label: 'CORRIENTE · A', dec: 0 },
          { key: 'power_factor', label: 'FACTOR DE POTENCIA (REFERENCIA 0,9)', refLine: { value: 0.9, label: '0,90' }, dec: 2 },
        ] as const
      ).map((v) => (
        <div key={v.key} className="flex flex-col gap-1">
          <Label>{v.label}</Label>
          <TimeSeriesChart
            title={v.label}
            series={splitSeries(v.key, variableSeries(rs, v.key), cp, color, 1.1)}
            refBand={'refBand' in v ? (v.refBand as [number, number]) : undefined}
            refLine={'refLine' in v ? v.refLine : undefined}
            changePoint={cp}
            highlight={flagged}
            decimals={v.dec}
            height={130}
          />
        </div>
      ))}
      {flagged.size > 0 && <span className="font-mono text-[10px] text-data">● {flagged.size} lecturas marcadas por las reglas de calidad de datos</span>}
    </div>
  )
}

/** Diagnostic charts: physical relation k, current vs. kWh before/after, cumulative energy. */
export function Diagnostics({ a, color, rs, b }: { a: AnomalyDetail; color: string; rs: Reading[]; b: Baseline }) {
  const k = useMemo(() => kSeries(rs), [rs])
  const flagged = flaggedSet(a.flagged_readings ?? b.flagged_readings)
  const scatter = useMemo(() => scatterSplit(rs, a.change_point_at, a.ended_at), [rs, a.change_point_at, a.ended_at])
  const cum = useMemo(() => cumulativeEnergy(rs, b.median), [rs, b.median])
  const kf = a.k_factor || b.k_factor
  const tol = b.k_tolerance
  // Mean k after the change against the usual one: a sustained shift is a change of regime, not bad data.
  const cp = ms(a.change_point_at)
  const after = cp == null ? [] : k.filter(([t]) => t >= cp).map(([, v]) => v)
  const kShift = after.length && kf ? (after.reduce((x, y) => x + y, 0) / after.length / kf - 1) * 100 : 0
  const real = cum.real.at(-1)?.[1] ?? 0
  const exp = cum.expected.at(-1)?.[1] ?? 0
  return (
    <Panel className="flex flex-col gap-3 px-4 py-3.5">
      <Label>DIAGNÓSTICO</Label>
      <div className="flex flex-col gap-1">
        <Label>RELACIÓN FÍSICA k = kWh / (V·I·PF/1000) · BANDA ±{fmtNum(tol * 100)}%</Label>
        <TimeSeriesChart
          title="Relación física k a lo largo del tiempo"
          series={[{ key: 'k', points: k, color: flagged.size ? 'var(--color-data)' : 'var(--color-soft)', width: 1.1 }]}
          refBand={[kf * (1 - tol), kf * (1 + tol)]}
          highlight={flagged}
          highlightColor="var(--color-real)"
          changePoint={ms(a.change_point_at)}
          decimals={2}
          height={150}
        />
        <span className="text-[12px] text-muted">
          k habitual {fmtNum(kf, 2)} (MAD {fmtNum(a.k_mad || b.k_mad, 3)}).{' '}
          {flagged.size > 0
            ? 'Los puntos rojos salen de la banda: el consumo no cuadra con V·I·PF, el problema está en la medición.'
            : Math.abs(kShift) >= 10
              ? `Tras el cambio k se desplaza ${fmtPct(kShift)} de forma sostenida: el equipo cambió de régimen (no son lecturas aisladas).`
              : 'k estable: la energía medida es coherente con voltaje, corriente y FP.'}
        </span>
      </div>
      <div className="grid gap-3 lg:grid-cols-2">
        <div className="flex flex-col gap-1">
          <Label>CORRIENTE VS. CONSUMO · {a.ended_at ? 'FUERA / DURANTE EL EPISODIO' : 'ANTES / DESPUÉS DEL CAMBIO'}</Label>
          <ScatterChart
            title="Corriente frente a consumo, antes y después del cambio"
            xLabel="A"
            yLabel="kWh"
            groups={[
              { key: 'before', label: a.ended_at ? 'fuera del episodio' : 'antes', points: scatter.before, color: 'var(--color-dim)' },
              { key: 'after', label: a.ended_at ? 'durante' : 'después', points: scatter.after, color: a.type === 'FALSE_POSITIVE' ? 'var(--color-expl)' : color },
            ]}
          />
        </div>
        <div className="flex flex-col gap-1">
          <Label>ENERGÍA ACUMULADA · REAL VS. ESPERADA</Label>
          <TimeSeriesChart
            title="Energía acumulada real frente a la esperada"
            series={[
              { key: 'exp', points: cum.expected, color: 'var(--color-dim)', dashed: true, label: 'esperada' },
              { key: 'real', points: cum.real, color, width: 1.8, label: 'real' },
            ]}
            changePoint={ms(a.change_point_at)}
            zero
            width={460}
            height={220}
          />
          <span className="font-mono text-[11px] text-muted">
            {fmtNum(real)} kWh reales vs. {fmtNum(exp)} esperados ({fmtPct(exp ? ((real - exp) / exp) * 100 : 0)})
          </span>
        </div>
      </div>
    </Panel>
  )
}
