import { isFiltered, toMeterQuery, useMeterFilters } from './filtersStore'

describe('meter filters', () => {
  beforeEach(() => useMeterFilters.setState({ status: 'ALL', q: '', sort: 'severity' }))

  it('translates UI filters to the API query', () => {
    expect(toMeterQuery({ status: 'ALL', q: '  ', sort: 'severity' })).toEqual({ status: undefined, q: undefined, sort: 'severity' })
    expect(toMeterQuery({ status: 'CRITICAL', q: ' m-109 ', sort: 'variation' })).toEqual({ status: 'CRITICAL', q: 'm-109', sort: 'variation' })
  })

  it('knows when something is filtered', () => {
    expect(isFiltered({ status: 'ALL', q: '' })).toBe(false)
    expect(isFiltered({ status: 'OK', q: '' })).toBe(true)
    expect(isFiltered({ status: 'ALL', q: 'M-1' })).toBe(true)
  })

  it('clears status and search but keeps the sort', () => {
    const s = useMeterFilters.getState()
    s.setStatus('ALERT')
    s.setQ('M-104')
    s.setSort('consumption')
    useMeterFilters.getState().clear()
    expect(useMeterFilters.getState()).toMatchObject({ status: 'ALL', q: '', sort: 'consumption' })
  })
})
