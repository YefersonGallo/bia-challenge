import { useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useReport } from '@/shared/api/queries'
import type { Report } from '@/shared/api/types'
import { fmtConf, fmtDateTime, fmtNum, fmtPct } from '@/shared/lib/format'
import { priorityTag, SEVERITY_LABEL, TYPE_META } from '@/shared/lib/labels'
import { Button, EmptyState, ErrorBox, Label, Loading, Segmented } from '@/shared/ui/primitives'
import { printPage } from '@/shared/lib/print'
import { useAnalysis } from '@/features/analysis/useAnalysis'
import { FindingCard } from './FindingCard'
import { classificationHeadline, consumptionHeadline, executiveSummary } from './narrative'
import { useReportReview } from './useReportReview'

type Mode = 'full' | 'exec'

const SECTIONS = [
  { id: 'resumen', n: '00', title: 'Resumen ejecutivo', exec: true },
  { id: 'que-pasa', n: '01', title: '¿Qué está pasando con los medidores?' },
  { id: 'lecturas', n: '02', title: '¿Qué lecturas se salen de lo esperado?' },
  { id: 'clasificacion', n: '03', title: '¿Es real, explicable o de calidad de datos?' },
  { id: 'prioridad', n: '04', title: '¿Cuál debería investigarse primero?' },
  { id: 'por-que', n: '05', title: '¿Por qué la IA llegó a esa conclusión?' },
  { id: 'accion', n: '06', title: '¿Qué acción recomienda?', exec: true },
  { id: 'anexos', n: 'A', title: 'Anexos' },
]

function Section({ id, n, title, headline, children }: { id: string; n: string; title: string; headline?: string; children: ReactNode }) {
  return (
    <section id={id} aria-labelledby={`${id}-h`} className="flex scroll-mt-6 flex-col gap-3 border-t border-paper-line pt-7">
      <span className="font-mono text-[11px] font-semibold tracking-[0.1em] text-report uppercase">
        {n} · {title}
      </span>
      {headline && (
        <h2 id={`${id}-h`} className="m-0 max-w-[720px] font-display text-[26px] leading-tight font-semibold tracking-[-0.02em]">
          {headline}
        </h2>
      )}
      {children}
    </section>
  )
}

const th = 'border-b border-paper-ink py-2 pr-3 text-left font-mono text-[10px] font-normal tracking-[0.08em] text-paper-muted'
const td = 'border-b border-paper-line py-2 pr-3 align-top text-[13px]'

function Contributions({ r }: { r: Report }) {
  const rows = [...r.contributions].sort((a, b) => Math.abs(b.delta_kwh) - Math.abs(a.delta_kwh)).slice(0, 6)
  const max = Math.max(...rows.map((c) => Math.abs(c.delta_kwh)), 1)
  const typeOf = new Map(r.findings.map((f) => [f.meter_id, f.type]))
  return (
    <figure className="m-0 flex flex-col gap-1.5">
      <figcaption className="font-mono text-[10px] tracking-[0.1em] text-paper-muted">CAMBIO VS BASELINE DIARIO · KWH EN LAS ÚLTIMAS 24 H</figcaption>
      {rows.map((c) => {
        const t = typeOf.get(c.meter_id)
        const color = t ? TYPE_META[t].paper : '#b8bdc7'
        const w = (Math.abs(c.delta_kwh) / max) * 50
        return (
          <div key={c.meter_id} className="grid grid-cols-[150px_minmax(0,1fr)_80px] items-center gap-3 text-[13px]">
            <span className="truncate">
              <span className="font-mono font-semibold">{c.meter_id}</span> <span className="text-paper-muted">{c.name}</span>
            </span>
            <span className="relative h-3.5">
              <span className="absolute inset-y-0 left-1/2 w-px bg-paper-muted" />
              <span className="absolute inset-y-0.5" style={{ background: color, width: `${w}%`, left: c.delta_kwh >= 0 ? '50%' : `${50 - w}%` }} />
            </span>
            <span className="text-right font-mono">
              {c.delta_kwh > 0 ? '+' : ''}
              {fmtNum(c.delta_kwh)}
            </span>
          </div>
        )
      })}
    </figure>
  )
}

