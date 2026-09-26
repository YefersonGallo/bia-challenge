import { dayOf, fmtNum } from '@/shared/lib/format'
import type { BandPt, Pt } from '@/shared/lib/series'
import { bandPath, extent, linear, linePath } from './geometry'

export interface Series {
  key: string
  points: Pt[]
  color: string
  width?: number
  dashed?: boolean
  label?: string
}

export interface TimeMarker {
  t: number
  label: string
  color?: string
}

interface Props {
  series: Series[]
  /** Shaded band per point (e.g. baseline p10–p90). */
  band?: BandPt[]
  bandColor?: string
  /** Horizontal reference band, e.g. the 209–231 V window. */
  refBand?: [number, number]
  /** Horizontal reference line, e.g. PF 0,9. */
  refLine?: { value: number; label: string }
  markers?: TimeMarker[]
  /** Vertical line where the change starts. */
  changePoint?: number | null
  /** Timestamps drawn as dots on the solid series (flagged readings). */
  highlight?: Set<number>
  highlightColor?: string
  /** Region shaded as "projection". */
  future?: [number, number] | null
  decimals?: number
  zero?: boolean
  width?: number
  height?: number
  title: string
}

const DAY = 86_400_000

/** Hourly time series in a fixed viewBox: series, bands, reference lines and markers. */
export function TimeSeriesChart({
  series,
  band,
  bandColor = '#16202b',
  refBand,
  refLine,
  markers = [],
  changePoint,
  highlight,
  highlightColor = 'var(--color-data)',
  future,
  decimals = 0,
  zero = false,
  width = 940,
  height = 220,
  title,
}: Props) {
  const pad = { l: 46, r: 12, t: 18, b: 22 }
  const ts = [...series.flatMap((s) => s.points.map((p) => p[0])), ...(band ?? []).map((b) => b[0])]
  if (ts.length === 0) return null
  const t0 = Math.min(...ts)
  const t1 = Math.max(...ts)
  const ys = [
    ...series.map((s) => s.points.map((p) => p[1])),
    (band ?? []).map((b) => b[1]),
    (band ?? []).map((b) => b[2]),
    refBand ?? [],
    refLine ? [refLine.value] : [],
  ]
  const y = linear(extent(ys, { pad: 0.1, zero }), [height - pad.b, pad.t])
  const x = linear([t0, Math.max(t1, t0 + 1)], [pad.l, width - pad.r])
  const ticks = [0.1, 0.5, 0.9].map((f) => y.domain[0] + (y.domain[1] - y.domain[0]) * f)

  const firstDay = Math.ceil(t0 / DAY) * DAY
  const days: number[] = []
  for (let d = firstDay; d <= t1; d += DAY) days.push(d)
  const every = days.length > 10 ? 2 : 1
  const text = 'var(--color-muted)'

  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="block h-auto w-full overflow-visible" role="img" aria-label={title}>
      <title>{title}</title>
      {ticks.map((t) => (
        <g key={t}>
          <line x1={pad.l} x2={width - pad.r} y1={y(t)} y2={y(t)} stroke="#141b23" />
          <text x={pad.l - 8} y={y(t) + 3} textAnchor="end" fontSize="10" fontFamily="var(--font-mono)" fill={text}>
            {fmtNum(t, decimals)}
          </text>
        </g>
      ))}
      {future && <rect x={x(future[0])} width={x(future[1]) - x(future[0])} y={pad.t} height={height - pad.t - pad.b} fill="var(--color-accent)" opacity={0.06} />}
      {refBand && (
        <rect x={pad.l} width={width - pad.l - pad.r} y={y(refBand[1])} height={y(refBand[0]) - y(refBand[1])} fill="var(--color-ok)" opacity={0.07} />
      )}
      {band && band.length > 0 && (
        <path d={bandPath(band.map((b) => x(b[0])), band.map((b) => y(b[2])), band.map((b) => y(b[1])))} fill={bandColor} />
      )}
      {refLine && (
        <g>
          <line x1={pad.l} x2={width - pad.r} y1={y(refLine.value)} y2={y(refLine.value)} stroke="var(--color-ok)" strokeDasharray="4 4" opacity={0.7} />
          <text x={width - pad.r} y={y(refLine.value) - 4} textAnchor="end" fontSize="10" fontFamily="var(--font-mono)" fill="var(--color-ok)">
            {refLine.label}
          </text>
        </g>
      )}
      {changePoint != null && (
        <g>
          <line x1={x(changePoint)} x2={x(changePoint)} y1={pad.t - 6} y2={height - pad.b} stroke="var(--color-ink-2)" strokeWidth={1.2} />
          <text x={x(changePoint) + 4} y={pad.t + 8} fontSize="10" fontFamily="var(--font-mono)" fill="var(--color-ink-2)">
            cambio
          </text>
        </g>
      )}
      {markers.map((m) => {
        const right = x(m.t) > width * 0.6
        return (
          <g key={`${m.t}-${m.label}`}>
            <line x1={x(m.t)} x2={x(m.t)} y1={pad.t - 6} y2={height - pad.b} stroke={m.color ?? 'var(--color-expl)'} strokeDasharray="2 3" strokeWidth={1.2} />
            <text x={x(m.t) + (right ? -5 : 5)} y={pad.t - 2} textAnchor={right ? 'end' : 'start'} fontSize="10" fontFamily="var(--font-mono)" fill={m.color ?? 'var(--color-expl)'}>
              {m.label}
            </text>
          </g>
        )
      })}
      {series.map((s) => (
        <path
          key={s.key}
          d={linePath(s.points.map(([t, v]) => [x(t), y(v)]))}
          fill="none"
          stroke={s.color}
          strokeWidth={s.width ?? 1.4}
          strokeDasharray={s.dashed ? '5 4' : undefined}
          strokeLinejoin="round"
        >
          {s.label && <title>{s.label}</title>}
        </path>
      ))}
      {highlight &&
        [...new Map(series.filter((s) => !s.dashed).flatMap((s) => s.points.filter(([t]) => highlight.has(t)).map((p) => [p[0], p] as const))).values()]
          .map(([t, v]) => <circle key={`h-${t}`} cx={x(t)} cy={y(v)} r={3.2} fill={highlightColor} stroke="var(--color-bg)" strokeWidth={1} />)}
      {days.map((d, i) =>
        i % every === 0 ? (
          <text key={d} x={x(d)} y={height - 4} textAnchor="middle" fontSize="10" fontFamily="var(--font-mono)" fill={text}>
            D{dayOf(new Date(d).toISOString())}
          </text>
        ) : null,
      )}
    </svg>
  )
}
