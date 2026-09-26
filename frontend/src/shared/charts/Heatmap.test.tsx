import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { heatmap } from '@/test/fixtures'
import { Heatmap } from './Heatmap'

describe('Heatmap', () => {
  it('draws one row per meter and one cell per day, and selects a row', async () => {
    const onSelect = vi.fn()
    render(<Heatmap rows={heatmap} onSelect={onSelect} />)
    const table = screen.getByRole('table', { name: 'Mapa de calor de desviación diaria' })
    expect(table.querySelectorAll('tbody tr')).toHaveLength(12)
    expect(table.querySelectorAll('tbody tr:first-child td')).toHaveLength(14)
    expect(screen.getByTitle(/M-109 · D14: \+1\d\d,\d% frente al baseline/)).toBeInTheDocument()
    expect(screen.getByTitle(/M-112 · D13: .* 16 lecturas marcadas/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Ver M-109' }))
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ meter_id: 'M-109' }))
  })
})
