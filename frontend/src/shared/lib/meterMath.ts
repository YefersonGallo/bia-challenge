import type { DayPoint, EventItem, MeterStats } from '@/shared/api/types'
import type { ChartMarker } from '@/shared/charts/DailyChart'
import { dayOf, fmtNum, fmtPct } from './format'
import { eventLabel } from './labels'

/** Start of the window of a daily series (falls back to the synthetic dataset start). */
export const windowStart = (days: DayPoint[]) => (days[0]?.date ? `${days[0].date.slice(0, 10)}T00:00:00Z` : undefined)

/** Chart markers for the operational events of a meter. */
export function eventMarkers(events: EventItem[] | null | undefined, days: DayPoint[]): ChartMarker[] {
  const start = windowStart(days)
  return (events ?? []).map((e) => ({ day: dayOf(e.timestamp, start), label: eventLabel(e.type).toUpperCase() }))
}

export interface ElectricalRow {
  label: string
  value: string
  delta: string
  /** The change is relevant (>5% or a power-factor drop). */
  alarm: boolean
}

/** Baseline → current comparison of voltage, current and power factor. */
export function electricalRows(stats: Pick<MeterStats, 'base_electrical' | 'current_electrical' | 'pf_out_of_range' | 'pf_jumps'>): ElectricalRow[] {
  const b = stats.base_electrical
  const c = stats.current_electrical
  const pct = (x: number, y: number) => (x ? ((y - x) / x) * 100 : 0)
  const dv = pct(b.voltage_v, c.voltage_v)
  const di = pct(b.current_a, c.current_a)
  const dpf = c.power_factor - b.power_factor
  return [
    { label: 'VOLTAJE', value: `${fmtNum(c.voltage_v)} V`, delta: fmtPct(dv), alarm: Math.abs(dv) > 5 },
    { label: 'CORRIENTE', value: `${fmtNum(c.current_a, 1)} A`, delta: fmtPct(di), alarm: Math.abs(di) > 25 },
    {
      label: 'FACTOR P.',
      value: fmtNum(c.power_factor, 2),
      delta:
        stats.pf_out_of_range > 0
          ? `${stats.pf_out_of_range} fuera de [0,1]`
          : stats.pf_jumps > 0
            ? `${stats.pf_jumps} saltos`
            : `${dpf < 0 ? '−' : '+'}${fmtNum(Math.abs(dpf), 2)}`,
      alarm: dpf <= -0.05 || stats.pf_out_of_range > 0 || stats.pf_jumps > 0,
    },
  ]
}
