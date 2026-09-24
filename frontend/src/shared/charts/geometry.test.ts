import { areaPath, bandPath, extent, linear, linePath, mean, sumSeries } from './geometry'

describe('chart geometry', () => {
  it('maps a domain onto a range (inverted y works too)', () => {
    const y = linear([0, 100], [200, 0])
    expect(y(0)).toBe(200)
    expect(y(50)).toBe(100)
    expect(y(100)).toBe(0)
  })

  it('maps a degenerate domain to the middle', () => {
    expect(linear([5, 5], [0, 10])(5)).toBe(5)
  })

  it('computes padded extents, optionally anchored at zero', () => {
    expect(extent([[10, 20]], { pad: 0 })).toEqual([10, 20])
    expect(extent([[10, 20]], { pad: 0, zero: true })).toEqual([0, 20])
    expect(extent([[]])).toEqual([0, 1])
    const [lo, hi] = extent([[100, 100]])
    expect(lo).toBeLessThan(100)
    expect(hi).toBeGreaterThan(100)
  })

  it('builds SVG paths', () => {
    expect(linePath([[0, 0], [10, 5.25]])).toBe('M0 0 L10 5.3')
    expect(bandPath([0, 10], [1, 2], [3, 4])).toBe('M0 1 L10 2 L10 4 L0 3 Z')
    expect(areaPath([[0, 5], [10, 2]], 20)).toBe('M0 5 L10 2 L10 20 L0 20 Z')
    expect(bandPath([], [], [])).toBe('')
  })

  it('sums series element-wise and averages', () => {
    expect(sumSeries([[1, 2, 3], [10, 20]])).toEqual([11, 22, 3])
    expect(mean([2, 4])).toBe(3)
    expect(mean([])).toBe(0)
  })
})
