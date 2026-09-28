import { useState, type FormEvent } from 'react'
import { Link, Navigate, useLocation, useNavigate } from 'react-router-dom'
import { useLogin } from '@/shared/api/queries'
import { Lamp } from '@/shared/ui/primitives'
import { useAuthStore } from './authStore'

const PREVIEW = [
  { id: 'M-109', tag: 'P1 · REAL', color: 'var(--color-real)', bg: '#1a0d0b' },
  { id: 'M-112', tag: 'P2 · DATOS', color: 'var(--color-data)' },
  { id: 'M-104', tag: 'P3 · EXPLICABLE', color: 'var(--color-expl)' },
  { id: 'M-106', tag: 'P4 · FALSO +', color: 'var(--color-fp)' },
]
const FLOW = ['DATOS', 'ANÁLISIS', 'ANOMALÍA', 'EXPLICACIÓN', 'PRIORIZACIÓN', 'ACCIÓN']

export function LoginPage() {
  const token = useAuthStore((s) => s.token)
  const setSession = useAuthStore((s) => s.login)
  const notice = useAuthStore((s) => s.notice)
  const navigate = useNavigate()
  const location = useLocation()
  const login = useLogin()
  const [email, setEmail] = useState('operador@vatio.demo')
  const [password, setPassword] = useState('')

  const from = (location.state as { from?: string } | null)?.from ?? '/'
  if (token) return <Navigate to={from} replace />

  const submit = (e: FormEvent) => {
    e.preventDefault()
    login.mutate(
      { email, password },
      {
        onSuccess: (r) => {
          setSession(r.token, r.user)
          navigate(from, { replace: true })
        },
      },
    )
  }

  return (
    <div className="grid min-h-full grid-cols-1 lg:grid-cols-[minmax(0,1fr)_500px]">
      <div
        className="hidden flex-col justify-between px-16 py-12 lg:flex"
        style={{
          backgroundImage:
            'linear-gradient(rgba(79,179,255,.05) 1px, transparent 1px), linear-gradient(90deg, rgba(79,179,255,.05) 1px, transparent 1px)',
          backgroundSize: '32px 32px',
        }}
      >
        <div className="flex items-center gap-3.5 font-mono text-[11px] tracking-[0.1em] text-muted">
          <span className="font-display text-[22px] font-bold tracking-normal text-ink">VATIO</span>
          <span>AI ENERGY MANAGEMENT</span>
        </div>
        <div className="flex flex-col gap-6">
          <h1 className="m-0 max-w-[720px] font-display text-[68px] leading-none font-semibold tracking-[-0.035em]">
            Qué medidor revisar primero, <span className="text-accent">y por qué.</span>
          </h1>
          <p className="m-0 max-w-[620px] text-lg leading-relaxed text-soft">
            La IA analiza consumo, voltaje, corriente y factor de potencia de cada medidor, separa las fallas reales de los eventos
            operativos y de los errores de medición, y recomienda la acción.
          </p>
          <div className="grid max-w-[700px] grid-cols-4 gap-2" aria-hidden>
            {PREVIEW.map((p) => (
              <div key={p.id} className="flex flex-col gap-1 rounded-md border p-3" style={{ borderColor: p.color, background: p.bg ?? 'var(--color-panel)' }}>
                <span className="flex justify-between font-mono text-xs font-semibold">
                  {p.id}
                  <Lamp color={p.color} />
                </span>
                <span className="font-mono text-[10px]" style={{ color: p.color }}>
                  {p.tag}
                </span>
              </div>
            ))}
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2 font-mono text-[11px] tracking-[0.06em] text-muted">
          {FLOW.map((s, i) => (
            <span key={s} className="flex items-center gap-2">
              {i > 0 && <span className="text-line-2">→</span>}
              <span className={i === FLOW.length - 1 ? 'text-accent' : undefined}>{s}</span>
            </span>
          ))}
        </div>
      </div>

      <form onSubmit={submit} className="flex flex-col justify-center gap-5 border-l border-line bg-panel-2 px-8 py-12 sm:px-14" aria-label="Iniciar sesión">
        <div className="flex flex-col gap-1.5">
          <span className="font-mono text-[11px] tracking-[0.1em] text-muted">CENTRO DE CONTROL · PLANTA DEMO</span>
          <h2 className="m-0 font-display text-3xl font-semibold tracking-[-0.02em]">Iniciar sesión</h2>
        </div>
        {notice && (
          <span role="status" className="rounded border border-line-2 bg-bg px-3.5 py-2.5 text-[13px] text-ink-2">
            {notice === 'expired' ? 'Tu sesión expiró. Vuelve a entrar para continuar.' : 'Sesión cerrada. El último análisis queda guardado y lo verás al volver a entrar.'}
          </span>
        )}
        <label className="flex flex-col gap-1.5 font-mono text-[11px] tracking-[0.08em] text-muted">
          CORREO
          <input
            type="email"
            required
            autoComplete="username"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            className="h-[46px] rounded border border-line-2 bg-bg px-3.5 font-sans text-[15px] tracking-normal text-ink outline-none focus:border-accent"
          />
        </label>
        <label className="flex flex-col gap-1.5 font-mono text-[11px] tracking-[0.08em] text-muted">
          CONTRASEÑA
          <input
            type="password"
            required
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="••••••••"
            className="h-[46px] rounded border border-line-2 bg-bg px-3.5 font-sans text-[15px] tracking-normal text-ink outline-none focus:border-accent"
          />
        </label>
        {login.isError && (
          <span role="alert" className="text-[13px] text-real">
            Credenciales inválidas.
          </span>
        )}
        <button
          type="submit"
          disabled={login.isPending}
          className="h-[50px] rounded bg-ink font-mono text-[13px] font-semibold tracking-[0.08em] text-bg disabled:opacity-60"
        >
          {login.isPending ? 'ENTRANDO…' : 'ENTRAR'}
        </button>
        <span className="flex flex-wrap items-center justify-between gap-2 font-mono text-[11px] text-muted">
          <span>Demo · operador@vatio.demo</span>
          <Link to="/dev" className="text-accent no-underline hover:underline">
            DEV MODE · ARQUITECTURA →
          </Link>
        </span>
      </form>
    </div>
  )
}
