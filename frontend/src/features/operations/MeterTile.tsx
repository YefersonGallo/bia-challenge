import type { MeterSummary } from '@/shared/api/types'
import { BandMarker } from '@/shared/charts/small'
import { fmtNum, fmtPct } from '@/shared/lib/format'
import { meterVerdict } from '@/shared/lib/verdict'
import { Lamp } from '@/shared/ui/primitives'

interface Props {
  meter: MeterSummary
  selected: boolean
  dimmed: boolean
  onSelect: (id: string) => void
}

/** One meter of the operations grid: lamp, 7-day kWh, variation, band position and verdict. */
export function MeterTile({ meter: m, selected, dimmed, onSelect }: Props) {
  const v = meterVerdict(m)
  const real = m.anomaly?.type === 'REAL_ANOMALY' && m.anomaly.status !== 'RESOLVED'
  const border = v.classified || m.status !== 'OK' ? v.color : 'var(--color-line)'
  const varColor = Math.abs(m.variation_pct) >= 25 ? v.color : 'var(--color-soft)'
  return (
    <button
      type="button"
      onClick={() => onSelect(m.id)}
      aria-pressed={selected}
      aria-label={`${m.id} ${m.name}: ${v.text}`}
      className="flex h-[124px] flex-col gap-[3px] rounded-md border px-3 py-2.5 text-left text-ink transition-opacity"
      style={{
        background: real ? '#1a0d0b' : 'var(--color-panel)',
        borderColor: border,
        borderStyle: v.classified || m.status === 'OK' ? 'solid' : 'dashed',
        boxShadow: selected ? `0 0 0 2px var(--color-bg), 0 0 0 3px ${border === 'var(--color-line)' ? 'var(--color-ink)' : border}` : undefined,
        opacity: dimmed ? 0.22 : 1,
      }}
    >
      <span className="flex items-center justify-between">
        <span className="font-mono text-[13px] font-semibold">{m.id}</span>
        <Lamp color={v.color} glow={v.attention || m.status === 'OK'} />
      </span>
      <span className="truncate text-[11px] text-muted">{m.name}</span>
      <span className="flex items-baseline justify-between">
        <span className="font-mono text-[22px] font-medium">{fmtNum(m.current_kwh)}</span>
        <span className="font-mono text-xs font-semibold" style={{ color: varColor }}>
          {fmtPct(m.variation_pct)}
        </span>
      </span>
      <BandMarker pct={m.variation_pct} color={varColor} />
      <span className="truncate font-mono text-[10px] font-semibold" style={{ color: v.color }}>
        {!v.classified && m.invalid_readings > 0 ? 'REGLA' : v.text}
        {m.invalid_readings > 0 && !v.classified && <span className="text-data"> · {m.invalid_readings} INVÁLIDAS</span>}
      </span>
    </button>
  )
}
