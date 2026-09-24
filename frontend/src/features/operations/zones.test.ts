import { meters } from '@/test/fixtures'
import { groupByZone, packRows } from './zones'

describe('operations grid zones', () => {
  it('groups the 12 meters by zone, sorted by id', () => {
    const groups = groupByZone(meters)
    expect(groups.map((g) => [g.zone, g.meters.length])).toEqual([
      ['Subestaciones', 2],
      ['Planta Norte', 3],
      ['Planta Sur', 3],
      ['Servicios', 4],
    ])
    expect(groups[1].meters.map((m) => m.id)).toEqual(['M-101', 'M-104', 'M-109'])
  })

  it('packs whole zones into full rows of 6 tiles', () => {
    const rows = packRows(groupByZone(meters))
    expect(rows.map((r) => r.map((g) => g.zone))).toEqual([
      ['Subestaciones', 'Servicios'],
      ['Planta Norte', 'Planta Sur'],
    ])
    for (const r of rows) expect(r.reduce((n, g) => n + g.meters.length, 0)).toBe(6)
  })

  it('gives a zone wider than a row its own row', () => {
    const wide = [{ zone: 'Servicios' as const, meters: meters.slice(0, 8) }]
    expect(packRows(wide)).toHaveLength(1)
  })
})
