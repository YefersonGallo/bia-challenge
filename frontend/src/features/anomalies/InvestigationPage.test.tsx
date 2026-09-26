import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { anomalies, baselineM109, detailM109, detailM112, forecastM109, readingsM109 } from '@/test/fixtures'
import { mockApi, renderWithProviders } from '@/test/utils'
import { InvestigationPage } from './InvestigationPage'

const m109 = detailM109

function renderM109(extra: Record<string, unknown> = {}) {
  const api = mockApi({
    [`GET /anomalies/${m109.id}`]: m109,
    'GET /anomalies': anomalies,
    'GET /meters/M-109/readings': readingsM109,
    'GET /meters/M-109/baseline': baselineM109,
    'GET /meters/M-109/forecast': forecastM109,
    'GET /meters/M-109/events': m109.evidence.related_events,
    ...extra,
  })
  renderWithProviders(<InvestigationPage />, { route: `/anomalies/${m109.id}`, path: '/anomalies/:id' })
  return api
}

describe('InvestigationPage', () => {
  it('explains the anomaly with evidence, action and evaluated events', async () => {
    renderM109()
    expect(await screen.findByRole('heading', { name: 'M-109' })).toBeInTheDocument()
    expect(screen.getByText(/confianza alta \(0,97\) · prioridad 1 de 4/)).toBeInTheDocument()
    expect(screen.getByText('Investigar medidor e instalación.')).toBeInTheDocument()
    // The only record near the onset is an UNKNOWN event, which never explains a change.
    expect(screen.getByText('No operational event reported')).toBeInTheDocument()
    expect(screen.getByText(/tipo UNKNOWN: no describe una causa operativa/)).toBeInTheDocument()
    expect(screen.getByText('Corriente media')).toBeInTheDocument()
    expect(screen.getByText(/^Evidencia: /)).toBeInTheDocument()
  })

  it('shows the confidence breakdown and the projected impact', async () => {
    renderM109()
    await screen.findByRole('heading', { name: 'M-109' })
    expect(screen.getByRole('img', { name: 'Desglose de la confianza' }).children).toHaveLength(4)
    expect(screen.getByText('Magnitud')).toBeInTheDocument()
    expect(screen.getByText('IMPACTO PROYECTADO · 30 DÍAS')).toBeInTheDocument()
    expect(screen.getByText(/por encima del límite de 0,5/)).toBeInTheDocument()
  })

  it('draws the hourly series, the electrical panels and the diagnostics from the readings', async () => {
    renderM109()
    expect(await screen.findByText('DIAGNÓSTICO')).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'Relación física k a lo largo del tiempo' })).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'Corriente frente a consumo, antes y después del cambio' })).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'SERIE HORARIA' }))
    expect(screen.getByRole('img', { name: 'Consumo horario frente a la banda p10–p90 del baseline' })).toBeInTheDocument()
    expect(screen.getByText(/próximas 24 h/)).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'ELÉCTRICO' }))
    expect(screen.getByRole('img', { name: 'VOLTAJE · V (BANDA 209–231 V)' })).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'FACTOR DE POTENCIA (REFERENCIA 0,9)' })).toBeInTheDocument()
  })

  it('switches comparison views and shows the AI output as JSON', async () => {
    renderM109()
    await userEvent.click(await screen.findByRole('button', { name: 'HORARIO' }))
    expect(screen.getByText(/De noche consume 2,1×/)).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'VER JSON' }))
    const json = JSON.parse(screen.getByLabelText('Salida IA en JSON').textContent!)
    expect(json).toMatchObject({ meter_id: 'M-109', anomaly: true, type: 'REAL_ANOMALY', severity: 'HIGH' })
  })

  it('records an action with a note and shows it in the history', async () => {
    const done = {
      ...m109,
      status: 'IN_PROGRESS',
      actions: [{ id: 'X-1', anomaly_id: m109.id, action: 'investigate', note: 'cuadrilla asignada', status: 'IN_PROGRESS', actor: 'operador@vatio.demo', at: '2026-09-26T10:00:00Z' }],
    }
    let posted = false
    const { calls } = renderM109({
      [`GET /anomalies/${m109.id}`]: () => (posted ? done : m109),
      [`POST /anomalies/${m109.id}/actions`]: () => {
        posted = true
        return done
      },
    })
    await screen.findByRole('heading', { name: 'M-109' })
    await userEvent.type(screen.getByPlaceholderText(/Qué se revisó/), 'cuadrilla asignada')
    await userEvent.click(screen.getByRole('button', { name: 'Investigar' }))
    await waitFor(() =>
      expect(calls).toContainEqual({ method: 'POST', path: `/anomalies/${m109.id}/actions`, body: { action: 'investigate', note: 'cuadrilla asignada' } }),
    )
    const history = await screen.findByRole('list', { name: 'Historial de acciones' })
    expect(within(history).getByText('cuadrilla asignada')).toBeInTheDocument()
    expect(within(history).getByText(/operador@vatio.demo · Investigar/)).toBeInTheDocument()
  })

  it('marks the flagged readings of a data-quality anomaly', async () => {
    mockApi({
      [`GET /anomalies/${detailM112.id}`]: detailM112,
      'GET /anomalies': anomalies,
      'GET /meters/M-112/readings': readingsM109.map((r) => ({ ...r, meter_id: 'M-112' })),
      'GET /meters/M-112/baseline': { ...baselineM109, meter_id: 'M-112' },
    })
    renderWithProviders(<InvestigationPage />, { route: `/anomalies/${detailM112.id}`, path: '/anomalies/:id' })
    expect(await screen.findByText(/Los puntos rojos salen de la banda/)).toBeInTheDocument()
    expect(screen.getByText(/registro de calidad de datos: corrobora el diagnóstico/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Validar medidor' })).toBeInTheDocument()
  })
})
