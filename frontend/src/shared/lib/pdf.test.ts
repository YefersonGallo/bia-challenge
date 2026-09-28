import { paginate } from './pdf'

describe('PDF pagination', () => {
  it('ends each page at the last block that fits', () => {
    // 2500 px of content, pages of 1000 px, blocks ending every 300 px.
    const breaks = [300, 600, 900, 1200, 1500, 1800, 2100, 2400, 2500]
    expect(paginate(2500, 1000, breaks)).toEqual([
      [0, 900],
      [900, 1800],
      [1800, 2500],
    ])
  })

  it('cuts at full height only when no block ends in the lower half of the page', () => {
    expect(paginate(2200, 1000, [100, 2200])).toEqual([
      [0, 1000],
      [1000, 2000],
      [2000, 2200],
    ])
  })

  it('keeps a short document on one page', () => {
    expect(paginate(640, 1000, [200, 640])).toEqual([[0, 640]])
  })
})
