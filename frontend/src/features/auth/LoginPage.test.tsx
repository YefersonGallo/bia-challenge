import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { mockApi, renderWithProviders } from '@/test/utils'
import { useAuthStore } from './authStore'
import { LoginPage } from './LoginPage'
import { RequireAuth } from './RequireAuth'

describe('authentication', () => {
  beforeEach(() => useAuthStore.setState({ token: null, user: null, notice: null }))

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

  it('confirms an explicit logout and links to dev mode', () => {
    useAuthStore.setState({ notice: 'user' })
    renderWithProviders(<LoginPage />)
    expect(screen.getByRole('status')).toHaveTextContent('Sesión cerrada')
    expect(screen.getByRole('link', { name: /DEV MODE/ })).toHaveAttribute('href', '/dev')
  })

  it('explains an expired session', () => {
    useAuthStore.setState({ notice: 'expired' })
    renderWithProviders(<LoginPage />)
    expect(screen.getByRole('status')).toHaveTextContent('Tu sesión expiró')
  })

  it('clears the notice on the next login', () => {
    useAuthStore.setState({ notice: 'user' })
    useAuthStore.getState().login('tok', { email: 'a@b.c', name: 'A' })
    expect(useAuthStore.getState().notice).toBeNull()
  })
})
