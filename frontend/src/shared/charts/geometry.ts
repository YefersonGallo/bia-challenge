// Pure SVG geometry helpers. Kept free of React so they are easy to unit-test.

export interface Scale {
  (v: number): number
  domain: [number, number]
  range: [number, number]
}

/** Linear scale; a degenerate domain maps everything to the middle of the range. */
export function linear(domain: [number, number], range: [number, number]): Scale {
  const [d0, d1] = domain
  const [r0, r1] = range
  const f = ((v: number) => (d1 === d0 ? (r0 + r1) / 2 : r0 + ((v - d0) / (d1 - d0)) * (r1 - r0))) as Scale
  f.domain = domain
  f.range = range
  return f
}

/** Domain [min, max] of several series, padded by `pad` (fraction) and optionally forced to include 0. */
export function extent(series: number[][], { pad = 0.08, zero = false } = {}): [number, number] {
  const all = series.flat().filter((v) => Number.isFinite(v))
  if (all.length === 0) return [0, 1]
  let lo = Math.min(...all)
  let hi = Math.max(...all)
  if (zero) lo = Math.min(0, lo)
  const span = hi - lo || Math.abs(hi) || 1
  hi += span * pad
  if (!zero) lo -= span * pad
  return [lo, hi]
}

const r = (n: number) => Math.round(n * 10) / 10

/** Polyline path "M x y L x y …". */
export function linePath(points: [number, number][]): string {
  return points.map(([x, y], i) => `${i ? 'L' : 'M'}${r(x)} ${r(y)}`).join(' ')
}

/** Closed band between an upper and a lower series sharing the same x's. */
export function bandPath(xs: number[], upper: number[], lower: number[]): string {
  if (xs.length === 0) return ''
  const top = xs.map((x, i) => [x, upper[i]] as [number, number])
  const bottom = xs.map((x, i) => [x, lower[i]] as [number, number]).reverse()
  return linePath([...top, ...bottom]) + ' Z'
}

/** Area under a series down to `baseY`. */
export function areaPath(points: [number, number][], baseY: number): string {
  if (points.length === 0) return ''
  const first = points[0]
  const last = points[points.length - 1]
  return `${linePath(points)} L${r(last[0])} ${r(baseY)} L${r(first[0])} ${r(baseY)} Z`
}

/** Sums several equally long series element-wise. */
export function sumSeries(series: number[][]): number[] {
  const n = Math.max(0, ...series.map((s) => s.length))
  return Array.from({ length: n }, (_, i) => series.reduce((acc, s) => acc + (s[i] ?? 0), 0))
}

export const mean = (xs: number[]) => (xs.length ? xs.reduce((a, b) => a + b, 0) / xs.length : 0)
