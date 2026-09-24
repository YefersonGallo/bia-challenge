import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { mockApi, renderWithProviders } from '@/test/utils'
import { useAuthStore } from './authStore'
import { LoginPage } from './LoginPage'
import { RequireAuth } from './RequireAuth'

describe('authentication', () => {
  beforeEach(() => useAuthStore.setState({ token: null, user: null }))

  it('logs in and stores the session', async () => {
    const { calls } = mockApi({
      'POST /auth/login': { token: 'tok', expires_at: '2026-09-25T00:00:00Z', user: { email: 'operador@vatio.demo', name: 'Operador' } },
    })
    renderWithProviders(<LoginPage />)
    await userEvent.type(screen.getByLabelText('CONTRASEÑA'), 'demo')
    await userEvent.click(screen.getByRole('button', { name: 'ENTRAR' }))
    await waitFor(() => expect(useAuthStore.getState().token).toBe('tok'))
    expect(calls[0].body).toEqual({ email: 'operador@vatio.demo', password: 'demo' })
  })

  it('shows an error on wrong credentials', async () => {
    mockApi({})
    renderWithProviders(<LoginPage />)
    await userEvent.type(screen.getByLabelText('CONTRASEÑA'), 'bad')
    await userEvent.click(screen.getByRole('button', { name: 'ENTRAR' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Credenciales inválidas')
    expect(useAuthStore.getState().token).toBeNull()
  })

  it('protects private routes', () => {
    renderWithProviders(
      <RequireAuth>
        <span>privado</span>
      </RequireAuth>,
      { route: '/meters', path: '/meters' },
    )
    expect(screen.queryByText('privado')).not.toBeInTheDocument()
    expect(screen.getByTestId('elsewhere')).toBeInTheDocument()
  })
})
