import { baselineM109, detailM109, detailM112, readingsM109 } from '@/test/fixtures'
import type { AnomalyDetail, EventItem } from '@/shared/api/types'
import { availableActions } from './labels'
import { baselineBand, confidenceLevel, cumulativeEnergy, deviationColor, eventVerdict, kOf, kSeries, scatterSplit } from './series'

const ev = (over: Partial<EventItem>): EventItem => ({
  id: 'E',
  meter_id: 'M-1',
  timestamp: '2026-09-08T00:00:00Z',
  type: 'SCHEDULED_OUTAGE',
  description: '',
  category: 'EXPLANATORY',
  expected_effect: 'DOWN',
  ...over,
})
const outage = {
  change_point_at: '2026-09-08T00:00:00Z',
  ended_at: '2026-09-08T12:00:00Z',
  evidence: { ...detailM109.evidence, shift_pct: -80, event_explains_shift: true },
} as AnomalyDetail

describe('series helpers', () => {
  it('computes k ≈ 1 for a coherent hourly reading', () => {
    const k = kOf(readingsM109[0])!
    expect(k).toBeGreaterThan(0.8)
    expect(k).toBeLessThan(1.3)
    expect(kSeries(readingsM109)).toHaveLength(readingsM109.length)
  })

  it('repeats the hour-of-day band and accumulates energy', () => {
    const band = baselineBand(readingsM109, baselineM109)
    expect(band[0][1]).toBe(baselineM109.p10[0])
    const { real, expected } = cumulativeEnergy(readingsM109, baselineM109.median)
    expect(real.at(-1)![1]).toBeGreaterThan(expected.at(-1)![1])
  })

  it('splits the scatter at the change point', () => {
    const { before, after } = scatterSplit(readingsM109, detailM109.change_point_at)
    expect(before.length + after.length).toBe(readingsM109.length)
    expect(before.length).toBeGreaterThan(0)
    expect(after.length).toBeGreaterThan(0)
  })

  it('labels confidence and colors deviations', () => {
    expect([0.97, 0.7, 0.4].map(confidenceLevel)).toEqual(['Alta', 'Media', 'Baja'])
    expect(deviationColor(2)).toBe('var(--color-raise)')
    expect(deviationColor(120)).toMatch(/^rgba\(255, 77, 61/)
    expect(deviationColor(-50)).toMatch(/^rgba\(79, 179, 255/)
  })
})

describe('eventVerdict', () => {
  it('rejects UNKNOWN events and keeps data-quality events informational', () => {
    expect(eventVerdict(detailM109.evidence.related_events![0], detailM109).explains).toBe(false)
    expect(eventVerdict(detailM112.evidence.related_events![0], detailM112).explains).toBeNull()
  })

  it('checks window, direction and outage duration', () => {
    expect(eventVerdict(ev({ duration_hours: 12 }), outage)).toMatchObject({ explains: true })
    expect(eventVerdict(ev({ duration_hours: 48 }), outage).text).toMatch(/duró 12 h y el evento anuncia 48 h/)
    expect(eventVerdict(ev({ expected_effect: 'UP' }), outage).text).toMatch(/no explica la dirección/)
    expect(eventVerdict(ev({ timestamp: '2026-09-08T09:00:00Z' }), outage).text).toMatch(/fuera de la ventana de ±3 h/)
  })
})

describe('availableActions', () => {
  it('offers the actions that fit each state', () => {
    expect(availableActions('REAL_ANOMALY', 'OPEN')).toEqual(['acknowledge', 'investigate', 'resolve'])
    expect(availableActions('DATA_QUALITY', 'ACKNOWLEDGED')).toEqual(['validate', 'resolve'])
    expect(availableActions('FALSE_POSITIVE', 'OPEN')).toEqual(['acknowledge', 'dismiss'])
    expect(availableActions('REAL_ANOMALY', 'RESOLVED')).toEqual([])
  })
})

describe('scatterSplit with a recovered episode', () => {
  it('keeps only the episode window as the change', () => {
    const { after } = scatterSplit(readingsM109, '2026-09-12T00:00:00Z', '2026-09-12T11:00:00Z')
    expect(after).toHaveLength(12)
  })
})
