import type { Anomaly } from '@/shared/api/types'

/** The AI output exactly in the format requested by the challenge. */
export interface AiOutput {
  meter_id: string
  anomaly: boolean
  type: Anomaly['type']
  severity: Anomaly['severity']
  confidence: number
  reason: string
  recommended_action: string
}

export function toAiOutput(a: Anomaly): AiOutput {
  return {
    meter_id: a.meter_id,
    anomaly: a.type !== 'FALSE_POSITIVE',
    type: a.type,
    severity: a.severity,
    confidence: a.confidence,
    reason: a.reason,
    recommended_action: a.recommended_action,
  }
}
