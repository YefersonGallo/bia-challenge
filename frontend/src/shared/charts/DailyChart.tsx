import type { DayPoint } from '@/shared/api/types'
import { fmtNum } from '@/shared/lib/format'
import { bandPath, extent, linear, linePath } from './geometry'

export interface ChartMarker {
  day: number
  label: string
  color?: string
}

interface Props {
  days: DayPoint[]
  /** Color of the highlighted window (the anomaly color). */
  color?: string
  /** Inclusive day range to highlight: the episode. */
  window?: [number, number] | null
  markers?: ChartMarker[]
  /** Paints days with invalid readings. */
  showInvalid?: boolean
  width?: number
  height?: number
  /** Light palette for the report paper. */
  paper?: boolean
  title?: string
}

/**
 * Daily kWh vs. the expected baseline, with a ±10% band, the episode window and event markers.
 * Renders in a fixed viewBox and scales to the width of its container.
 */
export function DailyChart({
  days,
  color = 'var(--color-accent)',
  window,
  markers = [],
  showInvalid = true,
  width = 940,
  height = 250,
  paper = false,
  title = 'Consumo diario frente al baseline',
}: Props) {
  const pad = { l: 46, r: 12, t: 18, b: 22 }
  const kwh = days.map((d) => d.kwh)
  const exp = days.map((d) => d.expected_kwh)
  const y = linear(extent([kwh, exp.map((e) => e * 1.1), exp.map((e) => e * 0.9)], { pad: 0.12 }), [height - pad.b, pad.t])
  const x = linear([1, Math.max(2, days.length)], [pad.l, width - pad.r])
  const xs = days.map((d) => x(d.day))

  const c = paper
    ? { grid: 'var(--color-paper-line)', band: '#eceef3', base: 'var(--color-paper-muted)', line: 'var(--color-paper-2)', text: 'var(--color-paper-muted)' }
    : { grid: '#141b23', band: '#16202b', base: 'var(--color-dim)', line: 'var(--color-soft)', text: 'var(--color-muted)' }

  const ticks = [0.1, 0.5, 0.9].map((f) => y.domain[0] + (y.domain[1] - y.domain[0]) * f)
  const win = window ? days.filter((d) => d.day >= window[0] && d.day <= window[1]) : []

  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="block h-auto w-full overflow-visible" role="img" aria-label={title}>
      {ticks.map((t) => (
        <g key={t}>
          <line x1={pad.l} x2={width - pad.r} y1={y(t)} y2={y(t)} stroke={c.grid} />
          <text x={pad.l - 8} y={y(t) + 3} textAnchor="end" fontSize="10" fontFamily="var(--font-mono)" fill={c.text}>
            {fmtNum(t)}
          </text>
        </g>
      ))}
      {win.length > 0 && (
        <rect
          x={x(win[0].day) - 8}
          width={x(win[win.length - 1].day) - x(win[0].day) + 16}
          y={pad.t}
          height={height - pad.t - pad.b}
          fill={color}
          opacity={0.08}
        />
      )}
      <path d={bandPath(xs, exp.map((e) => y(e * 1.1)), exp.map((e) => y(e * 0.9)))} fill={c.band} />
      <path d={linePath(days.map((d) => [x(d.day), y(d.expected_kwh)]))} fill="none" stroke={c.base} strokeWidth={1.2} strokeDasharray="4 4" />
      {markers.map((m) => (
        <g key={`${m.day}-${m.label}`}>
          <line x1={x(m.day)} x2={x(m.day)} y1={pad.t - 6} y2={height - pad.b} stroke={m.color ?? 'var(--color-expl)'} strokeDasharray="2 3" strokeWidth={1.2} />
          <text
            x={x(m.day) + (x(m.day) > width * 0.6 ? -5 : 5)}
            y={pad.t - 2}
            textAnchor={x(m.day) > width * 0.6 ? 'end' : 'start'}
            fontSize="10"
            fontFamily="var(--font-mono)"
            fill={m.color ?? 'var(--color-expl)'}
          >
            {m.label}
          </text>
        </g>
      ))}
      <path d={linePath(days.map((d) => [x(d.day), y(d.kwh)]))} fill="none" stroke={c.line} strokeWidth={1.8} strokeLinejoin="round" />
      {win.length > 0 && (
        <path d={linePath(win.map((d) => [x(d.day), y(d.kwh)]))} fill="none" stroke={color} strokeWidth={2.8} strokeLinejoin="round" />
      )}
      {showInvalid &&
        days
          .filter((d) => d.invalid > 0)
          .map((d) => (
            <g key={`inv-${d.day}`}>
              <circle cx={x(d.day)} cy={y(d.kwh)} r={4.5} fill="var(--color-data)" />
              <title>{`D${d.day}: ${d.invalid} lecturas inválidas`}</title>
            </g>
          ))}
      {days.map((d) => (
        <text key={`x-${d.day}`} x={x(d.day)} y={height - 4} textAnchor="middle" fontSize="10" fontFamily="var(--font-mono)" fill={c.text}>
          {d.day === 1 || d.day === 8 || d.day === days.length || days.length <= 7 ? `D${d.day}` : ''}
        </text>
      ))}
    </svg>
  )
}
