import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { mockApi, renderWithProviders } from '@/test/utils'
import { ImportPage } from './ImportPage'

const result = (applied: boolean) => ({
  rows: 3,
  added: 2,
  replaced: 1,
  duplicates: 0,
  meters: ['M-101'],
  from: '2026-09-15T00:00:00Z',
  to: '2026-09-15T02:00:00Z',
  warnings: [],
  applied,
})

const csv = () => new File(['meter_id,timestamp,consumption_kwh,voltage_v,current_a,power_factor\n'], 'lecturas.csv', { type: 'text/csv' })

describe('CSV import', () => {
  it('previews the file, imports it only on confirmation and offers the analysis', async () => {
    const { calls } = mockApi({
      'GET /meters': [{ id: 'M-101' }],
      'POST /data/readings': (c: { path: string }) => result(!c.path.includes('dry_run')),
    })
    renderWithProviders(<ImportPage />)
    await userEvent.upload(screen.getByLabelText('Archivo CSV de lecturas'), csv())
    expect(await screen.findByText('VISTA PREVIA · TODAVÍA NO SE GUARDA')).toBeInTheDocument()
    const posts = () => calls.filter((c) => c.method === 'POST')
    expect(posts()).toHaveLength(1)
    expect(posts()[0].path).toBe('/data/readings?dry_run=true')

    await userEvent.click(screen.getByRole('button', { name: 'IMPORTAR 3 LECTURAS' }))
    expect(await screen.findByRole('status')).toHaveTextContent('LECTURAS IMPORTADAS')
    expect(posts()[1].path).toBe('/data/readings')
    expect(screen.getByRole('button', { name: 'EJECUTAR ANÁLISIS' })).toBeInTheDocument()
  })

  it('shows why a file is rejected and saves nothing', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).includes('/data/readings'))
        return new Response(JSON.stringify({ error: { code: 'INVALID_CSV', message: 'línea 4: voltage_v: valor vacío' } }), { status: 400 })
      if (String(input).includes('/meters')) return new Response('[]', { status: 200 })
      return new Response(JSON.stringify({ error: { code: 'NOT_FOUND', message: 'not found' } }), { status: 404 })
    })
    vi.stubGlobal('fetch', fetchMock)
    renderWithProviders(<ImportPage />)
    await userEvent.upload(screen.getByLabelText('Archivo CSV de lecturas'), csv())
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('línea 4: voltage_v: valor vacío'))
    expect(screen.queryByRole('button', { name: /IMPORTAR/ })).not.toBeInTheDocument()
    vi.unstubAllGlobals()
  })
})
