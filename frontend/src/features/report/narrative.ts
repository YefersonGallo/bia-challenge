// Builds the prose of the report from the structured findings. Pure functions: every
// figure comes from the API payload, nothing is invented on the client.
import type { AnomalyDetail, AnomalyType, Report } from '@/shared/api/types'
import { fmtNum, fmtPct } from '@/shared/lib/format'

const byType = (r: Report, t: AnomalyType) => r.findings.filter((f) => f.type === t)
const ids = (fs: AnomalyDetail[]) => joinEs(fs.map((f) => f.meter_id))

/** "A", "A y B", "A, B y C". */
export function joinEs(xs: string[]): string {
  if (xs.length <= 1) return xs[0] ?? ''
  return `${xs.slice(0, -1).join(', ')} y ${xs[xs.length - 1]}`
}

/** Executive summary: one paragraph per kind of finding, most urgent first. */
export function executiveSummary(r: Report): string[] {
  const out: string[] = []
  for (const f of byType(r, 'REAL_ANOMALY')) {
    out.push(
      `${f.meter_id} (${f.meter.name}) consume ${fmtPct(f.evidence.variation_pct)} frente a su baseline y ningún evento operativo lo explica. ` +
        `Es una anomalía real${f.rank === 1 ? ', la primera en prioridad,' : ''} y debe investigarse hoy.`,
    )
  }
  for (const f of byType(r, 'DATA_QUALITY')) {
    out.push(
      `${f.meter_id} no tiene un problema de consumo sino de medición: ${fmtNum(f.evidence.invalid_readings)} lecturas son físicamente imposibles ` +
        `y solo el ${fmtNum(f.evidence.physical_coherence * 100)}% cumple kWh ≈ V·I·PF. Hay que validar el medidor antes de usar sus datos.`,
    )
  }
  const explained = [...byType(r, 'EXPLAINABLE_ANOMALY'), ...byType(r, 'FALSE_POSITIVE')]
  if (explained.length) {
    const events = explained.flatMap((f) => f.evidence.related_events ?? []).map((e) => e.description.charAt(0).toLowerCase() + e.description.slice(1))
    out.push(
      `Los cambios de ${ids(explained)} se explican por eventos operativos registrados y no requieren escalamiento` +
        (events.length ? `: ${joinEs(events)}.` : '.'),
    )
  }
  if (out.length === 0) out.push('El análisis no encontró anomalías: todos los medidores se mantienen dentro de su banda.')
  return out
}

/** Headline of section 01: plant consumption vs. baseline and the biggest contributors. */
export function consumptionHeadline(r: Report): string {
  const s = r.summary
  const top = [...r.contributions].filter((c) => c.delta_kwh > 0).sort((a, b) => b.delta_kwh - a.delta_kwh).slice(0, 2)
  const who = top.length ? ` El aumento viene sobre todo de ${joinEs(top.map((c) => c.meter_id))}.` : ''
  return `La planta consumió ${fmtNum(s.current_kwh)} kWh en 7 días, ${fmtPct(s.variation_pct)} frente a su baseline.${who}`
}

/** Headline of section 03: count of findings by class. */
export function classificationHeadline(r: Report): string {
  const n = (t: AnomalyType) => byType(r, t).length
  const parts = [
    [n('REAL_ANOMALY'), 'anomalía real', 'anomalías reales'],
    [n('EXPLAINABLE_ANOMALY') + n('FALSE_POSITIVE'), 'cambio explicado por la operación', 'cambios explicados por la operación'],
    [n('DATA_QUALITY'), 'problema de medición', 'problemas de medición'],
  ] as const
  const txt = parts.filter(([c]) => c > 0).map(([c, one, many]) => `${c} ${c === 1 ? one : many}`)
  return txt.length ? `${joinEs(txt)}.`.replace(/^./, (c) => c.toUpperCase()) : 'Sin hallazgos.'
}
