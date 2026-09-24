import type { MeterSummary } from '@/shared/api/types'
import { ZONES, zoneOf, type Zone } from '@/shared/lib/labels'

/** Groups meters by plant zone keeping the zone order of the design; empty zones are dropped. */
export function groupByZone(meters: MeterSummary[]): { zone: Zone; meters: MeterSummary[] }[] {
  return ZONES.map((zone) => ({
    zone,
    meters: meters.filter((m) => zoneOf(m.location) === zone).sort((a, b) => a.id.localeCompare(b.id)),
  })).filter((g) => g.meters.length > 0)
}

export interface ZoneGroup {
  zone: Zone
  meters: MeterSummary[]
}

/**
 * Packs zone groups into rows of `cols` tiles (first-fit decreasing) so the grid keeps
 * whole zones together without leaving holes. A zone wider than a row gets its own row.
 */
export function packRows(groups: ZoneGroup[], cols = 6): ZoneGroup[][] {
  const rows: { used: number; groups: ZoneGroup[] }[] = []
  const sorted = [...groups].sort((a, b) => b.meters.length - a.meters.length || ZONES.indexOf(a.zone) - ZONES.indexOf(b.zone))
  for (const g of sorted) {
    const size = Math.min(g.meters.length, cols)
    const row = rows.find((r) => r.used + size <= cols)
    if (row) {
      row.groups.push(g)
      row.used += size
    } else {
      rows.push({ used: size, groups: [g] })
    }
  }
  // Inside a row, keep the natural zone order.
  return rows.map((r) => r.groups.sort((a, b) => ZONES.indexOf(a.zone) - ZONES.indexOf(b.zone)))
}
