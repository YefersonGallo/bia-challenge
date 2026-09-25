import { anomalies, report } from '@/test/fixtures'
import { toAiOutput } from '@/features/anomalies/aiOutput'
import { classificationHeadline, consumptionHeadline, executiveSummary, joinEs } from './narrative'

describe('report narrative', () => {
  it('joins lists in Spanish', () => {
    expect(joinEs([])).toBe('')
    expect(joinEs(['A'])).toBe('A')
    expect(joinEs(['A', 'B', 'C'])).toBe('A, B y C')
  })

  it('writes the executive summary from the findings, most urgent first', () => {
    const [real, dq, rest] = executiveSummary(report)
    expect(real).toMatch(/^M-109 \(Compresor línea 3\) consume \+110,7% .* anomalía real, la primera en prioridad/)
    expect(dq).toMatch(/^M-112 .* 17 lecturas son físicamente inconsistentes desde el día 13/)
    expect(rest).toMatch(/^Los cambios de M-104 y M-106 se explican por eventos operativos/)
    expect(rest).toContain('«Scheduled maintenance outage for 12 hours»')
  })

  it('handles a report without findings', () => {
    expect(executiveSummary({ ...report, findings: [] })).toEqual([
      'El análisis no encontró anomalías: todos los medidores se mantienen dentro de su banda.',
    ])
  })

  it('summarises consumption and classification', () => {
    expect(consumptionHeadline(report)).toBe(
      'En las últimas 24 h la planta consumió 12.503 kWh, +16,2% frente a su baseline diario. El aumento viene sobre todo de M-109 y M-104.',
    )
    expect(classificationHeadline(report)).toBe('1 anomalía real, 2 cambios explicados por la operación y 1 problema de medición.')
  })
})

describe('AI output format of the challenge', () => {
  it('exposes exactly the requested fields', () => {
    const out = toAiOutput(anomalies[0])
    expect(Object.keys(out)).toEqual(['meter_id', 'anomaly', 'type', 'severity', 'confidence', 'reason', 'recommended_action'])
    expect(out).toMatchObject({ meter_id: 'M-109', anomaly: true, type: 'REAL_ANOMALY', severity: 'HIGH' })
  })

  it('marks false positives as anomaly: false', () => {
    const fp = anomalies.find((a) => a.type === 'FALSE_POSITIVE')!
    expect(toAiOutput(fp)).toMatchObject({ meter_id: 'M-106', anomaly: false, severity: 'LOW' })
  })
})
