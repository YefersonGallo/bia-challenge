import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/shared/api/client'
import { useAuthStore } from '@/features/auth/authStore'
import { Label, Lamp, Panel } from '@/shared/ui/primitives'
import { cx } from '@/shared/lib/cx'

interface Health {
  status: string
  ai: string
  store?: string
}

const ENGINE = 'var(--color-accent)'
const LLM = 'var(--color-data)'

const PIPELINE: { key: string; label: string; who: 'Motor' | 'Claude'; text: string }[] = [
  { key: '1', label: 'Lecturas', who: 'Motor', text: 'Carga las lecturas horarias y marca las físicamente inconsistentes (kWh frente a V·I·FP). La columna status del CSV se ignora.' },
  { key: '2', label: 'Baseline', who: 'Motor', text: 'Perfil por hora del día de cada medidor con los días 1–7, con banda p10–p90.' },
  { key: '3', label: 'Detección', who: 'Motor', text: 'Busca desviaciones sostenidas frente al baseline: un episodio necesita persistir, no basta una lectura suelta.' },
  { key: '4', label: 'Correlación', who: 'Motor', text: 'Cruza consumo con corriente, voltaje y factor de potencia para saber si el cambio es eléctricamente coherente o es el medidor.' },
  { key: '5', label: 'Eventos', who: 'Motor', text: 'Cruza los eventos cercanos y en la misma dirección; clasifica tipo, severidad y confianza, y ordena por prioridad.' },
  { key: '6', label: 'Explicación', who: 'Claude', text: 'Claude redacta la razón y los próximos pasos sobre la evidencia. Un validador rechaza cualquier cifra que no esté en ella.' },
  { key: '7', label: 'Recomendación', who: 'Motor', text: 'Acción canónica por tipo de anomalía y resumen de la corrida: cuántas son prioritarias y cuál va primero.' },
]

const PACKAGES: [string, string][] = [
  ['internal/domain', 'Entidades (medidor, lectura, evento, anomalía, corrida) y reglas puras. Sin dependencias ni I/O.'],
  ['internal/analysis', 'Motor determinístico: estadísticas, baseline, episodios, clasificación, confianza, impacto y prioridad.'],
  ['internal/explain', 'Puerto Explainer: Claude con tool_use forzado, plantilla, validador de cifras, caché y fallback con timeout.'],
  ['internal/app', 'Casos de uso: corre el análisis en segundo plano paso a paso, acciones del operador y reporte. Define los puertos.'],
  ['internal/httpapi', 'Adaptador REST (net/http): auth, middlewares, errores JSON y la SPA en el despliegue de una sola imagen.'],
  ['internal/store/postgres · memory', 'Adaptadores del puerto Store: PostgreSQL con JSONB en despliegue, memoria en desarrollo y tests.'],
  ['internal/ingest', 'Lee y valida los CSV (separador , o ;), con errores por línea y columna.'],
  ['cmd/api · cmd/analyze', 'Binarios: el servidor y el análisis por línea de comandos en el formato de salida de la prueba.'],
]

const ENDPOINTS: [string, string][] = [
  ['POST', '/api/auth/login'],
  ['GET', '/api/health'],
  ['GET', '/api/dashboard/summary · /heatmap'],
  ['GET', '/api/meters · /{id} · /readings · /events · /baseline · /forecast'],
  ['GET', '/api/events'],
  ['GET', '/api/anomalies · /{id}'],
  ['PATCH', '/api/anomalies/{id}'],
  ['POST', '/api/anomalies/{id}/actions'],
  ['POST', '/api/ai/analyze'],
  ['GET', '/api/ai/analysis/{id|latest}'],
  ['GET', '/api/reports/latest'],
]

function Box({ title, sub, color, active, children }: { title: string; sub?: string; color?: string; active?: boolean; children?: ReactNode }) {
  return (
    <div
      className={cx('flex flex-col gap-1 rounded-md border bg-panel-2 px-3 py-2.5', active === false && 'opacity-45')}
      style={{ borderColor: color ?? 'var(--color-line-2)' }}
    >
      <span className="flex items-center justify-between gap-2 font-mono text-[12px] font-semibold text-ink">
        {title}
        {active && <Lamp color={color ?? 'var(--color-ok)'} size={7} />}
      </span>
      {sub && <span className="text-[12px] leading-snug text-muted">{sub}</span>}
      {children}
    </div>
  )
}

