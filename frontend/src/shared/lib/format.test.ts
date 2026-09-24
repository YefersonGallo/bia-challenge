import { dayOf, fmtConf, fmtDayHour, fmtNum, fmtPct, fmtTime, variationTone } from './format'

describe('format (es-CO)', () => {
  it('groups thousands with dots and uses comma decimals', () => {
    expect(fmtNum(2180.4)).toBe('2.180')
    expect(fmtNum(13983.6)).toBe('13.984')
    expect(fmtNum(0.81, 2)).toBe('0,81')
    expect(fmtNum(1234567.891, 1)).toBe('1.234.567,9')
  })

  it('uses a real minus sign and never prints "−0"', () => {
    expect(fmtNum(-344)).toBe('−344')
    expect(fmtNum(-0.004, 2)).toBe('0,00')
  })

  it('handles missing values', () => {
    expect(fmtNum(null)).toBe('—')
    expect(fmtNum(Number.NaN)).toBe('—')
    expect(fmtPct(undefined)).toBe('—')
  })

  it('formats signed percentages like the engine texts', () => {
    expect(fmtPct(103.14)).toBe('+103,1%')
    expect(fmtPct(-38.9)).toBe('−38,9%')
    expect(fmtPct(0.01)).toBe('0,0%')
  })

  it('formats confidence and times in UTC', () => {
    expect(fmtConf(0.97)).toBe('0,97')
    expect(fmtTime('2026-09-08T06:00:00Z')).toBe('06:00')
    expect(fmtTime('nope')).toBe('—')
  })

  it('maps timestamps to days of the window', () => {
    expect(dayOf('2026-09-01T00:00:00Z')).toBe(1)
    expect(dayOf('2026-09-08T06:00:00Z')).toBe(8)
    expect(fmtDayHour('2026-09-10T00:00:00Z')).toBe('D10 · 00:00')
  })

  it('classifies the tone of a variation', () => {
    expect(variationTone(103)).toBe('up')
    expect(variationTone(-38)).toBe('down')
    expect(variationTone(4)).toBe('flat')
  })
})
