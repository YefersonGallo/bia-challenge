import type { HeatmapRow } from '@/shared/api/types'
import { fmtPct } from '@/shared/lib/format'
import { METER_STATUS_META } from '@/shared/lib/labels'
import { deviationColor } from '@/shared/lib/series'
import { Lamp } from '@/shared/ui/primitives'

/**
 * Meters × days, colored by the deviation of each day against the meter's own baseline
 * (diverging: blue below, red above). A violet dot marks days with inconsistent readings.
 */
export function Heatmap({ rows, onSelect }: { rows: HeatmapRow[]; onSelect: (row: HeatmapRow) => void }) {
  const days = rows[0]?.days ?? []
  return (
    <div className="flex flex-col gap-1.5">
      <table className="w-full border-separate border-spacing-[2px] text-left" aria-label="Mapa de calor de desviación diaria">
        <thead>
          <tr>
            <th className="w-[150px]" />
            {days.map((d) => (
              <th key={d.day} scope="col" className="text-center font-mono text-[9px] font-normal text-muted">
                D{d.day}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.meter_id}>
              <th scope="row" className="p-0 font-normal">
                <button
                  type="button"
                  onClick={() => onSelect(r)}
                  className="flex w-full items-center gap-1.5 rounded px-1 py-0.5 text-left hover:bg-raise"
                  aria-label={`Ver ${r.meter_id}`}
                >
                  <Lamp color={METER_STATUS_META[r.status].color} glow={r.status !== 'OK'} size={7} />
                  <span className="font-mono text-[11px]">{r.meter_id}</span>
                  <span className="truncate text-[11px] text-muted">{r.name}</span>
                </button>
              </th>
              {r.days.map((d) => (
                <td
                  key={d.day}
                  onClick={() => onSelect(r)}
                  title={`${r.meter_id} · D${d.day}: ${fmtPct(d.deviation_pct)} frente al baseline${d.invalid ? ` · ${d.invalid} lecturas marcadas` : ''}`}
                  className="relative h-[18px] min-w-[18px] cursor-pointer rounded-[2px]"
                  style={{ background: deviationColor(d.deviation_pct) }}
                >
                  {d.invalid > 0 && <span className="absolute top-[3px] right-[3px] h-[5px] w-[5px] rounded-full bg-data" />}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
      <div className="flex items-center gap-3 font-mono text-[10px] text-muted">
        <span className="flex items-center gap-1">
          <span className="inline-block h-2.5 w-4 rounded-[2px]" style={{ background: deviationColor(-80) }} /> −80%
        </span>
        <span className="flex items-center gap-1">
          <span className="inline-block h-2.5 w-4 rounded-[2px]" style={{ background: deviationColor(0) }} /> ±5%
        </span>
        <span className="flex items-center gap-1">
          <span className="inline-block h-2.5 w-4 rounded-[2px]" style={{ background: deviationColor(80) }} /> +80%
        </span>
        <span className="flex items-center gap-1">
          <span className="inline-block h-[5px] w-[5px] rounded-full bg-data" /> lecturas marcadas
        </span>
      </div>
    </div>
  )
}
