import type { ReactElement } from 'react'
import { render } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { vi } from 'vitest'

export interface ApiCall {
  method: string
  path: string
  body: unknown
}

type Handler = (call: ApiCall) => unknown

/**
 * Replaces fetch with a tiny router over the API: keys are "METHOD /path" (query string
 * excluded) and handlers return the JSON body. Unknown routes answer 404.
 */
export function mockApi(routes: Record<string, Handler | unknown>) {
  const calls: ApiCall[] = []
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), 'http://localhost')
    const method = (init?.method ?? 'GET').toUpperCase()
    const path = url.pathname.replace(/^\/api/, '')
    const call: ApiCall = { method, path: path + url.search, body: typeof init?.body === 'string' ? JSON.parse(init.body) : init?.body }
    calls.push(call)
    const key = `${method} ${path}`
    if (!(key in routes)) return new Response(JSON.stringify({ error: 'not found' }), { status: 404 })
    const h = routes[key]
    const body = typeof h === 'function' ? (h as Handler)(call) : h
    return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
  })
  vi.stubGlobal('fetch', fetchMock)
  return { calls, fetchMock }
}

/** Renders inside a fresh QueryClient and a MemoryRouter at `route` (matched by `path`). */
export function renderWithProviders(ui: ReactElement, { route = '/', path = '*' }: { route?: string; path?: string } = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return {
    client,
    ...render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={[route]}>
          <Routes>
            <Route path={path} element={ui} />
            <Route path="*" element={<div data-testid="elsewhere" />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    ),
  }
}
