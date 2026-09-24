import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AnalysisRun } from '@/shared/api/types'
import { run as completed } from '@/test/fixtures'
import { AnalysisStrip } from './AnalysisStrip'
import { runProgress } from './useAnalysis'

const running: AnalysisRun = {
  ...completed,
  status: 'RUNNING',
  current_step: 3,
  summary: undefined,
  steps: completed.steps.map((s, i) =>
    i < 3 ? s : i === 3 ? { ...s, status: 'RUNNING', result: undefined } : { ...s, status: 'PENDING', result: undefined },
  ),
}

const noop = () => {}

describe('AnalysisStrip', () => {
  it('shows every step with its live status while running', () => {
    render(<AnalysisStrip run={running} onClose={noop} onGoAnomalies={noop} onGoReport={noop} />)
    expect(screen.getByText('Analizando · paso 4 de 7')).toBeInTheDocument()
    expect(screen.getByText('RUN AI ANALYSIS · 43%')).toBeInTheDocument()
    const steps = screen.getAllByRole('listitem')
    expect(steps.map((s) => s.dataset.status)).toEqual(['COMPLETED', 'COMPLETED', 'COMPLETED', 'RUNNING', 'PENDING', 'PENDING', 'PENDING'])
    expect(screen.getByText('En curso…')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'VER ANOMALÍAS' })).not.toBeInTheDocument()
  })

  it('shows the headline and the next steps when completed', async () => {
    const goAnomalies = vi.fn()
    const close = vi.fn()
    render(<AnalysisStrip run={completed} onClose={close} onGoAnomalies={goAnomalies} onGoReport={noop} />)
    expect(screen.getByText('4 anomalías detectadas · 2 requieren atención prioritaria')).toBeInTheDocument()
    expect(screen.getByText('2 de prioridad alta · M-109 primero')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'VER ANOMALÍAS' }))
    await userEvent.click(screen.getByRole('button', { name: 'Cerrar estado del análisis' }))
    expect(goAnomalies).toHaveBeenCalledOnce()
    expect(close).toHaveBeenCalledOnce()
  })

  it('computes progress from completed steps', () => {
    expect(runProgress(null)).toBe(0)
    expect(runProgress(running)).toBe(43)
    expect(runProgress(completed)).toBe(100)
  })
})
