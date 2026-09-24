import type { MeterSummary } from '@/shared/api/types'
import { METER_STATUS_META, priorityTag, TYPE_META } from './labels'

export interface MeterVerdict {
  /** Lamp / accent color. */
  color: string
  /** Short verdict for tables and tiles: "P1 · REAL", "NORMAL", "REGLA · SIN VEREDICTO". */
  text: string
  /** True when the AI already classified this meter. */
  classified: boolean
  /** True when the operator should look at it (rule flag or AI anomaly still open). */
  attention: boolean
}

/**
 * Combines the rule status of a meter with the AI verdict (if any) into what the UI shows.
 * The AI verdict wins: a rule alert explained by an event becomes "EXPLICABLE".
 */
export function meterVerdict(m: Pick<MeterSummary, 'status' | 'anomaly'>): MeterVerdict {
  if (m.anomaly) {
    const meta = TYPE_META[m.anomaly.type]
    return {
      color: meta.color,
      text: `${priorityTag(m.anomaly.rank)} · ${meta.short}`,
      classified: true,
      attention: m.anomaly.status !== 'RESOLVED' && m.anomaly.type !== 'FALSE_POSITIVE',
    }
  }
  if (m.status === 'OK') return { color: METER_STATUS_META.OK.color, text: 'NORMAL', classified: false, attention: false }
  return { color: METER_STATUS_META[m.status].color, text: 'REGLA · SIN VEREDICTO', classified: false, attention: true }
}
