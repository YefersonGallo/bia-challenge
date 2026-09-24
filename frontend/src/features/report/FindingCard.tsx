import type { AnomalyDetail } from '@/shared/api/types'
import { DailyChart } from '@/shared/charts/DailyChart'
import { HourlyChart } from '@/shared/charts/small'
import { fmtConf, fmtNum, fmtPct } from '@/shared/lib/format'
import { priorityTag, SEVERITY_LABEL, TYPE_META } from '@/shared/lib/labels'
import { eventMarkers } from '@/shared/lib/meterMath'

/** Evidence sheet of one finding on the report paper. */
export function FindingCard({ f }: { f: AnomalyDetail }) {
  const meta = TYPE_META[f.type]
  const ev = f.evidence
  const showHourly = ev.night_ratio >= 1.5
  return (
    <article id={`ficha-${f.meter_id}`} className="flex scroll-mt-6 flex-col gap-3 rounded-md border border-paper-line bg-white p-5 break-inside-avoid">
      <header className="flex flex-wrap items-center gap-3">
        <span className="rounded-[3px] px-2 py-0.5 font-mono text-xs font-semibold text-white" style={{ background: meta.paper }}>
          {priorityTag(f.rank)}
        </span>
        <span className="font-mono text-lg font-semibold">{f.meter_id}</span>
        <span className="text-sm text-paper-2">
          {f.meter.name} · {f.meter.location}
        </span>
        <span className="ml-auto text-[13px] font-semibold" style={{ color: meta.paper }}>
          {meta.label} · severidad {SEVERITY_LABEL[f.severity].toLowerCase()} · conf {fmtConf(f.confidence)}
        </span>
      </header>
      <p className="m-0 border-l-2 pl-3 text-[15px] leading-relaxed" style={{ borderColor: meta.paper }}>
        <span className="mr-1.5 font-mono text-[10px] tracking-[0.1em] text-paper-muted">{f.explained_by === 'claude' ? 'CLAUDE' : 'MOTOR'} ·</span>
        {f.reason}
      </p>
      <div className={showHourly ? 'grid gap-4 md:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]' : ''}>
        <div className="flex flex-col gap-1">
          <span className="font-mono text-[10px] tracking-[0.1em] text-paper-muted">KWH/DÍA · 14 DÍAS · BANDA = BASELINE ±10%</span>
          <DailyChart
            paper
            days={f.days}
            color={meta.paper}
            window={ev.onset_day ? [ev.onset_day, ev.end_day ?? f.days.length] : null}
            markers={eventMarkers(ev.related_events, f.days).map((m) => ({ ...m, color: 'var(--color-expl-l)' }))}
            width={640}
            height={190}
          />
        </div>
        {showHourly && (
          <div className="flex flex-col gap-1">
            <span className="font-mono text-[10px] tracking-[0.1em] text-paper-muted">PERFIL HORARIO · BASELINE VS ACTUAL</span>
            <HourlyChart paper baseline={f.hourly_baseline} current={f.hourly_current} color={meta.paper} width={420} height={190} />
            <span className="text-xs text-paper-2">De noche consume {fmtNum(ev.night_ratio, 1)}× el baseline: la carga no se apaga fuera de turno.</span>
          </div>
        )}
      </div>
      <div className="grid gap-4 md:grid-cols-2">
        <div className="flex flex-col gap-1">
          <span className="font-mono text-[10px] tracking-[0.1em] text-paper-muted">EVIDENCIA</span>
          <ul className="m-0 flex list-none flex-col gap-1 p-0 text-[13px] text-paper-2">
            {(ev.signals ?? []).map((s) => (
              <li key={s.code} className="flex gap-2">
                <span style={{ color: meta.paper }}>▸</span>
                {s.description}
              </li>
            ))}
          </ul>
        </div>
        <table className="w-full border-collapse self-start text-[13px]">
          <tbody>
            {(ev.variables ?? []).map((v) => (
              <tr key={v.name} className="border-b border-paper-line last:border-0">
                <td className="py-1 text-paper-2">{v.name}</td>
                <td className="py-1 text-right font-mono">
                  {fmtNum(v.baseline, v.baseline < 10 ? 2 : 0)} → {fmtNum(v.current, v.current < 10 ? 2 : 0)} {v.unit}
                </td>
                <td className="py-1 pl-3 text-right font-mono font-semibold" style={{ color: Math.abs(v.delta_pct) >= 5 ? meta.paper : undefined }}>
                  {fmtPct(v.delta_pct)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="m-0 text-[13px]">
        <span className="font-semibold">Acción: </span>
        {f.recommended_action}
      </p>
    </article>
  )
}
