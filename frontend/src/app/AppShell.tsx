import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { useAnomalies, useSummary, useUpdateAnomaly } from '@/shared/api/queries'
import { fmtConf, fmtNum, fmtPct, fmtTime } from '@/shared/lib/format'
import { cx } from '@/shared/lib/cx'
import { AnalysisStrip } from '@/features/analysis/AnalysisStrip'
import { useAnalysisUi } from '@/features/analysis/analysisStore'
import { useAnalysis } from '@/features/analysis/useAnalysis'
import { UserMenu } from './UserMenu'

function HeaderKpis() {
  const { data: s } = useSummary()
  if (!s) return null
  const items = [
    { l: 'MEDIDORES', v: String(s.meters) },
    { l: `KWH ${s.period_days || 14} D`, v: fmtNum(s.period_kwh), wide: true },
    { l: '24 H VS BASELINE', v: fmtPct(s.variation_pct), c: Math.abs(s.variation_pct) >= 10 ? 'var(--color-expl)' : undefined },
    { l: 'ANOMALÍAS', v: s.anomalies == null ? '—' : String(s.anomalies) },
    { l: 'ALTA PRIOR.', v: s.high_priority == null ? '—' : String(s.high_priority), c: s.high_priority ? 'var(--color-real)' : undefined },
    { l: 'CONF.', v: s.avg_confidence == null ? '—' : fmtConf(s.avg_confidence), wide: true },
  ]
  return (
    <div className="hidden gap-px overflow-hidden rounded border border-line bg-line xl:flex">
      {items.map((k) => (
        <div key={k.l} className={cx('flex-col bg-panel-2 px-2.5 py-[3px] whitespace-nowrap', 'wide' in k && k.wide ? 'hidden min-[1440px]:flex' : 'flex')}>
          <span className="font-mono text-[9px] tracking-[0.1em] text-muted">{k.l}</span>
          <span className="font-mono text-sm font-semibold" style={{ color: k.c }}>
            {k.v}
          </span>
        </div>
      ))}
    </div>
  )
}

/** P1 banner: the top-priority real anomaly while it is still new. */
function P1Banner() {
  const navigate = useNavigate()
  const { data = [] } = useAnomalies()
  const update = useUpdateAnomaly()
  const p1 = data.find((a) => a.rank === 1 && a.type === 'REAL_ANOMALY' && a.status === 'OPEN')
  if (!p1) return null
  return (
    <div role="alert" className="flex h-11 shrink-0 items-center gap-4 border-b border-[#5c1a12] bg-[#2a0d09] pr-6 print:hidden">
      <span className="flex h-11 w-[60px] items-center justify-center bg-real font-mono text-[15px] font-semibold text-bg">P1</span>
      <span className="font-mono text-sm font-semibold whitespace-nowrap">{p1.meter_id}</span>
      <span className="truncate text-sm text-[#ffd2cc]">
        Anomalía real · {p1.reason} · conf {fmtConf(p1.confidence)}
      </span>
      <span className="ml-auto flex shrink-0 gap-2">
        <button type="button" onClick={() => navigate(`/anomalies/${p1.id}`)} className="h-8 rounded border border-real px-3.5 text-[13px] font-semibold text-white">
          Ver por qué
        </button>
        <button
          type="button"
          disabled={update.isPending}
          onClick={() => update.mutate({ id: p1.id, status: 'ACKNOWLEDGED' })}
          className="h-8 rounded bg-real px-3.5 text-[13px] font-semibold text-bg"
        >
          Reconocer
        </button>
      </span>
    </div>
  )
}

/** Header with sections, KPIs and the Run AI Analysis control; the strip and P1 banner below it. */
export function AppShell() {
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const { data: summary } = useSummary()
  const { stripOpen, close } = useAnalysisUi()
  const { run, running, start, startError } = useAnalysis()

  const nav = [
    { to: '/', label: 'OPERACIÓN', end: true },
    { to: '/meters', label: 'MEDIDORES' },
    { to: '/anomalies', label: 'ANOMALÍAS IA', badge: summary?.open_anomalies || undefined, badgeColor: 'var(--color-real)' },
    { to: '/report', label: 'REPORTE IA', badge: summary?.last_analysis?.status === 'COMPLETED' ? 'NUEVO' : undefined, badgeColor: 'var(--color-accent)' },
  ]
  const last = summary?.last_analysis
  const aiText = running ? 'IA · analizando…' : last ? `IA · ${fmtTime(last.finished_at ?? last.started_at)}` : 'IA · sin análisis'

  return (
    <div className="flex h-full flex-col overflow-hidden print:block print:h-auto print:overflow-visible">
      <header className="flex h-14 shrink-0 items-center gap-3 border-b border-line px-6 whitespace-nowrap print:hidden">
        <span className="font-display text-[17px] font-bold">VATIO</span>
        <nav aria-label="Secciones" className="ml-2 flex self-stretch">
          {nav.map((n) => (
            <NavLink
              key={n.to}
              to={n.to}
              end={n.end}
              className={({ isActive }) =>
                cx(
                  'flex items-center gap-2 border-b-2 px-3 font-mono whitespace-nowrap text-[11px] font-semibold tracking-[0.08em] no-underline',
                  isActive || (n.to !== '/' && pathname.startsWith(n.to)) ? 'border-accent text-ink' : 'border-transparent text-muted hover:text-ink-2',
                )
              }
            >
              {n.label}
              {n.badge != null && (
                <span className="rounded-[3px] px-1.5 py-px text-[9px] text-bg" style={{ background: n.badgeColor }}>
                  {n.badge}
                </span>
              )}
            </NavLink>
          ))}
        </nav>
        {pathname === '/' && <HeaderKpis />}
        <div className="ml-auto flex items-center gap-3 font-mono text-[11px]">
          <span className={running ? 'text-accent' : last ? 'text-ok' : 'text-muted'} title={last ? `Último análisis ${last.id}` : undefined}>
            {aiText}
          </span>
          {startError && <span className="text-real">No se pudo iniciar</span>}
          <button type="button" onClick={() => start()} disabled={running} className="h-[34px] rounded bg-ink px-3.5 text-[11px] font-semibold text-bg disabled:opacity-60">
            {running ? 'ANALIZANDO…' : 'RUN AI ANALYSIS'}
          </button>
          <UserMenu />
        </div>
      </header>
      {stripOpen && run && (
        <AnalysisStrip
          run={run}
          onClose={close}
          onGoAnomalies={() => {
            close()
            navigate('/anomalies')
          }}
          onGoReport={() => {
            close()
            navigate('/report')
          }}
        />
      )}
      <P1Banner />
      <main className="flex min-h-0 flex-1 flex-col print:block">
        <Outlet />
      </main>
    </div>
  )
}
