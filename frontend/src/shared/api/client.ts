import { useAuthStore } from '@/features/auth/authStore'

export class ApiError extends Error {
  status: number
  code: string
  constructor(status: number, message: string, code = 'ERROR') {
    super(message)
    this.status = status
    this.code = code
  }
}

/** The API answers errors as {"error": {"code", "message"}}; older builds sent {"error": "…"}. */
function toError(status: number, statusText: string, body: unknown): ApiError {
  const err = (body as { error?: unknown } | null)?.error
  if (err && typeof err === 'object') {
    const { code, message } = err as { code?: string; message?: string }
    return new ApiError(status, message ?? statusText, code)
  }
  return new ApiError(status, typeof err === 'string' ? err : statusText)
}

export const API_BASE = import.meta.env.VITE_API_URL ?? '/api'

/** Thin fetch wrapper: JSON in/out, bearer token, typed errors, logout on 401. */
export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const token = useAuthStore.getState().token
  const res = await fetch(API_BASE + path, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...init.headers,
    },
  })
  if (res.status === 401 && token) useAuthStore.getState().logout('expired')
  const text = await res.text()
  const body = text ? JSON.parse(text) : null
  if (!res.ok) throw toError(res.status, res.statusText, body)
  return body as T
}

export const qs = (params: Record<string, string | undefined>) => {
  const p = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) if (v) p.set(k, v)
  const s = p.toString()
  return s ? `?${s}` : ''
}
