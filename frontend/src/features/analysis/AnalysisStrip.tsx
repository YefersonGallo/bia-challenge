import type { AnalysisRun, StepState } from '@/shared/api/types'
import { fmtConf } from '@/shared/lib/format'
import { cx } from '@/shared/lib/cx'
import { Button } from '@/shared/ui/primitives'
import { runProgress } from './useAnalysis'

function stepTone(s: StepState) {
  switch (s.status) {
    case 'COMPLETED':
      return { num: 'text-ok', title: 'text-ink', text: 'text-soft', bg: '' }
    case 'RUNNING':
      return { num: 'text-accent', title: 'text-ink', text: 'text-accent', bg: 'bg-[#0f1a26]' }
    case 'FAILED':
      return { num: 'text-real', title: 'text-ink', text: 'text-real', bg: '' }
    default:
      return { num: 'text-dim', title: 'text-dim', text: 'text-dim', bg: '' }
  }
}

function stepText(s: StepState) {
  if (s.result) return s.result
  if (s.status === 'RUNNING') return 'En curso…'
  if (s.status === 'FAILED') return 'Falló'
  return 'En espera'
}

interface Props {
  run: AnalysisRun
  onClose: () => void
  onGoAnomalies: () => void
  onGoReport: () => void
}

/** "Run AI Analysis" status strip: one column per step with its live status and result. */
export function AnalysisStrip({ run, onClose, onGoAnomalies, onGoReport }: Props) {
  const progress = runProgress(run)
  const done = run.status === 'COMPLETED'
  const title =
    run.status === 'FAILED'
      ? 'El análisis falló'
      : done
        ? (run.summary?.headline ?? 'Análisis completado')
        : `Analizando · paso ${Math.min(run.current_step + 1, run.steps.length)} de ${run.steps.length}`

  return (
    <section
      aria-label="Run AI Analysis"
      className="relative grid shrink-0 border-b border-line bg-panel-2 print:hidden"
      style={{ gridTemplateColumns: `270px repeat(${run.steps.length}, minmax(0, 1fr)) 48px` }}
    >
      <span className="absolute top-0 left-0 h-0.5 bg-accent transition-[width] duration-300" style={{ width: `${progress}%` }} />
      <div className="flex flex-col gap-1 py-3.5 pr-4 pl-6">
        <span className="label">RUN AI ANALYSIS · {progress}%</span>
        <span className="font-display text-base font-semibold" aria-live="polite">
          {title}
        </span>
        {done && run.summary && <span className="font-mono text-[10px] text-muted">confianza media {fmtConf(run.summary.avg_confidence)}</span>}
        {run.error && <span className="text-[11px] text-real">{run.error}</span>}
        {done && (
          <span className="flex gap-1.5 pt-1">
            <Button size="sm" variant="primary" onClick={onGoAnomalies}>
              VER ANOMALÍAS
            </Button>
            <Button size="sm" onClick={onGoReport}>
              VER REPORTE
            </Button>
          </span>
        )}
      </div>
      <ol className="contents">
        {run.steps.map((s, i) => {
          const t = stepTone(s)
          return (
            <li key={s.key} data-status={s.status} className={cx('flex flex-col gap-[3px] border-l border-line px-3 pt-3.5 pb-3', t.bg)}>
              <span className={cx('font-mono text-[10px] font-semibold', t.num)}>
                {s.status === 'COMPLETED' ? '✓' : String(i + 1).padStart(2, '0')}
                {s.status === 'COMPLETED' && s.duration_ms != null && <span className="font-normal text-dim"> · {s.duration_ms} ms</span>}
              </span>
              <span className={cx('font-display text-sm font-semibold', t.title)}>{s.label}</span>
              <span className={cx('text-[11px] leading-snug', t.text)}>{stepText(s)}</span>
            </li>
          )
        })}
      </ol>
      <div className="flex justify-center border-l border-line pt-2">
        <button type="button" onClick={onClose} aria-label="Cerrar estado del análisis" className="h-9 w-9 rounded text-lg text-muted hover:text-ink">
          ×
        </button>
      </div>
    </section>
  )
}
