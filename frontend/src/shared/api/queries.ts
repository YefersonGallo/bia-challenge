import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, qs } from './client'
import type {
  AnalysisRun,
  Anomaly,
  AnomalyDetail,
  AnomalyStatus,
  AnomalyType,
  DashboardSummary,
  EventItem,
  LoginResponse,
  MeterDetail,
  MeterStatus,
  MeterSummary,
  Report,
  Severity,
} from './types'

/** Query keys in one place so invalidation stays consistent. */
export const keys = {
  summary: ['summary'] as const,
  meters: (q: MeterQuery) => ['meters', q] as const,
  meter: (id: string) => ['meter', id] as const,
  meterEvents: (id: string) => ['meter', id, 'events'] as const,
  events: ['events'] as const,
  anomalies: (q: AnomalyQuery) => ['anomalies', q] as const,
  anomaly: (id: string) => ['anomaly', id] as const,
  run: (id: string) => ['run', id] as const,
  report: ['report'] as const,
}

/** Everything that changes when an analysis finishes or an anomaly moves in its lifecycle. */
const DERIVED = [['summary'], ['meters'], ['meter'], ['anomalies'], ['anomaly'], ['report'], ['run', 'latest']]

export function useInvalidateDerived() {
  const qc = useQueryClient()
  return () => Promise.all(DERIVED.map((queryKey) => qc.invalidateQueries({ queryKey })))
}

/** Returns null instead of throwing when the resource does not exist yet (e.g. no analysis). */
async function orNull<T>(p: Promise<T>): Promise<T | null> {
  try {
    return await p
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return null
    throw e
  }
}

// --- auth ---------------------------------------------------------------------

export function useLogin() {
  return useMutation({
    mutationFn: (body: { email: string; password: string }) =>
      api<LoginResponse>('/auth/login', { method: 'POST', body: JSON.stringify(body) }),
  })
}

// --- dashboard & meters ---------------------------------------------------------

export function useSummary() {
  return useQuery({ queryKey: keys.summary, queryFn: () => api<DashboardSummary>('/dashboard/summary') })
}

export type MeterSort = 'severity' | 'consumption' | 'variation'
export interface MeterQuery {
  status?: MeterStatus
  q?: string
  sort?: MeterSort
}

export function useMeters(query: MeterQuery = {}) {
  return useQuery({
    queryKey: keys.meters(query),
    queryFn: () => api<MeterSummary[]>(`/meters${qs({ status: query.status, q: query.q?.trim(), sort: query.sort })}`),
    placeholderData: keepPreviousData,
  })
}

export function useMeter(id: string | undefined) {
  return useQuery({
    queryKey: keys.meter(id ?? ''),
    queryFn: () => api<MeterDetail>(`/meters/${encodeURIComponent(id!)}`),
    enabled: !!id,
  })
}

export function useMeterEvents(id: string | undefined) {
  return useQuery({
    queryKey: keys.meterEvents(id ?? ''),
    queryFn: () => api<EventItem[]>(`/meters/${encodeURIComponent(id!)}/events`),
    enabled: !!id,
  })
}

export function useEvents() {
  return useQuery({ queryKey: keys.events, queryFn: () => api<EventItem[]>('/events'), staleTime: 5 * 60_000 })
}

// --- anomalies ----------------------------------------------------------------

export interface AnomalyQuery {
  type?: AnomalyType
  severity?: Severity
}

export function useAnomalies(query: AnomalyQuery = {}) {
  return useQuery({
    queryKey: keys.anomalies(query),
    queryFn: () => api<Anomaly[]>(`/anomalies${qs({ type: query.type, severity: query.severity })}`),
    placeholderData: keepPreviousData,
  })
}

export function useAnomaly(id: string | undefined) {
  return useQuery({
    queryKey: keys.anomaly(id ?? ''),
    queryFn: () => api<AnomalyDetail>(`/anomalies/${encodeURIComponent(id!)}`),
    enabled: !!id,
  })
}

export function useUpdateAnomaly() {
  const invalidate = useInvalidateDerived()
  return useMutation({
    mutationFn: ({ id, status }: { id: string; status: AnomalyStatus }) =>
      api<Anomaly>(`/anomalies/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify({ status }) }),
    onSuccess: () => invalidate(),
  })
}

// --- analysis runs --------------------------------------------------------------

const isActive = (r: AnalysisRun | null | undefined) => r?.status === 'RUNNING' || r?.status === 'PENDING'

/** A run by id ("latest" included). Polls while the run is in progress so the steps animate. */
export function useAnalysisRun(id: string | null, pollMs = 400) {
  return useQuery({
    queryKey: keys.run(id ?? 'latest'),
    queryFn: () => orNull(api<AnalysisRun>(`/ai/analysis/${encodeURIComponent(id ?? 'latest')}`)),
    refetchInterval: (q) => (isActive(q.state.data) ? pollMs : false),
  })
}

export function useStartAnalysis() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => api<AnalysisRun>('/ai/analyze', { method: 'POST' }),
    onSuccess: (run) => qc.setQueryData(keys.run(run.id), run),
  })
}

// --- report -------------------------------------------------------------------

export function useReport() {
  return useQuery({ queryKey: keys.report, queryFn: () => orNull(api<Report>('/reports/latest')) })
}
