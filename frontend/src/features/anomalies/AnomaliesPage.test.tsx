import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { anomalies, run } from '@/test/fixtures'
import { mockApi, renderWithProviders } from '@/test/utils'
import { AnomaliesPage } from './AnomaliesPage'

describe('AnomaliesPage', () => {
  it('opens an investigation from the keyboard', async () => {
    mockApi({ 'GET /anomalies': anomalies, 'GET /ai/analysis/latest': run })
    renderWithProviders(<AnomaliesPage />, { route: '/anomalies', path: '/anomalies' })
    const row = await screen.findByRole('row', { name: 'Abrir investigación de M-109' })
    expect(row).toHaveTextContent('Alta')
    row.focus()
    await userEvent.keyboard('{Enter}')
    expect(await screen.findByTestId('elsewhere')).toBeInTheDocument()
  })
})
