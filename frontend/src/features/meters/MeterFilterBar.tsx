import { useSummary } from '@/shared/api/queries'
import { Button, SearchBox, Segmented } from '@/shared/ui/primitives'
import { isFiltered, SORT_OPTIONS, STATUS_OPTIONS, useMeterFilters } from './filtersStore'

/** Status filter + meter_id search (+ sort, optionally). Counts come from the dashboard summary. */
export function MeterFilterBar({ withSort = false }: { withSort?: boolean }) {
  const f = useMeterFilters()
  const { data: summary } = useSummary()
  const count = (v: string) => (v === 'ALL' ? summary?.meters : summary?.status_counts[v as 'OK']) ?? ''

  return (
    <div className="flex flex-wrap items-center gap-3">
      <Segmented
        label="Filtrar por estado"
        value={f.status}
        onChange={f.setStatus}
        options={STATUS_OPTIONS.map((o) => ({ ...o, label: `${o.label} ${count(o.value)}`.trim() }))}
      />
      <SearchBox value={f.q} onChange={f.setQ} label="Buscar por meter_id" placeholder="meter_id · ej. M-109" className="w-60" />
      {isFiltered(f) && (
        <Button variant="link" onClick={f.clear}>
          LIMPIAR
        </Button>
      )}
      {withSort && (
        <div className="ml-auto flex items-center gap-1">
          <span className="label mr-1">ORDENAR</span>
          <Segmented label="Ordenar" value={f.sort} onChange={f.setSort} options={SORT_OPTIONS} />
        </div>
      )}
    </div>
  )
}
