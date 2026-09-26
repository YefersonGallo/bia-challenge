// Contract of the live replay (backend/internal/live): SSE on /api/stream.
import type { AnomalyType, MeterStatus, Severity } from '@/shared/api/types'

export interface LivePoint {
  meter_id: string
  timestamp: string
  consumption_kwh: number
  voltage_v: number
  current_a: number
  power_factor: number
}

export interface LiveState {
  clock: string
  start: string
  end: string
  hour: number
  total_hours: number
  running: boolean
  finished: boolean
  speed: number
  step_ms: number
  last_tick_ms: number
}

export type AlertState = 'CANDIDATE' | 'CONFIRMED' | 'CLOSED'

export interface LiveAlert {
  id: string
  meter_id: string
  state: AlertState
  type: AnomalyType
  severity: Severity
  confidence: number
  priority_score: number
  reason: string
  change_point_at?: string
  opened_at: string
  confirmed_at?: string
  updated_at: string
  closed_at?: string
  closure?: string
}

export type AlertChangeKind = 'candidate' | 'confirmed' | 'updated' | 'closed'

export interface AlertChange {
  change: AlertChangeKind
  alert: LiveAlert
}

export interface MeterLive {
  id: string
  name: string
  status: MeterStatus
  recent: LivePoint[]
}

export interface Snapshot {
  state: LiveState
  meters: MeterLive[]
  alerts: LiveAlert[]
}

export interface Tick {
  state: LiveState
  readings: LivePoint[]
  status: Record<string, MeterStatus>
}

export type LiveControl = 'start' | 'pause' | 'reset' | 'speed'
