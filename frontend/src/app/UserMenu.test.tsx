import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient } from '@tanstack/react-query'
import { renderWithProviders } from '@/test/utils'
import { useAuthStore } from '@/features/auth/authStore'
import { onSessionEnd } from '@/features/auth/session'
import { useAnalysisUi } from '@/features/analysis/analysisStore'
import { UserMenu } from './UserMenu'

describe('user menu', () => {
  beforeEach(() => useAuthStore.setState({ token: 'tok', user: { email: 'operador@vatio.demo', name: 'Operador Demo' }, notice: null }))

  it('opens a menu with the session and an explicit logout', async () => {
    renderWithProviders(<UserMenu />)
    expect(screen.queryByRole('menu')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /Menú de usuario/ }))
    expect(screen.getByRole('menu')).toHaveTextContent('operador@vatio.demo')
    expect(screen.getByRole('menuitem', { name: /Dev mode/ })).toHaveAttribute('href', '/dev')
    await userEvent.click(screen.getByRole('menuitem', { name: /Cerrar sesión/ }))
    expect(useAuthStore.getState()).toMatchObject({ token: null, notice: 'user' })
  })

  it('closes with Escape without logging out', async () => {
    renderWithProviders(<UserMenu />)
    await userEvent.click(screen.getByRole('button', { name: /Menú de usuario/ }))
    await userEvent.keyboard('{Escape}')
    expect(screen.queryByRole('menu')).not.toBeInTheDocument()
    expect(useAuthStore.getState().token).toBe('tok')
  })
})

describe('end of session', () => {
  it('drops cached server data and the analysis strip, not the analyses', () => {
    const client = new QueryClient()
    const stop = onSessionEnd(client)
    client.setQueryData(['anomalies'], [{ id: 'x' }])
    useAnalysisUi.getState().follow('A-1')
    useAuthStore.getState().logout('user')
    expect(client.getQueryData(['anomalies'])).toBeUndefined()
    expect(useAnalysisUi.getState()).toMatchObject({ runId: null, stripOpen: false })
    stop()
  })
})
