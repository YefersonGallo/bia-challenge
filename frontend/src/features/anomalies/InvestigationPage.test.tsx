import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { anomalies, report } from '@/test/fixtures'
import { mockApi, renderWithProviders } from '@/test/utils'
import { InvestigationPage } from './InvestigationPage'

const m109 = report.findings[0]

describe('InvestigationPage', () => {
  it('explains the anomaly with evidence, action and events', async () => {
    mockApi({ [`GET /anomalies/${m109.id}`]: m109, 'GET /anomalies': anomalies })
    renderWithProviders(<InvestigationPage />, { route: `/anomalies/${m109.id}`, path: '/anomalies/:id' })

    expect(await screen.findByRole('heading', { name: 'M-109' })).toBeInTheDocument()
    expect(screen.getByText(/prioridad 1 de 4/)).toBeInTheDocument()
    expect(screen.getByText('Investigar medidor e instalación.')).toBeInTheDocument()
    // The only record near the onset is an UNKNOWN event, which does not explain the change.
    expect(screen.getByText('No operational event reported')).toBeInTheDocument()
    expect(screen.getByText(/no explica la dirección del cambio/)).toBeInTheDocument()
    expect(screen.getByText('Corriente media')).toBeInTheDocument()
  })

  it('switches comparison views and shows the AI output as JSON', async () => {
    mockApi({ [`GET /anomalies/${m109.id}`]: m109, 'GET /anomalies': anomalies })
    renderWithProviders(<InvestigationPage />, { route: `/anomalies/${m109.id}`, path: '/anomalies/:id' })

    await userEvent.click(await screen.findByRole('button', { name: 'HORARIO' }))
    expect(screen.getByText(/De noche consume 2,1×/)).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'VER JSON' }))
    const json = JSON.parse(screen.getByLabelText('Salida IA en JSON').textContent!)
    expect(json).toMatchObject({ meter_id: 'M-109', anomaly: true, type: 'REAL_ANOMALY', severity: 'HIGH' })
  })
})
