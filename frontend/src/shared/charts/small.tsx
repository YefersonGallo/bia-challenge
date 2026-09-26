import { fmtNum } from '@/shared/lib/format'
import { areaPath, bandPath, extent, linear, linePath, mean } from './geometry'

/** 14-day sparkline with the baseline ±10% band (mean of days 1–7). */
export function Sparkline({
  values,
  color = 'var(--color-soft)',
  width = 140,
  height = 28,
  fluid = false,
}: {
  values: number[]
  color?: string
  width?: number
  height?: number
  /** Stretch to the container width instead of a fixed size. */
  fluid?: boolean
}) {
  if (values.length < 2) return null
  const base = mean(values.slice(0, 7))
  const y = linear(extent([values, [base * 1.1, base * 0.9]], { pad: 0.1 }), [height - 2, 2])
  const x = linear([0, values.length - 1], [0, width])
  return (
    <svg
      viewBox={`0 0 ${width} ${height}`}
      width={fluid ? '100%' : width}
      height={height}
      preserveAspectRatio={fluid ? 'none' : undefined}
      className={fluid ? 'block' : 'overflow-visible'}
      aria-hidden
    >
      <path d={bandPath([0, width], [y(base * 1.1), y(base * 1.1)], [y(base * 0.9), y(base * 0.9)])} fill="#141b23" />
      <path d={linePath(values.map((v, i) => [x(i), y(v)]))} fill="none" stroke={color} strokeWidth={1.5} strokeLinejoin="round" />
    </svg>
  )
}

/** Hourly profile: baseline vs. current window, night hours shaded. */
export function HourlyChart({
  baseline,
  current,
  color = 'var(--color-accent)',
  width = 560,
  height = 170,
  paper = false,
}: {
  baseline: number[]
  current: number[]
  color?: string
  width?: number
  height?: number
  paper?: boolean
}) {
  const pad = { l: 34, r: 8, t: 8, b: 20 }
  const y = linear(extent([baseline, current], { zero: true }), [height - pad.b, pad.t])
  const x = linear([0, 23], [pad.l, width - pad.r])
  const text = paper ? 'var(--color-paper-muted)' : 'var(--color-muted)'
  const night = paper ? '#f0f1f5' : '#0f151c'
  const base = paper ? 'var(--color-paper-muted)' : 'var(--color-dim)'
  const hours = [0, 6, 12, 18, 23]
  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="block h-auto w-full" role="img" aria-label="Perfil horario: baseline frente a ventana actual">
      <rect x={x(0)} y={pad.t} width={x(6) - x(0)} height={height - pad.t - pad.b} fill={night} />
      <rect x={x(22)} y={pad.t} width={x(23) - x(22)} height={height - pad.t - pad.b} fill={night} />
      <path d={linePath(baseline.map((v, h) => [x(h), y(v)]))} fill="none" stroke={base} strokeWidth={1.4} strokeDasharray="4 4" />
      <path d={linePath(current.map((v, h) => [x(h), y(v)]))} fill="none" stroke={color} strokeWidth={2.2} strokeLinejoin="round" />
      {hours.map((h) => (
 <text key={h} x={x(h)} y={height - 4} textAnchor={h === 0 ? 'start' : h === 23 ? 'end' : 'middle'} fontSize="10" fontFamily="var(--font-mono)" fill={text}>
          {String(h).padStart(2, '0')}:00
        </text>
      ))}
      <text x={pad.l - 6} y={y(y.domain[1] * 0.92) + 3} textAnchor="end" fontSize="10" fontFamily="var(--font-mono)" fill={text}>
        {fmtNum(y.domain[1] * 0.92)}
      </text>
    </svg>
  )
}

/** Plant total per day with the baseline dashed and the split between baseline and current window. */
export function PlantChart({ totals, width = 900, height = 170 }: { totals: number[]; width?: number; height?: number }) {
  if (totals.length < 2) return null
  const base = mean(totals.slice(0, 7))
  const pad = { t: 16, b: 4 }
  const y = linear(extent([totals, [base]], { pad: 0.15 }), [height - pad.b, pad.t])
  const x = linear([0, totals.length - 1], [0, width])
  const pts = totals.map((v, i) => [x(i), y(v)] as [number, number])
  const split = x(6.5)
  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="block h-auto w-full overflow-visible" role="img" aria-label="Carga total de la planta por día">
      <path d={areaPath(pts, height)} fill="#13202c" />
      <path d={linePath([[0, y(base)], [width, y(base)]])} stroke="var(--color-dim)" strokeWidth={1.2} strokeDasharray="4 4" />
      <path d={linePath(pts)} fill="none" stroke="var(--color-accent)" strokeWidth={2} strokeLinejoin="round" />
      <line x1={split} x2={split} y1={0} y2={height} stroke="var(--color-line-2)" />
      <text x={split + 6} y={10} fontSize="10" fontFamily="var(--font-mono)" fill="var(--color-muted)">
        D8 · FIN DEL BASELINE
      </text>
      <text x={width} y={y(base) - 4} textAnchor="end" fontSize="10" fontFamily="var(--font-mono)" fill="var(--color-muted)">
        baseline
      </text>
    </svg>
  )
}

/**
 * Horizontal bar of a variation around the baseline: zero at a fixed position,
 * positive to the right, negative to the left, clamped to ±range.
 */
export function VariationBar({ pct, color, width = 110, range = 110 }: { pct: number; color: string; width?: number; range?: number }) {
  const zero = width / 3
  const scale = (width - zero) / range
  const len = Math.min(Math.abs(pct), pct < 0 ? zero / scale : range) * scale
  const left = pct < 0 ? zero - len : zero
  return (
    <span className="relative block h-1.5 rounded-sm bg-raise" style={{ width }} aria-hidden>
      <span className="absolute -top-[3px] -bottom-[3px] w-px bg-[#3a4756]" style={{ left: zero }} />
      <span className="absolute top-0 h-1.5 rounded-sm" style={{ left, width: Math.max(2, len), background: color }} />
    </span>
  )
}

/** Position marker of the current value relative to the ±25% band (tiles of the operations grid). */
export function BandMarker({ pct, color, span = 120 }: { pct: number; color: string; span?: number }) {
  const pos = (p: number) => Math.max(0, Math.min(100, 50 + (p / span) * 50))
  return (
    <span className="relative my-1 block h-1.5 rounded-sm bg-raise" aria-hidden>
      <span className="absolute top-0 h-1.5 bg-[#2a3644]" style={{ left: `${pos(-25)}%`, width: `${pos(25) - pos(-25)}%` }} />
      <span className="absolute -top-[3px] h-3 w-[3px] rounded-[1px]" style={{ left: `calc(${pos(pct)}% - 1px)`, background: color }} />
    </span>
  )
}