function ReportBody({ r, mode, review }: { r: Report; mode: Mode; review: ReturnType<typeof useReportReview> }) {
  const exec = mode === 'exec'
  const sec = (id: string) => SECTIONS.find((s) => s.id === id)!
  const okMeters = r.summary.meters - r.rule_flags.length
  const steps = r.plan.flatMap((p) => (p.steps ?? []).map((s, i) => ({ key: `${p.meter_id}-${i}`, text: s, p })))

  return (
    <article className="mx-auto flex w-full max-w-[900px] flex-col gap-7 bg-paper px-6 py-10 text-paper-ink sm:px-14" aria-label="Reporte de análisis IA">
      <header className="flex flex-col gap-2">
        <span className="font-display text-xl font-bold">Vatio</span>
        <span className="font-mono text-[11px] tracking-[0.08em] text-paper-muted">
          REPORTE DE ANÁLISIS IA · #{r.run_id} · PLANTA DEMO · DÍAS 1–14 · GENERADO {fmtDateTime(r.generated_at).toUpperCase()}
        </span>
      </header>

      <Section {...sec('resumen')} headline={r.headline}>
        {executiveSummary(r).map((p) => (
          <p key={p} className="m-0 max-w-[720px] text-[15px] leading-relaxed text-paper-2">
            {p}
          </p>
        ))}
        <span className="pt-2 font-mono text-[10px] tracking-[0.1em] text-paper-muted">DECISIONES</span>
        <table className="w-full border-collapse">
          <thead>
            <tr>
              {['#', 'MEDIDOR', 'VEREDICTO', 'ACCIÓN', 'RESPONSABLE', 'PLAZO'].map((h) => (
                <th key={h} className={th}>
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {r.plan.map((p, i) => (
              <tr key={p.meter_id}>
                <td className={`${td} font-mono`}>{priorityTag(i + 1)}</td>
                <td className={`${td} font-mono font-semibold`}>{p.meter_id}</td>
                <td className={`${td} font-semibold`} style={{ color: TYPE_META[p.type].paper }}>
                  {TYPE_META[p.type].label}
                </td>
                <td className={td}>{p.action}</td>
                <td className={td}>{p.owner}</td>
                <td className={td}>{p.deadline}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </Section>

      {!exec && (
        <>
          <Section {...sec('que-pasa')} headline={consumptionHeadline(r)}>
            <Contributions r={r} />
          </Section>

          <Section
            {...sec('lecturas')}
            headline={`${r.rule_flags.length} medidores salen de su banda. Las reglas lo detectan, pero no saben por qué.`}
          >
            <table className="w-full border-collapse">
              <thead>
                <tr>
                  <th className={th}>MEDIDOR</th>
                  <th className={th}>REGLA QUE SE ACTIVÓ</th>
                  <th className={`${th} text-right`}>VARIACIÓN</th>
                </tr>
              </thead>
              <tbody>
                {r.rule_flags.map((m) => (
                  <tr key={m.id}>
                    <td className={`${td} font-mono font-semibold`}>
                      {m.id} <span className="font-sans font-normal text-paper-muted">{m.name}</span>
                    </td>
                    <td className={td}>{m.status_reason}</td>
                    <td className={`${td} text-right font-mono`}>{fmtPct(m.variation_pct)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            <p className="m-0 text-xs text-paper-muted">
              Banda: ±{fmtNum(r.methodology.shift_threshold_pct)}% del baseline horario (mediana de los días 1–{r.methodology.baseline_days}). Los otros {okMeters}{' '}
              medidores se mantienen dentro.
            </p>
          </Section>

          <Section {...sec('clasificacion')} headline={classificationHeadline(r)}>
            <div className="grid gap-2 sm:grid-cols-2">
              {r.findings.map((f) => (
                <div key={f.id} className="flex flex-col gap-0.5 border-l-[3px] bg-white px-3 py-2" style={{ borderColor: TYPE_META[f.type].paper }}>
                  <span className="font-mono text-[13px] font-semibold">
                    {f.meter_id} · <span style={{ color: TYPE_META[f.type].paper }}>{TYPE_META[f.type].label}</span>
                  </span>
                  <span className="text-xs text-paper-2">{f.evidence.signals?.[0]?.description}</span>
                </div>
              ))}
            </div>
            <p className="m-0 max-w-[720px] text-[13px] leading-relaxed text-paper-2">
              El motor clasifica en orden fijo: primero calidad de datos, después cruce con eventos operativos (±{fmtNum(r.methodology.event_window_hours)} h del inicio del
              cambio y coherentes con su dirección) y por último magnitud. Así un evento coherente descarta la alarma antes de asignar severidad, y un medidor con lecturas
              imposibles no se confunde con un cambio de carga.
            </p>
          </Section>

          <Section
            {...sec('prioridad')}
            headline={`${joinIds(r, 2)} primero. ${r.findings.length > 2 ? 'Los demás no compiten por atención.' : ''}`}
          >
            <table className="w-full border-collapse">
              <thead>
                <tr>
                  {['#', 'MEDIDOR', 'SEVERIDAD', 'CONFIANZA', 'PERSIST.', 'EVENTO', 'SCORE'].map((h) => (
                    <th key={h} className={th}>
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {r.findings.map((f) => (
                  <tr key={f.id}>
                    <td className={`${td} font-mono`}>{priorityTag(f.rank)}</td>
                    <td className={`${td} font-mono font-semibold`}>{f.meter_id}</td>
                    <td className={td}>{SEVERITY_LABEL[f.severity]}</td>
                    <td className={`${td} font-mono`}>{fmtConf(f.confidence)}</td>
                    <td className={`${td} font-mono`}>{f.evidence.persistent_hours} h</td>
                    <td className={td}>{f.evidence.event_explains_shift ? 'Lo explica' : 'No'}</td>
                    <td className={`${td} font-mono font-semibold`}>{fmtNum(f.priority_score, 3)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            <p className="m-0 font-mono text-[11px] text-paper-muted">
              prioridad = severidad × magnitud × persistencia × (1 − 0,8 si un evento lo explica) · la calcula el motor, no el LLM
            </p>
          </Section>

          <Section {...sec('por-que')} headline="Fichas de evidencia por hallazgo.">
            <p className="m-0 max-w-[720px] text-[13px] text-paper-2">
              Cada ficha muestra la evidencia que calculó el motor y la explicación redactada a partir de ella. Las cifras citadas se validan contra la evidencia antes de
              publicarse; si no coinciden, se usa la plantilla del motor.
            </p>
            {r.findings.map((f) => (
              <FindingCard key={f.id} f={f} />
            ))}
          </Section>
        </>
      )}

      <Section {...sec('accion')} headline={`Plan de acción: ${review.done.size} de ${steps.length} pasos completados.`}>
        {r.plan.map((p) => (
          <div key={p.meter_id} className="flex flex-col gap-1.5">
            <span className="text-sm">
              <span className="font-mono font-semibold">{p.meter_id}</span> ·{' '}
              <span className="font-semibold" style={{ color: TYPE_META[p.type].paper }}>
                {p.action}
              </span>{' '}
              <span className="text-paper-muted">
                · {p.owner} · {p.deadline}
              </span>
            </span>
            {(p.steps ?? []).map((s, i) => {
              const key = `${p.meter_id}-${i}`
              return (
                <label key={key} className="flex cursor-pointer items-start gap-2.5 pl-1 text-[13px] text-paper-2">
                  <input type="checkbox" checked={review.done.has(key)} onChange={() => review.toggle(key)} className="mt-0.5 accent-[var(--color-report)]" />
                  <span className={review.done.has(key) ? 'line-through opacity-60' : undefined}>{s}</span>
                </label>
              )
            })}
          </div>
        ))}
        <Link to="/" className="font-mono text-xs text-report print:hidden">
          Seguir las alarmas en Operación →
        </Link>
      </Section>

      {!exec && (
        <Section {...sec('anexos')}>
          <div className="grid gap-6 text-[13px] text-paper-2 sm:grid-cols-2">
            <div className="flex flex-col gap-1">
              <h3 className="m-0 text-sm font-semibold text-paper-ink">Metodología</h3>
              <span>Baseline: mediana por hora del día de los días 1–{r.methodology.baseline_days} (robusta a picos).</span>
              <span>
                Cambio sostenido: al menos {r.methodology.min_episode_hours} h fuera de ±{fmtNum(r.methodology.shift_threshold_pct)}% del perfil horario, a partir del día{' '}
                {r.methodology.baseline_days + 1}.
              </span>
              <span>Picos: z-score robusto (MAD) &gt; {fmtNum(r.methodology.z_threshold, 1)}.</span>
              <span>Crítico: variación &gt; {fmtNum(r.methodology.critical_pct)}%; severidad alta &gt; {fmtNum(r.methodology.high_pct)}%.</span>
            </div>
            <div className="flex flex-col gap-1">
              <h3 className="m-0 text-sm font-semibold text-paper-ink">Calidad de datos</h3>
              <span>
                {fmtNum(r.summary.readings)} lecturas, {fmtNum(r.summary.invalid_readings)} inválidas (PF fuera de [0,1] o 0 V con consumo).
              </span>
              <span>
                Lectura inconsistente: voltaje fuera de {fmtNum(r.methodology.voltage_min)}–{fmtNum(r.methodology.voltage_max)} V o con saltos de más de{' '}
                {fmtNum(r.methodology.voltage_jump)} V entre horas, salto de factor de potencia mayor a {fmtNum(r.methodology.pf_jump, 2)} frente a sus vecinas, o relación
                kWh / V·I·PF que se aparta ±{fmtNum(r.methodology.coherence_tol_pct)}% de la propia y de la local. Severidad alta si más del{' '}
                {fmtNum(r.methodology.dq_high_share_pct)}% de las lecturas de 24 h quedan marcadas. Un cambio de régimen sostenido no cuenta como error de datos.
              </span>
            </div>
            <div className="flex flex-col gap-1">
              <h3 className="m-0 text-sm font-semibold text-paper-ink">Fuentes y trazabilidad</h3>
              {r.sources.map((s) => (
                <span key={s} className="font-mono text-xs">
                  {s}
                </span>
              ))}
              <span>Evidencia de cada ficha guardada con el análisis #{r.run_id}.</span>
            </div>
            <div className="flex flex-col gap-1">
              <h3 className="m-0 text-sm font-semibold text-paper-ink">Limitaciones</h3>
              <span>{r.summary.period_days} días de datos: el baseline usa {r.methodology.baseline_days} días y no captura estacionalidad semanal.</span>
              <span>La topología eléctrica no viene en el dataset.</span>
              <span>Claude redacta la explicación, pero no cambia tipo, severidad ni cifras.</span>
            </div>
          </div>
        </Section>
      )}

      <footer className="border-t border-paper-line pt-4 font-mono text-[10px] tracking-[0.08em] text-paper-muted">
        GENERADO POR VATIO · MOTOR DE ANÁLISIS + CLAUDE (REDACCIÓN) · #{r.run_id}
      </footer>
    </article>
  )
}

function joinIds(r: Report, n: number) {
  const ids = r.findings.slice(0, n).map((f) => f.meter_id)
  return ids.length === 2 ? `${ids[0]}, luego ${ids[1]}` : (ids[0] ?? '—')
}

/** AI report: executive summary, the six questions of the plant, evidence sheets, plan and annexes. */
export function ReportPage() {
  const { data: r, isLoading, error } = useReport()
  const [mode, setMode] = useState<Mode>('full')
  const review = useReportReview(r?.run_id)
  const analysis = useAnalysis()

  if (isLoading) return <Loading what="el reporte" />
  if (error) return <ErrorBox error={error} />
  if (!r) {
    return (
      <div className="flex flex-1 flex-col gap-4 px-6 py-5">
        <h1 className="m-0 font-display text-2xl font-semibold">Reporte IA</h1>
        <EmptyState
          title="El reporte se genera con el análisis."
          action={
            <Button variant="primary" onClick={analysis.start} disabled={analysis.running}>
              {analysis.running ? 'ANALIZANDO…' : 'RUN AI ANALYSIS'}
            </Button>
          }
        >
          Incluye resumen ejecutivo, las 6 preguntas de la planta, fichas de evidencia, plan de acción y anexos de metodología.
        </EmptyState>
      </div>
    )
  }

  const toc = SECTIONS.filter((s) => mode === 'full' || s.exec)
  return (
    <div className="grid min-h-0 flex-1 lg:grid-cols-[260px_minmax(0,1fr)] print:block">
      <aside className="flex flex-col gap-5 border-r border-line bg-panel-2 px-5 py-5 print:hidden">
        <div className="flex flex-col gap-1">
          <Label>ANÁLISIS #{r.run_id}</Label>
          <span className="text-[13px] text-ink-2">{fmtDateTime(r.generated_at)}</span>
        </div>
        <div className="flex flex-col gap-2">
          <Label>VISTA</Label>
          <Segmented
            label="Vista del reporte"
            value={mode}
            onChange={setMode}
            options={[
              { value: 'full', label: 'COMPLETO' },
              { value: 'exec', label: 'EJECUTIVO' },
            ]}
          />
        </div>
        <nav aria-label="Contenido del reporte" className="flex flex-col gap-1">
          <Label>CONTENIDO</Label>
          {toc.map((s) => (
            <a key={s.id} href={`#${s.id}`} className="flex gap-2 py-0.5 text-[13px] text-soft no-underline hover:text-ink">
              <span className="w-5 font-mono text-[11px] text-dim">{s.n}</span>
              {s.title}
            </a>
          ))}
        </nav>
        <div className="mt-auto flex flex-col gap-2">
          <Label>ESTADO DEL REPORTE</Label>
          <label className="flex items-center gap-2 text-[13px]">
            <input type="checkbox" checked={review.reviewed} onChange={(e) => review.setReviewed(e.target.checked)} />
            Marcar como revisado
          </label>
          <Button onClick={printPage}>EXPORTAR PDF</Button>
        </div>
      </aside>
      <div className="min-h-0 overflow-y-auto bg-[#e9eaee] py-6 print:overflow-visible print:bg-white print:py-0">
        <ReportBody r={r} mode={mode} review={review} />
      </div>
    </div>
  )
}
