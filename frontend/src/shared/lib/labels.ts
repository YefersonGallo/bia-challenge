import type { ActionKind, AnomalyStatus, AnomalyType, MeterStatus, Severity } from '@/shared/api/types'

/** Visual and textual metadata of each classification the engine produces. */
export interface TypeMeta {
  label: string
  short: string
  /** CSS color for the dark control room. */
  color: string
  /** CSS color for the light report paper. */
  paper: string
  /** Owner used in the action plan. */
  owner: string
}

export const TYPE_META: Record<AnomalyType, TypeMeta> = {
  REAL_ANOMALY: {
    label: 'Anomalía real',
    short: 'REAL',
    color: 'var(--color-real)',
    paper: 'var(--color-real-l)',
    owner: 'Mantenimiento eléctrico',
  },
  DATA_QUALITY: {
    label: 'Calidad de datos',
    short: 'DATOS',
    color: 'var(--color-data)',
    paper: 'var(--color-data-l)',
    owner: 'Medición',
  },
  EXPLAINABLE_ANOMALY: {
    label: 'Explicable',
    short: 'EXPLICABLE',
    color: 'var(--color-expl)',
    paper: 'var(--color-expl-l)',
    owner: 'Producción',
  },
  FALSE_POSITIVE: {
    label: 'Falso positivo',
    short: 'FALSO +',
    color: 'var(--color-fp)',
    paper: 'var(--color-fp-l)',
    owner: '—',
  },
}

export const SEVERITY_LABEL: Record<Severity, string> = { HIGH: 'Alta', MEDIUM: 'Media', LOW: 'Baja' }

export const METER_STATUS_META: Record<MeterStatus, { label: string; color: string }> = {
  OK: { label: 'Normal', color: 'var(--color-ok)' },
  ALERT: { label: 'Alerta', color: 'var(--color-expl)' },
  CRITICAL: { label: 'Crítico', color: 'var(--color-real)' },
}

/** Priority tag as shown in the design: P1, P2… from the engine's rank. */
export const priorityTag = (rank: number) => `P${rank}`

// --- lifecycle ----------------------------------------------------------------

/** Labels of the four lifecycle states, adapted to what "in progress" means per type. */
export function lifecycleLabels(type: AnomalyType): Record<AnomalyStatus, string> {
  const inProgress: Record<AnomalyType, string> = {
    REAL_ANOMALY: 'OT abierta',
    DATA_QUALITY: 'En validación',
    EXPLAINABLE_ANOMALY: 'Con producción',
    FALSE_POSITIVE: 'En curso',
  }
  return {
    OPEN: 'Nueva',
    ACKNOWLEDGED: 'Reconocida',
    IN_PROGRESS: inProgress[type],
    RESOLVED: type === 'FALSE_POSITIVE' ? 'Cerrada' : 'Resuelta',
  }
}

export const LIFECYCLE: AnomalyStatus[] = ['OPEN', 'ACKNOWLEDGED', 'IN_PROGRESS', 'RESOLVED']

/**
 * The next action the operator can take on an alarm, mirroring the backend state machine
 * (OPEN→ACKNOWLEDGED→IN_PROGRESS→RESOLVED; a false positive is closed straight away).
 */
export function nextAction(type: AnomalyType, status: AnomalyStatus): { to: AnomalyStatus; label: string } | null {
  if (status === 'RESOLVED') return null
  if (type === 'FALSE_POSITIVE') return { to: 'RESOLVED', label: 'Cerrar sin escalar' }
  switch (status) {
    case 'OPEN':
      return { to: 'ACKNOWLEDGED', label: 'Reconocer' }
    case 'ACKNOWLEDGED': {
      const label: Record<AnomalyType, string> = {
        REAL_ANOMALY: 'Crear orden de trabajo',
        DATA_QUALITY: 'Solicitar validación',
        EXPLAINABLE_ANOMALY: 'Confirmar con Producción',
        FALSE_POSITIVE: '',
      }
      return { to: 'IN_PROGRESS', label: label[type] }
    }
    case 'IN_PROGRESS':
      return { to: 'RESOLVED', label: 'Marcar resuelta' }
  }
}

/** Actions the operator can record from each state (the API applies the transition). */
export function availableActions(type: AnomalyType, status: AnomalyStatus): ActionKind[] {
  if (status === 'RESOLVED') return []
  const work: ActionKind = type === 'DATA_QUALITY' ? 'validate' : 'investigate'
  const close: ActionKind = type === 'FALSE_POSITIVE' ? 'dismiss' : 'resolve'
  if (type === 'FALSE_POSITIVE') return status === 'OPEN' ? ['acknowledge', close] : [close]
  switch (status) {
    case 'OPEN':
      return ['acknowledge', work, close]
    case 'ACKNOWLEDGED':
      return [work, close]
    default:
      return [close]
  }
}

// --- zones --------------------------------------------------------------------

export type Zone = 'Subestaciones' | 'Planta Norte' | 'Planta Sur' | 'Servicios'
export const ZONES: Zone[] = ['Subestaciones', 'Planta Norte', 'Planta Sur', 'Servicios']

/** Groups a meter location in one of the plant zones used by the operations grid. */
export function zoneOf(location: string): Zone {
  const l = location.toLowerCase()
  if (l.includes('subestación') || l.includes('subestacion')) return 'Subestaciones'
  if (l.includes('planta norte')) return 'Planta Norte'
  if (l.includes('planta sur')) return 'Planta Sur'
  return 'Servicios'
}

// --- events -------------------------------------------------------------------

const EVENT_LABEL: Record<string, string> = {
  PRODUCTION_LINE_START: 'Arranque de línea',
  OPERATIONAL_CHANGE: 'Cambio operativo',
  SCHEDULED_SHUTDOWN: 'Parada programada',
  SCHEDULED_OUTAGE: 'Parada programada',
  MAINTENANCE: 'Mantenimiento',
  DATA_QUALITY: 'Calidad de datos',
  UNKNOWN: 'Sin causa registrada',
}
export const eventLabel = (type: string) => EVENT_LABEL[type] ?? type.replaceAll('_', ' ').toLowerCase()
