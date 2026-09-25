import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { meters, summary } from '@/test/fixtures'
import { mockApi, renderWithProviders } from '@/test/utils'
import { MetersPage } from './MetersPage'
import { useMeterFilters } from './filtersStore'

/** Server-side filtering is emulated so the test exercises the real query parameters. */
function serveMeters() {
  return mockApi({
    'GET /dashboard/summary': summary,
    'GET /meters': ({ path }: { path: string }) => {
      const p = new URL(path, 'http://x').searchParams
      let out = meters
      if (p.get('status')) out = out.filter((m) => m.status === p.get('status'))
      if (p.get('q')) out = out.filter((m) => m.id.toLowerCase().includes(p.get('q')!.toLowerCase()))
      if (p.get('sort') === 'consumption') out = [...out].sort((a, b) => b.current_kwh - a.current_kwh)
      return out
    },
  })
}

const rowIds = () =>
  screen
    .getAllByRole('row')
    .slice(1)
    .map((r) => within(r).getAllByRole('cell')[0].textContent?.match(/M-\d+/)?.[0])

describe('MetersPage', () => {
  beforeEach(() => useMeterFilters.setState({ status: 'ALL', q: '', sort: 'severity' }))

  it('lists meters ordered by severity with their AI verdict', async () => {
    serveMeters()
    renderWithProviders(<MetersPage />)
    await waitFor(() => expect(rowIds()).toHaveLength(12))
    expect(rowIds().slice(0, 4)).toEqual(['M-109', 'M-112', 'M-104', 'M-106'])
    expect(screen.getByText('P1 · REAL')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'TODOS 12' })).toHaveAttribute('aria-pressed', 'true')
  })

  it('filters by status and searches by meter_id through the API', async () => {
    const { calls } = serveMeters()
    renderWithProviders(<MetersPage />)
    await waitFor(() => expect(rowIds()).toHaveLength(12))

    await userEvent.click(screen.getByRole('button', { name: 'CRÍTICAS 1' }))
    await waitFor(() => expect(rowIds()).toEqual(['M-109']))
    expect(calls.some((c) => c.path.includes('status=CRITICAL'))).toBe(true)

    await userEvent.click(screen.getByRole('button', { name: 'LIMPIAR' }))
    await userEvent.type(screen.getByRole('searchbox', { name: 'Buscar por meter_id' }), 'm-11')
    await waitFor(() => expect(rowIds()).toEqual(['M-112', 'M-111', 'M-110']))
  })

  it('sorts by consumption', async () => {
    const { calls } = serveMeters()
    renderWithProviders(<MetersPage />)
    await waitFor(() => expect(rowIds()).toHaveLength(12))
    await userEvent.click(screen.getByRole('button', { name: 'CONSUMO' }))
    await waitFor(() => expect(rowIds()[0]).toBe('M-109'))
    expect(calls.at(-1)?.path).toContain('sort=consumption')
    expect(rowIds()[1]).toBe('M-104')
  })

  it('shows an empty state that clears the filters', async () => {
    serveMeters()
    renderWithProviders(<MetersPage />)
    await userEvent.type(await screen.findByRole('searchbox', { name: 'Buscar por meter_id' }), 'zzz')
    expect(await screen.findByText(/Ningún medidor coincide/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Limpiar filtros' }))
    await waitFor(() => expect(rowIds()).toHaveLength(12))
  })
})
