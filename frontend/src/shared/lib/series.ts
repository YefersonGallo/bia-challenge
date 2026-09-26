// Pure derivations over hourly readings for the investigation charts.
// The engine does the classification; these only reshape data for display.
import type { Anomaly, Baseline, EventItem, FlaggedReading, Forecast, Reading } from '@/shared/api/types'

export type Pt = [t: number, v: number]
export type BandPt = [t: number, lo: number, hi: number]

const ms = (iso: string) => new Date(iso).getTime()
const hourOf = (iso: string) => new Date(iso).getUTCHours()

/** kWh / (V·I·PF / 1000): the physical relation of a reading (≈1 for an hourly meter). */
export function kOf(r: Reading): number | null {
  const p = (r.voltage_v * r.current_a * r.power_factor) / 1000
  return p > 0 ? r.consumption_kwh / p : null
}

/** One series per variable of the readings, as [time, value] points. */
export function variableSeries(rs: Reading[], key: 'consumption_kwh' | 'voltage_v' | 'current_a' | 'power_factor'): Pt[] {
  return rs.map((r) => [ms(r.timestamp), r[key]])
}

/** The p10–p90 band of the baseline repeated over every reading (hour-of-day profile). */
export function baselineBand(rs: Reading[], b: Pick<Baseline, 'p10' | 'p90'>): BandPt[] {
  return rs.map((r) => {
    const h = hourOf(r.timestamp)
    return [ms(r.timestamp), b.p10[h], b.p90[h]]
  })
}

/** k over time; readings without power are skipped. */
export function kSeries(rs: Reading[]): Pt[] {
  const out: Pt[] = []
  for (const r of rs) {
    const k = kOf(r)
    if (k != null && Number.isFinite(k)) out.push([ms(r.timestamp), k])
  }
  return out
}

/** Set of flagged timestamps (ms) for quick lookup. */
export function flaggedSet(flagged: FlaggedReading[] | null | undefined): Set<number> {
  return new Set((flagged ?? []).map((f) => ms(f.timestamp)))
}

/** Cumulative measured energy vs. what the baseline profile would have used, hour by hour. */
export function cumulativeEnergy(rs: Reading[], median: number[]): { real: Pt[]; expected: Pt[] } {
  let real = 0
  let exp = 0
  const r: Pt[] = []
  const e: Pt[] = []
  for (const x of rs) {
    real += x.consumption_kwh
    exp += median[hourOf(x.timestamp)] ?? 0
    const t = ms(x.timestamp)
    r.push([t, real])
    e.push([t, exp])
  }
  return { real: r, expected: e }
}

/**
 * Current vs. kWh split at the change point: reference readings and those of the change.
 * When the episode ended (a recovered outage), only its window counts as "change".
 */
export function scatterSplit(rs: Reading[], changeAt: string | null | undefined, endedAt?: string | null): { before: Pt[]; after: Pt[] } {
  const cut = changeAt ? ms(changeAt) : Number.POSITIVE_INFINITY
  const end = endedAt ? ms(endedAt) : Number.POSITIVE_INFINITY
  const before: Pt[] = []
  const after: Pt[] = []
  for (const r of rs) {
    const t = ms(r.timestamp)
    ;(t >= cut && t <= end ? after : before).push([r.current_a, r.consumption_kwh])
  }
  return { before, after }
}

/** Forecast as chart series: projected line and the expected p10–p90 band. */
export function forecastSeries(f: Forecast): { projected: Pt[]; band: BandPt[] } {
  return {
    projected: f.points.map((p) => [ms(p.timestamp), p.projected_kwh]),
    band: f.points.map((p) => [ms(p.timestamp), p.p10, p.p90]),
  }
}

/** Confidence in words for the tables: ≥ 0.8 alta, ≥ 0.6 media, otherwise baja. */
export function confidenceLevel(c: number): 'Alta' | 'Media' | 'Baja' {
  if (c >= 0.8) return 'Alta'
  if (c >= 0.6) return 'Media'
  return 'Baja'
}

/** Diverging color for a deviation (%): blue below the baseline, red above, neutral near 0. */
export function deviationColor(pct: number, limit = 100): string {
  const f = Math.max(-1, Math.min(1, pct / limit))
  if (Math.abs(pct) < 5) return 'var(--color-raise)'
  const alpha = 0.18 + 0.72 * Math.abs(f)
  return f > 0 ? `rgba(255, 77, 61, ${alpha.toFixed(2)})` : `rgba(79, 179, 255, ${alpha.toFixed(2)})`
}

export interface EventVerdict {
  /** true: explains the change · false: evaluated and rejected · null: informational only. */
  explains: boolean | null
  text: string
}

/**
 * Why an event does or does not explain the anomaly, mirroring the engine's rules
 * (category, ±window around the change, direction and, for outages, duration).
 */
export function eventVerdict(
  e: EventItem,
  a: Pick<Anomaly, 'change_point_at' | 'ended_at' | 'evidence'>,
  windowHours = 3,
  durationTol = 2,
): EventVerdict {
  if (e.category === 'NON_EXPLANATORY') return { explains: false, text: `tipo ${e.type}: no describe una causa operativa, no explica el cambio` }
  if (e.category === 'INFORMATIONAL') return { explains: null, text: 'registro de calidad de datos: corrobora el diagnóstico, no lo decide' }
  const at = a.change_point_at ?? a.evidence.onset
  if (!at) return { explains: false, text: 'no hay un cambio sostenido con el que compararlo' }
  const gap = Math.abs(ms(e.timestamp) - ms(at)) / 3_600_000
  if (gap > windowHours) return { explains: false, text: `a ${Math.round(gap)} h del cambio: fuera de la ventana de ±${windowHours} h` }
  const dir = a.evidence.shift_pct >= 0 ? 'UP' : 'DOWN'
  if (e.expected_effect !== 'NONE' && e.expected_effect !== dir) {
    return { explains: false, text: `su efecto esperado (${e.expected_effect === 'UP' ? 'subir' : 'bajar'}) no explica la dirección del cambio` }
  }
  if (e.duration_hours && a.ended_at) {
    const lasted = (ms(a.ended_at) - ms(at)) / 3_600_000
    if (Math.abs(lasted - e.duration_hours) > durationTol) {
      return { explains: false, text: `duró ${Math.round(lasted)} h y el evento anuncia ${e.duration_hours} h` }
    }
    return { explains: true, text: `en la ventana, misma dirección y duración coherente (${Math.round(lasted)} h de ${e.duration_hours} h)` }
  }
  return { explains: a.evidence.event_explains_shift, text: a.evidence.event_explains_shift ? 'en la ventana y con la misma dirección del cambio' : 'no explica la dirección del cambio' }
}
