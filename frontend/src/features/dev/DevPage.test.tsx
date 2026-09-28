import { screen } from '@testing-library/react'
import { mockApi, renderWithProviders } from '@/test/utils'
import { useAuthStore } from '@/features/auth/authStore'
import { DevPage } from './DevPage'

describe('dev mode', () => {
  beforeEach(() => useAuthStore.setState({ token: null, user: null, notice: null }))

  it('shows the adapters this deploy runs, without a session', async () => {
    mockApi({ 'GET /health': { status: 'ok', ai: 'claude:claude-sonnet-4-5', store: 'postgres' } })
    renderWithProviders(<DevPage />)
    expect(await screen.findByText('Claude · claude-sonnet-4-5')).toBeInTheDocument()
    expect(screen.getByLabelText('Estado de este despliegue')).toHaveTextContent('PostgreSQL')
    expect(screen.getByRole('link', { name: /VOLVER AL LOGIN/ })).toHaveAttribute('href', '/login')
    expect(screen.getAllByRole('listitem').some((li) => li.textContent?.includes('EXPLICACIÓN'))).toBe(true)
  })

  it('says when explanations fall back to the template', async () => {
    mockApi({ 'GET /health': { status: 'ok', ai: 'template', store: 'memory' } })
    renderWithProviders(<DevPage />)
    expect(await screen.findByText('Plantilla (sin API key)')).toBeInTheDocument()
    expect(screen.getByLabelText('Estado de este despliegue')).toHaveTextContent('Memoria')
  })
})
