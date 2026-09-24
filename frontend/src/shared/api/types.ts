// Types mirror the JSON contract of the Go API (backend/internal/...).

export type MeterStatus = 'OK' | 'ALERT' | 'CRITICAL'
export type AnomalyType = 'REAL_ANOMALY' | 'EXPLAINABLE_ANOMALY' | 'FALSE_POSITIVE' | 'DATA_QUALITY'
export type Severity = 'HIGH' | 'MEDIUM' | 'LOW'
export type AnomalyStatus = 'OPEN' | 'ACKNOWLEDGED' | 'IN_PROGRESS' | 'RESOLVED'
export type RunStatus = 'PENDING' | 'RUNNING' | 'COMPLETED' | 'FAILED'

export interface Meter {
  id: string
  name: string
  location: string
  created_at: string
}

export interface AnomalyRef {
  id: string
  type: AnomalyType
  severity: Severity
  rank: number
  confidence: number
  status: AnomalyStatus
}

export interface MeterSummary extends Meter {
  status: MeterStatus
  status_reason: string
  baseline_kwh: number
  current_kwh: number
  variation_pct: number
  invalid_readings: number
  daily_kwh: number[]
  anomaly: AnomalyRef | null
}

export interface DayPoint {
  day: number
  date: string
  kwh: number
  expected_kwh: number
  deviation_pct: number
  invalid: number
  voltage_v: number
  current_a: number
  power_factor: number
}

export interface Electrical {
  kwh_per_day: number
  voltage_v: number
  current_a: number
  power_factor: number
}

export interface Episode {
  onset_day: number
  end_day: number
  onset: string
  direction: 1 | -1
  hours: number
  recovered: boolean
  mean_deviation_pct: number
}

export interface MeterStats {
  meter_id: string
  baseline_kwh: number
  current_kwh: number
  variation_pct: number
  status: MeterStatus
  status_reason: string
  invalid_readings: number
  pf_out_of_range: number
  zero_voltage: number
  physical_coherence: number
  days: DayPoint[]
  hourly_baseline: number[]
  hourly_current: number[]
  night_ratio: number
  spikes: number
  episode?: Episode
  base_electrical: Electrical
  current_electrical: Electrical
}

export interface EventItem {
  id: string
  meter_id: string
  timestamp: string
  type: string
  description: string
}

export interface Signal {
  code: string
  description: string
  value: number
}

export interface VariableChange {
  name: string
  unit: string
  baseline: number
  current: number
  delta_pct: number
}

export interface Evidence {
  meter_id: string
  meter_name: string
  baseline_kwh: number
  current_kwh: number
  variation_pct: number
  onset_day?: number
  end_day?: number
  persistent_hours: number
  night_ratio: number
  invalid_readings: number
  physical_coherence: number
  signals: Signal[] | null
  variables: VariableChange[] | null
  related_events: EventItem[] | null
  event_explains_shift: boolean
}

export interface Anomaly {
  id: string
  analysis_id: string
  meter_id: string
  detected_at: string
  type: AnomalyType
  severity: Severity
  confidence: number
  priority_score: number
  rank: number
  reason: string
  recommended_action: string
  next_steps: string[] | null
  explained_by: 'claude' | 'template'
  status: AnomalyStatus
  evidence: Evidence
}

export interface AnomalyDetail extends Anomaly {
  meter: Meter
  days: DayPoint[]
  hourly_baseline: number[]
  hourly_current: number[]
  anomaly: boolean
}

export interface MeterDetail extends MeterSummary {
  stats: MeterStats
  events: EventItem[]
  finding: Anomaly | null
}

export interface StepState {
  key: string
  label: string
  status: RunStatus
  result?: string
}

export interface AnalysisRun {
  id: string
  status: RunStatus
  started_at: string
  finished_at?: string
  current_step: number
  steps: StepState[]
  summary?: { anomalies: number; high_priority: number; avg_confidence: number; headline: string }
  error?: string
}

export interface DashboardSummary {
  meters: number
  status_counts: Record<MeterStatus, number>
  current_kwh: number
  baseline_kwh: number
  variation_pct: number
  anomalies: number | null
  high_priority: number | null
  open_anomalies: number | null
  avg_confidence: number | null
  last_analysis: AnalysisRun | null
  invalid_readings: number
  readings: number
}

export interface Report {
  run_id: string
  generated_at: string
  headline: string
  summary: DashboardSummary
  contributions: { meter_id: string; name: string; delta_kwh: number }[]
  rule_flags: MeterSummary[]
  findings: AnomalyDetail[]
  plan: { meter_id: string; type: AnomalyType; action: string; owner: string; deadline: string; steps: string[] | null }[]
  methodology: Methodology
  sources: string[]
}

export interface Methodology {
  baseline_days: number
  shift_threshold_pct: number
  critical_pct: number
  high_pct: number
  z_threshold: number
  event_window_hours: number
  coherence_tol_pct: number
  min_invalid: number
  min_coherence_pct: number
  pf_drop_threshold: number
  night_ratio_signal: number
}

export interface LoginResponse {
  token: string
  expires_at: string
  user: { email: string; name: string }
}