function Arrow() {
  return (
    <span aria-hidden className="flex items-center justify-center font-mono text-lg text-line-2 max-lg:rotate-90">
      →
    </span>
  )
}

function Section({ label, title, children }: { label: string; title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-3">
      <div className="flex flex-col gap-1">
        <Label>{label}</Label>
        <h2 className="m-0 font-display text-xl font-semibold">{title}</h2>
      </div>
      {children}
    </section>
  )
}

/** Dev mode: how Vatio is built, with the adapters this deploy is running right now. */
export function DevPage() {
  const token = useAuthStore((s) => s.token)
  const health = useQuery({ queryKey: ['health'], queryFn: () => api<Health>('/health'), staleTime: 60_000 })
  const h = health.data
  const claude = h?.ai?.startsWith('claude:')
  const model = claude ? h!.ai.slice('claude:'.length) : null
  const pg = h?.store === 'postgres'
  const mem = h?.store === 'memory'

  const runtime = [
    { l: 'API', v: health.isError ? 'sin respuesta' : h ? 'en línea' : '…', c: health.isError ? 'var(--color-real)' : h ? 'var(--color-ok)' : undefined },
    { l: 'EXPLICACIONES', v: h ? (claude ? `Claude · ${model}` : 'Plantilla (sin API key)') : '…', c: h ? (claude ? LLM : 'var(--color-expl)') : undefined },
    { l: 'ALMACENAMIENTO', v: h ? (pg ? 'PostgreSQL' : mem ? 'Memoria' : 'no informado') : '…' },
    { l: 'FRONTEND', v: import.meta.env.MODE === 'production' ? 'build de producción' : `vite · ${import.meta.env.MODE}` },
  ]

  return (
    <div className="h-full overflow-y-auto">
      <header className="sticky top-0 z-10 flex h-14 items-center gap-3 border-b border-line bg-bg/95 px-6 backdrop-blur">
        <span className="font-display text-[17px] font-bold">VATIO</span>
        <span className="rounded-[3px] border border-accent px-1.5 py-px font-mono text-[10px] font-semibold tracking-[0.1em] text-accent">DEV MODE</span>
        <Link to={token ? '/' : '/login'} className="ml-auto font-mono text-[11px] font-semibold tracking-[0.06em] text-muted no-underline hover:text-ink">
          ← {token ? 'VOLVER A LA APP' : 'VOLVER AL LOGIN'}
        </Link>
      </header>

      <div className="mx-auto flex max-w-[1180px] flex-col gap-10 px-6 pt-8 pb-16">
        <div className="flex flex-col gap-3">
          <h1 className="m-0 font-display text-[40px] leading-tight font-semibold tracking-[-0.02em]">
            Arquitectura: <span className="text-accent">decide el motor, redacta Claude.</span>
          </h1>
          <p className="m-0 max-w-[820px] text-[15px] leading-relaxed text-soft">
            Backend en Go con arquitectura hexagonal y un motor determinístico que clasifica, puntúa y prioriza cada anomalía. Claude
            solo escribe la explicación, y cada número que escribe se valida contra la evidencia. El frontend es una SPA en React que
            consume la API REST.
          </p>
        </div>

        <div className="grid grid-cols-2 gap-px overflow-hidden rounded-md border border-line bg-line md:grid-cols-4" aria-label="Estado de este despliegue">
          {runtime.map((r) => (
            <div key={r.l} className="flex flex-col gap-0.5 bg-panel-2 px-4 py-3">
              <Label>{r.l}</Label>
              <span className="font-mono text-[14px] font-semibold" style={{ color: r.c }}>
                {r.v}
              </span>
            </div>
          ))}
        </div>

        <Section label="HEXÁGONO" title="Puertos y adaptadores">
          <div className="grid items-stretch gap-3 lg:grid-cols-[minmax(0,1fr)_28px_minmax(0,1.3fr)_28px_minmax(0,1fr)]">
            <div className="flex flex-col gap-2">
              <Label>ENTRADAS</Label>
              <Box title="Navegador · React SPA" sub="REST sobre /api con token Bearer." />
              <Box title="cmd/analyze" sub="Mismo motor desde la terminal, sin servidor." />
              <Box title="ingest · CSV" sub="Al arrancar, si el almacenamiento está vacío: medidores, lecturas y eventos." />
            </div>
            <Arrow />
            <Panel className="flex flex-col gap-2 p-3" accent={ENGINE}>
              <Label>NÚCLEO</Label>
              <Box title="app · casos de uso" sub="Corrida en segundo plano con 7 pasos, acciones del operador, reporte." />
              <div className="grid grid-cols-2 gap-2">
                <Box title="analysis" sub="Motor determinístico" color={ENGINE} />
                <Box title="domain" sub="Entidades y reglas puras" />
              </div>
              <div className="flex flex-wrap gap-2 pt-1 font-mono text-[10px] text-muted">
                <span className="rounded border border-line-2 px-1.5 py-0.5">puerto Store</span>
                <span className="rounded border border-line-2 px-1.5 py-0.5">puerto Explainer</span>
              </div>
            </Panel>
            <Arrow />
            <div className="flex flex-col gap-2">
              <Label>SALIDAS</Label>
              <div className="grid grid-cols-2 gap-2">
                <Box title="PostgreSQL" sub="JSONB" color={pg ? 'var(--color-ok)' : undefined} active={h ? pg : undefined} />
                <Box title="Memoria" sub="dev y tests" color={mem ? 'var(--color-ok)' : undefined} active={h ? mem : undefined} />
              </div>
              <div className="grid grid-cols-2 gap-2">
                <Box title="Claude" sub="tool_use + validador" color={claude ? LLM : undefined} active={h ? !!claude : undefined} />
                <Box title="Plantilla" sub="fallback" color={h && !claude ? 'var(--color-expl)' : undefined} active={h ? true : undefined} />
              </div>
              <span className="text-[12px] text-muted">Con luz: lo que este despliegue está usando ahora. La plantilla siempre está lista como respaldo.</span>
            </div>
          </div>
        </Section>

        <Section label="RUN AI ANALYSIS" title="Los 7 pasos de una corrida">
          <ol className="m-0 grid list-none gap-2 p-0 md:grid-cols-2 xl:grid-cols-4">
            {PIPELINE.map((p) => {
              const color = p.who === 'Claude' ? LLM : ENGINE
              return (
                <li key={p.key} className="flex flex-col gap-1.5 rounded-md border border-line bg-panel px-3.5 py-3">
                  <span className="flex items-center justify-between font-mono text-[11px]">
                    <span className="font-semibold text-ink">
                      {p.key} · {p.label.toUpperCase()}
                    </span>
                    <span className="rounded-[3px] px-1.5 py-px text-[9px] font-semibold text-bg" style={{ background: color }}>
                      {p.who.toUpperCase()}
                    </span>
                  </span>
                  <span className="text-[13px] leading-snug text-ink-2">{p.text}</span>
                </li>
              )
            })}
          </ol>
        </Section>

        <Section label="RESPONSABILIDADES" title="Quién decide qué">
          <div className="grid gap-3 md:grid-cols-2">
            <Panel className="flex flex-col gap-2 p-4" accent={ENGINE}>
              <span className="font-mono text-[12px] font-semibold" style={{ color: ENGINE }}>
                MOTOR GO · DECIDE
              </span>
              <ul className="m-0 flex list-disc flex-col gap-1.5 pl-4 text-[13px] text-ink-2 marker:text-line-2">
                <li>Tipo: anomalía real, calidad de datos, explicable o falso positivo.</li>
                <li>Severidad e impacto proyectado (kWh y COP por mes).</li>
                <li>Confianza: magnitud 35 %, persistencia 25 %, variables que coinciden 25 %, coherencia con eventos 15 %.</li>
                <li>Prioridad con desempate explícito: puntaje, tipo, kWh extra por día, medidor. Nunca depende del orden del CSV.</li>
                <li>Acción recomendada: una acción canónica por tipo.</li>
              </ul>
            </Panel>
            <Panel className="flex flex-col gap-2 p-4" accent={LLM}>
              <span className="font-mono text-[12px] font-semibold" style={{ color: LLM }}>
                CLAUDE · REDACTA
              </span>
              <ul className="m-0 flex list-disc flex-col gap-1.5 pl-4 text-[13px] text-ink-2 marker:text-line-2">
                <li>Razón, resumen de la evidencia y próximos pasos, con tool_use forzado (salida estructurada).</li>
                <li>Cada cifra se valida contra la evidencia; si inventa una, se le pide reescribir una vez.</li>
                <li>Si vuelve a fallar, tarda más del límite o no hay API key, la plantilla redacta lo mismo.</li>
                <li>Solo se guardan en caché las respuestas que pasaron la validación.</li>
                <li>La API key vive solo en el backend; expected_results.csv no se usa en ningún lado.</li>
              </ul>
            </Panel>
          </div>
        </Section>

        <Section label="BACKEND" title="Paquetes de Go">
          <div className="overflow-hidden rounded-md border border-line">
            <table className="w-full border-collapse text-left text-[13px]">
              <tbody>
                {PACKAGES.map(([pkg, role]) => (
                  <tr key={pkg} className="border-b border-line last:border-b-0">
                    <th scope="row" className="w-[34%] bg-panel-2 px-3.5 py-2.5 align-top font-mono text-[12px] font-semibold text-ink">
                      {pkg}
                    </th>
                    <td className="bg-panel px-3.5 py-2.5 text-ink-2">{role}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Section>

        <div className="grid gap-10 lg:grid-cols-2">
          <Section label="FRONTEND" title="SPA en React">
            <ul className="m-0 flex list-disc flex-col gap-2 pl-4 text-[13px] text-ink-2 marker:text-line-2">
              <li>React 19, TypeScript, Vite y React Router.</li>
              <li>
                <b className="text-ink">TanStack Query</b> para el estado del servidor: caché, invalidación al terminar un análisis y
                polling mientras corre.
              </li>
              <li>
                <b className="text-ink">Zustand</b> para el estado del cliente: sesión (persistida), filtros y la tira del análisis.
              </li>
              <li>Tailwind CSS 4 y gráficos en SVG propio, sin librería de charts.</li>
              <li>Carpetas por feature (operación, medidores, anomalías, reporte) y un shared con API, UI y gráficos.</li>
              <li>Vitest y Testing Library.</li>
            </ul>
          </Section>
          <Section label="API" title="Endpoints REST">
            <ul className="m-0 flex list-none flex-col gap-1 p-0 font-mono text-[12px]">
              {ENDPOINTS.map(([m, p]) => (
                <li key={m + p} className="flex gap-2">
                  <span className="w-12 shrink-0 font-semibold" style={{ color: m === 'GET' ? 'var(--color-ok)' : m === 'POST' ? ENGINE : 'var(--color-expl)' }}>
                    {m}
                  </span>
                  <span className="text-ink-2">{p}</span>
                </li>
              ))}
            </ul>
            <span className="text-[12px] text-muted">Todo requiere token salvo login y health.</span>
          </Section>
        </div>

        <div className="grid gap-10 lg:grid-cols-2">
          <Section label="SEGURIDAD" title="Lo básico, bien hecho">
            <ul className="m-0 flex list-disc flex-col gap-2 pl-4 text-[13px] text-ink-2 marker:text-line-2">
              <li>Token firmado con HMAC-SHA256 y vencimiento de 12 h; al cerrar sesión se limpia la caché del cliente.</li>
              <li>Login con límite de intentos (429), cuerpos de hasta 1 MB y cabeceras de seguridad.</li>
              <li>Notas del operador validadas: UTF-8, sin caracteres de control, máximo 2000.</li>
              <li>CORS apagado por defecto; la SPA y la API comparten origen.</li>
            </ul>
          </Section>
          <Section label="CALIDAD Y DESPLIEGUE" title="Cómo se prueba y se publica">
            <ul className="m-0 flex list-disc flex-col gap-2 pl-4 text-[13px] text-ink-2 marker:text-line-2">
              <li>go test -race en todo el backend; Vitest y oxlint en el frontend.</li>
              <li>CI en GitHub Actions para las ramas main y live, incluida la imagen Docker.</li>
              <li>Docker Compose con PostgreSQL, API y nginx; o una sola imagen donde la API sirve la SPA.</li>
              <li>Publicado en Render; con DB_FALLBACK_MEMORY la app sigue en memoria si la base no responde.</li>
            </ul>
          </Section>
        </div>
      </div>
    </div>
  )
}
