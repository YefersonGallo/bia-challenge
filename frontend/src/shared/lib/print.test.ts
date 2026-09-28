import { BEFORE_PRINT, printPage } from './print'

it('announces the print before opening the dialog', () => {
  vi.useFakeTimers()
  const order: string[] = []
  const print = vi.spyOn(window, 'print').mockImplementation(() => order.push('print'))
  const onBefore = () => order.push('before')
  window.addEventListener(BEFORE_PRINT, onBefore)
  printPage()
  expect(order).toEqual(['before'])
  vi.runAllTimers()
  expect(order).toEqual(['before', 'print'])
  window.removeEventListener(BEFORE_PRINT, onBefore)
  print.mockRestore()
  vi.useRealTimers()
})
