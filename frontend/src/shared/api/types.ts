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
  /** Consumption of the whole period (14 days). */
  period_kwh: number
  /** Expected consumption of one day (hour-of-day medians of days 1–7). */
  baseline_kwh: number
  /** Consumption of the last 24 hours. */
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
  end: string
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
  voltage_anomalies: number
  pf_jumps: number
  incoherent_readings: number
  physical_coherence: number
  issue_onset?: string
  issue_hours: number
  voltage_range: [number, number]
  pf_range: [number, number]
  relation_shift_pct: number
  readings: number
  period_kwh: number
  days: DayPoint[]
  hourly_baseline: number[]
  hourly_current: number[]
  night_ratio: number
  spikes: number
  episode?: Episode
  base_electrical: Electrical
  current_electrical: Electrical
  voltage_jumps: number
  flagged_readings: FlaggedReading[] | null
  /** Highest share of flagged readings in any 24 h window (0–1). */
  max_flag_share_24h: number
  /** kWh / (V·I·PF/1000) of the reference days, and its robust spread. */
  k_factor: number
  k_mad: number
  hourly_p10: number[]
  hourly_p90: number[]
  /** Observed level since the change point divided by the baseline. */
  level_factor: number
  extra_kwh_so_far: number
  episode_z: number
  variable_z: Record<'kwh' | 'current' | 'voltage' | 'power_factor', number>
}

export type DQFlag = 'DQ_RANGE' | 'DQ_VOLTAGE' | 'DQ_JUMP' | 'DQ_PF_JUMP' | 'DQ_PHYSICS'

export interface FlaggedReading {
  timestamp: string
  flags: DQFlag[]
}

export interface Reading {
  meter_id: string
  timestamp: string
  consumption_kwh: number
  voltage_v: number
  current_a: number
  power_factor: number
}

export interface EventItem {
  id: string
  meter_id: string
  timestamp: string
  type: string
  description: string
  category: 'EXPLANATORY' | 'NON_EXPLANATORY' | 'INFORMATIONAL'
  expected_effect: 'UP' | 'DOWN' | 'NONE'
  duration_hours?: number
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
  /** Mean deviation of the episode (the change an event may explain). */
  shift_pct: number
  onset?: string
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
  evidence_summary?: string
  confidence_breakdown?: ConfidenceComponent[] | null
  projected_impact?: Impact | null
  change_point_at?: string
  ended_at?: string
}

export interface ConfidenceComponent {
  key: 'magnitude' | 'persistence' | 'variables' | 'events'
  label: string
  weight: number
  score: number
  detail: string
}

export interface Impact {
  extra_kwh_per_day: number
  extra_kwh_per_month: number
  extra_kwh_so_far: number
  cost_per_month_cop: number
  tariff_cop_per_kwh: number
  power_factor: number
  reactive_ratio: number
  reactive_excess: number
  reactive_kvarh_day: number
  normalized: number
}

export type ActionKind = 'acknowledge' | 'investigate' | 'validate' | 'resolve' | 'dismiss' | 'note'

export interface AnomalyAction {
  id: string
  anomaly_id: string
  action: ActionKind
  note?: string
  /** Status after the action, when it changed it (notes leave it empty). */
  status?: AnomalyStatus
  actor: string
  at: string
}

export interface AnomalyDetail extends Anomaly {
  meter: Meter
  days: DayPoint[]
  hourly_baseline: number[]
  hourly_current: number[]
  anomaly: boolean
  actions: AnomalyAction[] | null
  k_factor: number
  k_mad: number
  flagged_readings: FlaggedReading[] | null
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
  duration_ms?: number
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
  period_kwh: number
  period_days: number
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
  min_episode_hours: number
  voltage_min: number
  voltage_max: number
  voltage_jump: number
  duration_tol_hours: number
  dq_high_share_pct: number
  tariff_cop_per_kwh: number
  pf_jump: number
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

export interface HeatmapRow {
  meter_id: string
  name: string
  status: MeterStatus
  anomaly: AnomalyRef | null
  days: DayPoint[]
}

export interface Baseline {
  meter_id: string
  window_days: number
  median: number[]
  p10: number[]
  p90: number[]
  daily_kwh: number
  k_factor: number
  k_mad: number
  k_tolerance: number
  voltage_band: [number, number]
  flagged_readings: FlaggedReading[] | null
}

export interface ForecastPoint {
  timestamp: string
  expected_kwh: number
  p10: number
  p90: number
  projected_kwh: number
}

export interface Forecast {
  meter_id: string
  method: string
  level_factor: number
  points: ForecastPoint[]
  expected_kwh: number
  projected_kwh: number
  impact: Impact | null
}

/** What a readings CSV contains and what importing it changes (POST /data/readings). */
export interface ImportResult {
  rows: number
  added: number
  replaced: number
  duplicates: number
  meters: string[]
  from: string
  to: string
  warnings: string[]
  applied: boolean
}
