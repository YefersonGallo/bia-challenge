import { meters } from '@/test/fixtures'
import { meterVerdict } from './verdict'

const byId = (id: string) => meters.find((m) => m.id === id)!

describe('meterVerdict', () => {
  it('uses the AI verdict with its priority when there is one', () => {
    expect(meterVerdict(byId('M-109'))).toMatchObject({ text: 'P1 · REAL', classified: true, attention: true })
    expect(meterVerdict(byId('M-112')).text).toBe('P2 · DATOS')
  })

  it('does not ask for attention on false positives or resolved anomalies', () => {
    expect(meterVerdict(byId('M-106'))).toMatchObject({ text: 'P4 · FALSO +', attention: false })
    const resolved = { ...byId('M-109'), anomaly: { ...byId('M-109').anomaly!, status: 'RESOLVED' as const } }
    expect(meterVerdict(resolved).attention).toBe(false)
  })

  it('falls back to the rule status before the analysis', () => {
    expect(meterVerdict({ status: 'ALERT', anomaly: null })).toMatchObject({ text: 'REGLA · SIN VEREDICTO', classified: false, attention: true })
    expect(meterVerdict({ status: 'OK', anomaly: null })).toMatchObject({ text: 'NORMAL', attention: false })
  })
})
