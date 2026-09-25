import { Link, useNavigate } from 'react-router-dom'
import { useMeters } from '@/shared/api/queries'
import type { MeterSummary } from '@/shared/api/types'
import { Sparkline, VariationBar } from '@/shared/charts/small'
import { fmtNum, fmtPct } from '@/shared/lib/format'
import { METER_STATUS_META, zoneOf } from '@/shared/lib/labels'
import { meterVerdict } from '@/shared/lib/verdict'
import { ErrorBox, Lamp, Loading } from '@/shared/ui/primitives'
import { MeterFilterBar } from './MeterFilterBar'
import { toMeterQuery, useMeterFilters } from './filtersStore'

const COLS = 'minmax(0,1.4fr) minmax(0,1fr) 100px 100px 190px 90px 170px 140px 24px'

function MeterRow({ m }: { m: MeterSummary }) {
  const navigate = useNavigate()
  const v = meterVerdict(m)
  const st = METER_STATUS_META[m.status]
  const varColor = Math.abs(m.variation_pct) >= 25 ? v.color : 'var(--color-soft)'
  return (
    <tr
      onClick={() => navigate(`/meters/${m.id}`)}
      className="grid h-[54px] cursor-pointer items-center gap-3.5 border-b border-raise px-4 hover:bg-panel"
      style={{ gridTemplateColumns: COLS, background: m.anomaly?.type === 'REAL_ANOMALY' ? '#140b0a' : undefined }}
    >
      <td className="flex min-w-0 items-center gap-2.5">
        <Lamp color={v.color} glow={v.attention || m.status === 'OK'} />
        <Link to={`/meters/${m.id}`} className="font-mono text-sm font-semibold text-ink no-underline" onClick={(e) => e.stopPropagation()}>
          {m.id}
        </Link>
        <span className="truncate text-[13px] text-muted">{m.name}</span>
      </td>
      <td className="truncate text-[13px] text-soft">{zoneOf(m.location)}</td>
      <td className="text-right font-mono text-[13px]">{fmtNum(m.current_kwh)}</td>
      <td className="text-right font-mono text-[13px] text-muted">{fmtNum(m.baseline_kwh)}</td>
      <td className="flex items-center gap-2">
        <VariationBar pct={m.variation_pct} color={varColor} />
        <span className="font-mono text-xs font-semibold" style={{ color: varColor }}>
          {fmtPct(m.variation_pct)}
        </span>
      </td>
      <td className="font-mono text-[11px] font-semibold uppercase" style={{ color: st.color }}>
        {st.label}
      </td>
      <td className="font-mono text-[11px] font-semibold" style={{ color: v.classified ? v.color : 'var(--color-dim)' }}>
        {v.classified ? v.text : m.status === 'OK' ? '—' : 'SIN VEREDICTO'}
      </td>
      <td>
        <Sparkline values={m.daily_kwh} color={m.status === 'OK' ? 'var(--color-dim)' : v.color} />
      </td>
      <td className="text-right font-mono text-sm text-muted" aria-hidden>
        →
      </td>
    </tr>
  )
}

/** Meters list with status filter, meter_id search and sort (consumption / variation / severity). */
export function MetersPage() {
  const filters = useMeterFilters()
  const { data = [], isLoading, error } = useMeters(toMeterQuery(filters))

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3 px-6 py-4">
      <div className="flex items-baseline gap-3.5">
        <h1 className="m-0 font-display text-2xl font-semibold tracking-[-0.01em]">Medidores</h1>
        <span className="label text-[11px]">
          {data.length} MEDIDORES · KWH DE LAS ÚLTIMAS 24 H VS BASELINE DIARIO (DÍAS 1–7)
        </span>
      </div>
      <MeterFilterBar withSort />
      {error && <ErrorBox error={error} />}
      <div className="min-h-0 flex-1 overflow-auto rounded-md border border-line bg-panel-2">
        <table className="w-full min-w-[1100px] border-collapse">
          <thead>
            <tr className="grid h-[38px] items-center gap-3.5 border-b border-line px-4 text-left font-mono text-[10px] tracking-[0.08em] text-muted" style={{ gridTemplateColumns: COLS }}>
              <th className="font-normal">MEDIDOR</th>
              <th className="font-normal">ZONA</th>
              <th className="text-right font-normal">KWH 24 H</th>
              <th className="text-right font-normal">BASELINE/DÍA</th>
              <th className="font-normal">VARIACIÓN</th>
              <th className="font-normal">ESTADO</th>
              <th className="font-normal">ANOMALÍA IA</th>
              <th className="font-normal">14 DÍAS</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {data.map((m) => (
              <MeterRow key={m.id} m={m} />
            ))}
          </tbody>
        </table>
        {isLoading && <Loading what="medidores" />}
        {!isLoading && data.length === 0 && (
          <div className="px-4 py-8 text-sm text-soft">
            Ningún medidor coincide con el filtro y la búsqueda.{' '}
            <button type="button" onClick={filters.clear} className="font-semibold text-accent">
              Limpiar filtros
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
