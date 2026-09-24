// Spanish (es-CO) number formatting, consistent with the texts the engine writes:
// thousands with ".", decimals with "," and a real minus sign.

const MINUS = '−'

function group(intPart: string): string {
  return intPart.replace(/\B(?=(\d{3})+(?!\d))/g, '.')
}

/** Formats a number with `decimals` fixed decimals: 2180.4 → "2.180", 0.81 → "0,81". */
export function fmtNum(n: number | null | undefined, decimals = 0): string {
  if (n == null || Number.isNaN(n)) return '—'
  const fixed = Math.abs(n).toFixed(decimals)
  const [i, d] = fixed.split('.')
  const sign = n < 0 && Number(fixed) !== 0 ? MINUS : ''
  return sign + group(i) + (d ? ',' + d : '')
}

/** Signed percentage with one decimal: 103.14 → "+103,1%", −38.9 → "−38,9%". */
export function fmtPct(p: number | null | undefined, decimals = 1): string {
  if (p == null || Number.isNaN(p)) return '—'
  const body = fmtNum(Math.abs(p), decimals)
  const sign = p > 0.05 ? '+' : p < -0.05 ? MINUS : ''
  return `${sign}${body}%`
}

/** Confidence in [0,1] with two decimals: 0.97 → "0,97". */
export const fmtConf = (c: number | null | undefined) => fmtNum(c, 2)

/** kWh with the unit: 2174.6 → "2.175 kWh". */
export const fmtKwh = (n: number | null | undefined) => `${fmtNum(n)} kWh`

/** Clock time HH:MM in UTC (the dataset and the runs are in UTC). */
export function fmtTime(iso: string | null | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toISOString().slice(11, 16)
}

/** Short date "24 sep · 18:45" in UTC. */
export function fmtDateTime(iso: string | null | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  const months = ['ene', 'feb', 'mar', 'abr', 'may', 'jun', 'jul', 'ago', 'sep', 'oct', 'nov', 'dic']
  return `${d.getUTCDate()} ${months[d.getUTCMonth()]} · ${fmtTime(iso)}`
}

/** First day of the dataset window (days are numbered D1…D14 from here). */
export const DATASET_START = '2026-09-01T00:00:00Z'

/** 1-based day of the dataset window for a timestamp. */
export function dayOf(iso: string, start = DATASET_START): number {
  const ms = new Date(iso).getTime() - new Date(start).getTime()
  return Math.floor(ms / 86_400_000) + 1
}

/** Day and hour of a timestamp in the dataset window: "D8 · 06:00". */
export const fmtDayHour = (iso: string, start = DATASET_START) => `D${dayOf(iso, start)} · ${fmtTime(iso)}`

/** Tone of a variation for coloring: big increases vs. big decreases vs. neutral. */
export function variationTone(p: number, threshold = 25): 'up' | 'down' | 'flat' {
  if (p >= threshold) return 'up'
  if (p <= -threshold) return 'down'
  return 'flat'
}
