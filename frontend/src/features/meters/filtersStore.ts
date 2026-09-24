import { create } from 'zustand'
import type { MeterQuery, MeterSort } from '@/shared/api/queries'
import type { MeterStatus } from '@/shared/api/types'

export type StatusFilter = 'ALL' | MeterStatus

/** Meter filters shared by the operations grid and the meters table (so they stay in sync). */
interface FiltersState {
  status: StatusFilter
  q: string
  sort: MeterSort
  setStatus: (s: StatusFilter) => void
  setQ: (q: string) => void
  setSort: (s: MeterSort) => void
  clear: () => void
}

export const useMeterFilters = create<FiltersState>()((set) => ({
  status: 'ALL',
  q: '',
  sort: 'severity',
  setStatus: (status) => set({ status }),
  setQ: (q) => set({ q }),
  setSort: (sort) => set({ sort }),
  clear: () => set({ status: 'ALL', q: '' }),
}))

/** Translates the UI filters to the API query. */
export function toMeterQuery(f: Pick<FiltersState, 'status' | 'q' | 'sort'>): MeterQuery {
  return {
    status: f.status === 'ALL' ? undefined : f.status,
    q: f.q.trim() || undefined,
    sort: f.sort,
  }
}

export const isFiltered = (f: Pick<FiltersState, 'status' | 'q'>) => f.status !== 'ALL' || f.q.trim() !== ''

export const STATUS_OPTIONS: { value: StatusFilter; label: string; dot?: string }[] = [
  { value: 'ALL', label: 'TODOS' },
  { value: 'OK', label: 'NORMALES', dot: 'var(--color-ok)' },
  { value: 'ALERT', label: 'ALERTAS', dot: 'var(--color-expl)' },
  { value: 'CRITICAL', label: 'CRÍTICAS', dot: 'var(--color-real)' },
]

export const SORT_OPTIONS: { value: MeterSort; label: string }[] = [
  { value: 'severity', label: 'SEVERIDAD' },
  { value: 'consumption', label: 'CONSUMO' },
  { value: 'variation', label: 'VARIACIÓN' },
]
