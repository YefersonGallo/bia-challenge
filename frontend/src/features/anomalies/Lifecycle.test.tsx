import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { mockApi, renderWithProviders } from '@/test/utils'
import { LifecycleBar, NextActionButton } from './Lifecycle'

describe('anomaly lifecycle UI', () => {
  it('marks the current step of the lifecycle', () => {
    renderWithProviders(<LifecycleBar type="REAL_ANOMALY" status="IN_PROGRESS" />)
    expect(screen.getByText('OT abierta').closest('li')).toHaveAttribute('aria-current', 'step')
    expect(screen.getAllByRole('listitem')).toHaveLength(4)
  })

  it('shows only new/closed for a false positive', () => {
    renderWithProviders(<LifecycleBar type="FALSE_POSITIVE" status="OPEN" />)
    expect(screen.getAllByRole('listitem').map((li) => li.textContent)).toEqual(['Nueva', 'Cerrada'])
  })

  it('PATCHes the next status when the action is clicked', async () => {
    const { calls } = mockApi({ 'PATCH /anomalies/A-1-M-109': { id: 'A-1-M-109', status: 'ACKNOWLEDGED' } })
    renderWithProviders(<NextActionButton id="A-1-M-109" type="REAL_ANOMALY" status="OPEN" />)
    await userEvent.click(screen.getByRole('button', { name: 'Reconocer' }))
    await waitFor(() => expect(calls).toContainEqual({ method: 'PATCH', path: '/anomalies/A-1-M-109', body: { status: 'ACKNOWLEDGED' } }))
  })

  it('shows a resolved mark instead of an action at the end', () => {
    renderWithProviders(<NextActionButton id="x" type="DATA_QUALITY" status="RESOLVED" />)
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
    expect(screen.getByText('✓ resuelta')).toBeInTheDocument()
  })
})
