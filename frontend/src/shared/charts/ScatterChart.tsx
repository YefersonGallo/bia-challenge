import { fmtNum } from '@/shared/lib/format'
import type { Pt } from '@/shared/lib/series'
import { extent, linear } from './geometry'

interface Group {
  key: string
  label: string
  points: Pt[]
  color: string
}

/** x/y scatter of several groups (e.g. current vs. kWh before and after the change). */
export function ScatterChart({
  groups,
  xLabel,
  yLabel,
  width = 460,
  height = 220,
  title,
}: {
  groups: Group[]
  xLabel: string
  yLabel: string
  width?: number
  height?: number
  title: string
}) {
  const pad = { l: 46, r: 12, t: 12, b: 30 }
  const all = groups.flatMap((g) => g.points)
  const x = linear(extent([all.map((p) => p[0])], { pad: 0.05, zero: true }), [pad.l, width - pad.r])
  const y = linear(extent([all.map((p) => p[1])], { pad: 0.08, zero: true }), [height - pad.b, pad.t])
  const xt = [0.25, 0.5, 0.75].map((f) => x.domain[0] + (x.domain[1] - x.domain[0]) * f)
  const yt = [0.25, 0.5, 0.75].map((f) => y.domain[0] + (y.domain[1] - y.domain[0]) * f)
  const text = 'var(--color-muted)'
  return (
    <figure className="m-0 flex flex-col gap-1">
      <svg viewBox={`0 0 ${width} ${height}`} className="block h-auto w-full" role="img" aria-label={title}>
        <title>{title}</title>
        {yt.map((t) => (
          <g key={`y${t}`}>
            <line x1={pad.l} x2={width - pad.r} y1={y(t)} y2={y(t)} stroke="#141b23" />
            <text x={pad.l - 6} y={y(t) + 3} textAnchor="end" fontSize="10" fontFamily="var(--font-mono)" fill={text}>
              {fmtNum(t, 1)}
            </text>
          </g>
        ))}
        {xt.map((t) => (
          <text key={`x${t}`} x={x(t)} y={height - 14} textAnchor="middle" fontSize="10" fontFamily="var(--font-mono)" fill={text}>
            {fmtNum(t)}
          </text>
        ))}
        <text x={width - pad.r} y={height - 2} textAnchor="end" fontSize="10" fontFamily="var(--font-mono)" fill={text}>
          {xLabel}
        </text>
        <text x={pad.l} y={pad.t - 2} fontSize="10" fontFamily="var(--font-mono)" fill={text}>
          {yLabel}
        </text>
        {groups.map((g) => (
          <g key={g.key} fill={g.color} opacity={0.75}>
            {g.points.map(([a, b], i) => (
              <circle key={i} cx={x(a)} cy={y(b)} r={2.2} />
            ))}
          </g>
        ))}
      </svg>
      <figcaption className="flex gap-3 font-mono text-[10px] text-muted">
        {groups.map((g) => (
          <span key={g.key} className="flex items-center gap-1">
            <span className="inline-block h-2 w-2 rounded-full" style={{ background: g.color }} />
            {g.label} · {g.points.length}
          </span>
        ))}
      </figcaption>
    </figure>
  )
}
