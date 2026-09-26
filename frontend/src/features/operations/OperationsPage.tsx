import { useMemo } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useEvents, useHeatmap, useMeters } from '@/shared/api/queries'
import type { MeterSummary } from '@/shared/api/types'
import { Heatmap } from '@/shared/charts/Heatmap'
import { PlantChart } from '@/shared/charts/small'
import { sumSeries } from '@/shared/charts/geometry'
import { fmtDayHour } from '@/shared/lib/format'
import { Button, EmptyState, ErrorBox, Label, Loading } from '@/shared/ui/primitives'
import { MeterFilterBar } from '@/features/meters/MeterFilterBar'
import { isFiltered, toMeterQuery, useMeterFilters } from '@/features/meters/filtersStore'
import { AlarmStack } from './AlarmStack'
import { FocusPanel } from './FocusPanel'
import { MeterTile } from './MeterTile'
import { groupByZone, packRows } from './zones'

function PlantOverview({ meters }: { meters: MeterSummary[] }) {
  const { data: events = [] } = useEvents()
  const totals = useMemo(() => sumSeries(meters.map((m) => m.daily_kwh)), [meters])
  const withEvents = new Set(events.map((e) => e.meter_id))
  const flaggedWithout = meters.filter((m) => m.status !== 'OK' && !withEvents.has(m.id)).map((m) => m.id)
  return (
    <section className="grid flex-1 gap-5 rounded-md border border-line bg-panel px-4 py-3.5 xl:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)]">
      <div className="flex flex-col gap-1.5">
        <Label>CARGA TOTAL DE LA PLANTA · KWH/DÍA · 14 DÍAS</Label>
        <PlantChart totals={totals} />
        <span className="text-xs text-muted">Selecciona un medidor o una alarma para ver el foco con la explicación de la IA.</span>
      </div>
      <div className="flex flex-col gap-2">
        <Label>EVENTOS OPERATIVOS · EVENTS.CSV</Label>
        {events.map((e) => (
          <div key={e.id} className="flex flex-col gap-0.5 rounded border border-line bg-panel-2 px-3 py-2.5">
            <span className="font-mono text-[11px] text-expl">
              {fmtDayHour(e.timestamp)} · {e.meter_id}
            </span>
            <span className="text-[13px]">{e.description}</span>
          </div>
        ))}
        {flaggedWithout.length > 0 && <span className="font-mono text-[11px] text-muted">Sin eventos para {flaggedWithout.join(' ni ')}.</span>}
      </div>
    </section>
  )
}

/** Meters × days deviation map; clicking a row opens the investigation (or the meter). */
function DeviationMap() {
  const { data: rows = [] } = useHeatmap()
  const navigate = useNavigate()
  if (rows.length === 0) return null
  return (
    <section className="flex flex-col gap-1.5 rounded-md border border-line bg-panel px-4 py-3.5">
      <Label>DESVIACIÓN DIARIA FRENTE AL BASELINE · MEDIDOR × DÍA</Label>
      <Heatmap rows={rows} onSelect={(r) => navigate(r.anomaly ? `/anomalies/${r.anomaly.id}` : `/meters/${r.meter_id}`)} />
    </section>
  )
}

/** Control-room view: meters by zone, focus panel and AI alarm stack. */
export function OperationsPage() {
  const filters = useMeterFilters()
  const all = useMeters({})
  const filtered = useMeters(toMeterQuery(filters))
  const [params, setParams] = useSearchParams()
  const focus = params.get('focus')

  const setFocus = (id: string | null) =>
    setParams((p) => {
      if (id) p.set('focus', id)
      else p.delete('focus')
      return p
    })

  if (all.isLoading) return <Loading what="medidores" />
  if (all.error) return <ErrorBox error={all.error} />

  const meters = all.data ?? []
  const matches = new Set((filtered.data ?? meters).map((m) => m.id))
  const noResults = isFiltered(filters) && matches.size === 0

  return (
    <div className="grid min-h-0 flex-1 gap-4 px-6 pt-3.5 pb-4.5 lg:grid-cols-[minmax(0,1fr)_360px]">
      <div className="flex min-h-0 min-w-0 flex-col gap-3 overflow-y-auto pr-1">
        <MeterFilterBar />
        {noResults && (
          <EmptyState
            title="Ningún medidor coincide con el filtro y la búsqueda."
            action={
              <Button variant="link" onClick={filters.clear}>
                LIMPIAR FILTROS
              </Button>
            }
          />
        )}
        {packRows(groupByZone(meters)).map((row) => (
          <div key={row.map((g) => g.zone).join()} className="grid grid-cols-6 gap-x-2 gap-y-1.5">
            {row.map((g) => (
              <span key={g.zone} className="border-b border-line pb-[3px] font-mono text-[10px] tracking-[0.1em] text-muted uppercase" style={{ gridColumn: `span ${Math.min(g.meters.length, 6)}` }}>
                {g.zone}
              </span>
            ))}
            {row.flatMap((g) =>
              g.meters.map((m) => (
                <MeterTile key={m.id} meter={m} selected={focus === m.id} dimmed={!matches.has(m.id)} onSelect={(id) => setFocus(focus === id ? null : id)} />
              )),
            )}
          </div>
        ))}
        {focus ? (
          <FocusPanel meterId={focus} onClose={() => setFocus(null)} />
        ) : (
          <>
            <PlantOverview meters={meters} />
            <DeviationMap />
          </>
        )}
      </div>
      <AlarmStack onFocus={setFocus} />
    </div>
  )
}
