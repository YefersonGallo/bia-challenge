import type { AnomalyStatus, AnomalyType } from '@/shared/api/types'
import { eventLabel, lifecycleLabels, nextAction, priorityTag, zoneOf } from './labels'

describe('lifecycle', () => {
  it('names the in-progress state after what it means for each type', () => {
    expect(lifecycleLabels('REAL_ANOMALY').IN_PROGRESS).toBe('OT abierta')
    expect(lifecycleLabels('DATA_QUALITY').IN_PROGRESS).toBe('En validación')
    expect(lifecycleLabels('EXPLAINABLE_ANOMALY').IN_PROGRESS).toBe('Con producción')
    expect(lifecycleLabels('FALSE_POSITIVE').RESOLVED).toBe('Cerrada')
  })

  it('walks OPEN → ACKNOWLEDGED → IN_PROGRESS → RESOLVED for real anomalies', () => {
    const path: AnomalyStatus[] = []
    let status: AnomalyStatus = 'OPEN'
    for (let next = nextAction('REAL_ANOMALY', status); next; next = nextAction('REAL_ANOMALY', status)) {
      status = next.to
      path.push(status)
    }
    expect(path).toEqual(['ACKNOWLEDGED', 'IN_PROGRESS', 'RESOLVED'])
  })

  it('offers the type-specific action once acknowledged', () => {
    const cases: [AnomalyType, string][] = [
      ['REAL_ANOMALY', 'Crear orden de trabajo'],
      ['DATA_QUALITY', 'Solicitar validación'],
      ['EXPLAINABLE_ANOMALY', 'Confirmar con Producción'],
    ]
    for (const [t, label] of cases) expect(nextAction(t, 'ACKNOWLEDGED')).toEqual({ to: 'IN_PROGRESS', label })
  })

  it('closes a false positive directly (the backend allows OPEN → RESOLVED)', () => {
    expect(nextAction('FALSE_POSITIVE', 'OPEN')).toEqual({ to: 'RESOLVED', label: 'Cerrar sin escalar' })
    expect(nextAction('FALSE_POSITIVE', 'RESOLVED')).toBeNull()
  })
})

describe('zones and labels', () => {
  it('groups locations into plant zones', () => {
    expect(zoneOf('Subestación B')).toBe('Subestaciones')
    expect(zoneOf('Planta Norte · L3')).toBe('Planta Norte')
    expect(zoneOf('Planta Sur')).toBe('Planta Sur')
    expect(zoneOf('Edificio administrativo')).toBe('Servicios')
  })

  it('formats priorities and event types', () => {
    expect(priorityTag(1)).toBe('P1')
    expect(eventLabel('SCHEDULED_SHUTDOWN')).toBe('Parada programada')
    expect(eventLabel('NEW_THING')).toBe('new thing')
  })
})
