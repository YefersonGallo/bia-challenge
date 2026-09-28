import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AnalysisRun } from '@/shared/api/types'
import { run as completed } from '@/test/fixtures'
import { mockApi, renderWithProviders } from '@/test/utils'
import { useAnalysisUi } from './analysisStore'
import { useAnalysis } from './useAnalysis'

/** A run that advances one step on every poll, like the backend does. */
function progressingRun() {
  let polls = 0
  return (): AnalysisRun => {
    const done = Math.min(polls++, completed.steps.length)
    const finished = done === completed.steps.length
    return {
      ...completed,
      status: finished ? 'COMPLETED' : 'RUNNING',
      current_step: done,
      summary: finished ? completed.summary : undefined,
      steps: completed.steps.map((s, i) =>
        i < done ? s : { ...s, status: i === done ? 'RUNNING' : 'PENDING', result: undefined },
      ),
    }
  }
}

function Probe() {
  const a = useAnalysis()
  return (
    <div>
      <button onClick={() => a.start()}>start</button>
      <span data-testid="status">{a.run?.status ?? 'none'}</span>
      <span data-testid="progress">{a.progress}</span>
    </div>
  )
}

describe('useAnalysis', () => {
  beforeEach(() => useAnalysisUi.setState({ runId: null, stripOpen: false }))

  it('starts a run, polls its steps and refreshes derived data when it finishes', async () => {
    const next = progressingRun()
    const { calls } = mockApi({
      'POST /ai/analyze': () => next(),
      [`GET /ai/analysis/${completed.id}`]: () => next(),
    })
    const { client } = renderWithProviders(<Probe />)
    expect(await screen.findByTestId('status')).toHaveTextContent('none')

    await userEvent.click(screen.getByRole('button', { name: 'start' }))
    expect(useAnalysisUi.getState()).toMatchObject({ runId: completed.id, stripOpen: true })

    await waitFor(() => expect(screen.getByTestId('status')).toHaveTextContent('COMPLETED'), { timeout: 8000 })
    expect(screen.getByTestId('progress')).toHaveTextContent('100')
    const polls = calls.filter((c) => c.path === `/ai/analysis/${completed.id}`)
    expect(polls.length).toBeGreaterThanOrEqual(completed.steps.length)
    // On completion every derived query (meters, anomalies, report, latest run…) is invalidated.
    await waitFor(() => expect(client.getQueryState(['run', 'latest'])?.isInvalidated).toBe(true))
  })
})

describe('useAnalysis double click', () => {
  it('sends a single POST when start is called twice in the same tick', async () => {
    const { renderHook, act: hookAct } = await import('@testing-library/react')
    const { QueryClient, QueryClientProvider } = await import('@tanstack/react-query')
    const { mockApi } = await import('@/test/utils')
    const { useAnalysis } = await import('./useAnalysis')
    const { calls } = mockApi({ 'POST /ai/analyze': { id: 'A-1', status: 'RUNNING', steps: [], current_step: 0, started_at: '' } })
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { result } = renderHook(() => useAnalysis(), {
      wrapper: ({ children }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>,
    })
    hookAct(() => {
      result.current.start()
      result.current.start()
    })
    await new Promise((r) => setTimeout(r, 50))
    expect(calls.filter((c) => c.method === 'POST')).toHaveLength(1)
  })
})
