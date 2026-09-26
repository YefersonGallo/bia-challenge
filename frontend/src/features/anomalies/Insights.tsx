import { useState } from 'react'
import { useAnomalyAction } from '@/shared/api/queries'
import type { ActionKind, AnomalyDetail, EventItem } from '@/shared/api/types'
import { fmtConf, fmtDateTime, fmtDayHour, fmtNum } from '@/shared/lib/format'
import { availableActions, eventLabel, lifecycleLabels } from '@/shared/lib/labels'
import { confidenceLevel, eventVerdict } from '@/shared/lib/series'
import { Button, Label, Panel } from '@/shared/ui/primitives'

const COMPONENT_COLOR: Record<string, string> = {
  magnitude: 'var(--color-accent)',
  persistence: 'var(--color-expl)',
  variables: 'var(--color-data)',
  events: 'var(--color-ok)',
}

/** Stacked bar: how much each factor (weight × score) adds to the confidence. */
export function ConfidenceBreakdown({ a }: { a: AnomalyDetail }) {
  const parts = a.confidence_breakdown ?? []
  if (parts.length === 0) return null
  return (
    <Panel className="flex flex-col gap-2 px-4 py-3.5">
      <Label>
        CONFIANZA {fmtConf(a.confidence)} · {confidenceLevel(a.confidence).toUpperCase()}
      </Label>
      <div className="flex h-3 overflow-hidden rounded-sm bg-panel-2" role="img" aria-label="Desglose de la confianza">
        {parts.map((p) => (
          <span key={p.key} title={`${p.label}: ${fmtNum(p.weight * p.score, 2)}`} style={{ width: `${p.weight * p.score * 100}%`, background: COMPONENT_COLOR[p.key] }} />
        ))}
      </div>
      <ul className="m-0 flex list-none flex-col gap-1 p-0">
        {parts.map((p) => (
          <li key={p.key} className="grid grid-cols-[10px_minmax(0,1fr)_auto] items-baseline gap-2 text-[12px]">
            <span className="h-2 w-2 rounded-full" style={{ background: COMPONENT_COLOR[p.key] }} />
            <span className="text-ink-2">
              {p.label} <span className="text-muted">· {p.detail}</span>
            </span>
            <span className="font-mono text-[11px] text-muted">
              {fmtNum(p.score, 2)} × {fmtNum(p.weight, 2)}
            </span>
          </li>
        ))}
      </ul>
      <span className="font-mono text-[10px] text-dim">tope 0,97: la confianza nunca es absoluta</span>
    </Panel>
  )
}

const cop = (n: number) => `$${fmtNum(n)} COP`

/** Energy, cost and reactive impact of keeping the current behavior for a month. */
export function ImpactPanel({ a, color }: { a: AnomalyDetail; color: string }) {
  const im = a.projected_impact
  if (!im) return null
  const extra = im.extra_kwh_per_day
  // A data-quality issue or a false positive does not add energy: the day-to-day noise is not a cost.
  const noEnergy = a.type === 'DATA_QUALITY' || a.type === 'FALSE_POSITIVE'
  return (
    <Panel className="flex flex-col gap-2 px-4 py-3.5">
      <Label>IMPACTO PROYECTADO · 30 DÍAS</Label>
      {noEnergy ? (
        <p className="m-0 text-[13px] text-ink-2">
          {a.type === 'DATA_QUALITY'
            ? `Sin energía extra: el consumo es estable. El impacto es de confianza: ${fmtNum(a.evidence.invalid_readings)} lecturas no deberían usarse para facturación ni reportes hasta validar el medidor.`
            : 'Sin impacto sostenido: el cambio fue temporal y coincide con un evento registrado.'}
        </p>
      ) : (
      <div className="grid grid-cols-2 gap-2">
        <div className="flex flex-col rounded border border-line bg-panel-2 px-3 py-2">
          <span className="font-mono text-[10px] text-muted">ENERGÍA EXTRA</span>
          <span className="font-mono text-base font-semibold whitespace-nowrap" style={{ color: extra > 0 ? color : undefined }}>
            {fmtNum(im.extra_kwh_per_month)} kWh
          </span>
          <span className="font-mono text-[10px] text-muted">{fmtNum(extra, 1)} kWh/día</span>
        </div>
        <div className="flex flex-col rounded border border-line bg-panel-2 px-3 py-2">
          <span className="font-mono text-[10px] text-muted">COSTO ESTIMADO</span>
          <span className="font-mono text-base font-semibold whitespace-nowrap">{cop(im.cost_per_month_cop)}</span>
          <span className="font-mono text-[10px] text-muted">a {fmtNum(im.tariff_cop_per_kwh)} COP/kWh</span>
        </div>
      </div>
      )}
      <ul className="m-0 flex list-none flex-col gap-1 p-0 text-[12px] text-ink-2">
        {!noEnergy && im.extra_kwh_so_far > 0 && <li>Ya se consumieron {fmtNum(im.extra_kwh_so_far)} kWh de más desde el cambio.</li>}
        <li>
          Reactiva: tan φ {fmtNum(im.reactive_ratio, 2)} con FP {fmtNum(im.power_factor, 2)} (≈ {fmtNum(im.reactive_kvarh_day)} kVArh/día)
          {im.reactive_excess > 0 ? `, ${fmtNum(im.reactive_excess, 2)} por encima del límite de 0,5 que suele penalizarse.` : ', dentro del límite de 0,5.'}
        </li>
      </ul>
      <span className="font-mono text-[10px] text-dim">estimación con la tarifa configurada; no es una factura</span>
    </Panel>
  )
}

/** Every event evaluated for the meter, with the reason it does or does not explain the change. */
export function EventTimeline({ a, events }: { a: AnomalyDetail; events: EventItem[] }) {
  const byId = new Map<string, EventItem>()
  for (const e of [...events, ...(a.evidence.related_events ?? [])]) byId.set(e.id, e)
  const list = [...byId.values()].sort((x, y) => x.timestamp.localeCompare(y.timestamp))
  return (
    <Panel className="flex flex-col gap-2 px-4 py-3.5">
      <Label>EVENTOS EVALUADOS · VENTANA ±3 H DEL CAMBIO</Label>
      {a.change_point_at && <span className="font-mono text-[11px] text-muted">cambio detectado {fmtDayHour(a.change_point_at)}{a.ended_at ? ` · terminó ${fmtDayHour(a.ended_at)}` : ''}</span>}
      {list.length === 0 ? (
        <span className="text-[13px] text-muted">Ningún evento registrado para este medidor. Por eso no se puede explicar por la operación.</span>
      ) : (
        <ol className="m-0 flex list-none flex-col gap-1.5 border-l border-line-2 p-0 pl-3">
          {list.map((e) => {
            const v = eventVerdict(e, a)
            const tone = v.explains === true ? 'var(--color-ok)' : v.explains === false ? 'var(--color-real)' : 'var(--color-data)'
            return (
              <li key={e.id} className="relative flex flex-col gap-0.5 rounded border border-line bg-panel-2 px-3 py-2">
                <span className="absolute top-3 -left-[17px] h-2 w-2 rounded-full" style={{ background: tone }} />
                <span className="font-mono text-[11px] text-expl">
                  {fmtDayHour(e.timestamp)} · {eventLabel(e.type).toUpperCase()}
                </span>
                <span className="text-[13px]">{e.description}</span>
                <span className="font-mono text-[10px]" style={{ color: tone }}>
                  {v.explains === true ? '✓' : v.explains === false ? '✗' : 'i'} {v.text}
                </span>
              </li>
            )
          })}
        </ol>
      )}
    </Panel>
  )
}

const ACTION_LABEL: Record<ActionKind, string> = {
  acknowledge: 'Reconocer',
  investigate: 'Investigar',
  validate: 'Validar medidor',
  resolve: 'Resolver',
  dismiss: 'Descartar',
  note: 'Agregar nota',
}

/** Buttons for the next actions, an optional note and the history of what was done. */
export function ActionsPanel({ a }: { a: AnomalyDetail }) {
  const [note, setNote] = useState('')
  const act = useAnomalyAction()
  const run = (action: ActionKind) =>
    act.mutate(
      { id: a.id, action, note: note.trim() || undefined },
      {
        onSuccess: () => setNote(''),
      },
    )
  const history = [...(a.actions ?? [])].reverse()
  const labels = lifecycleLabels(a.type)
  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap gap-1.5">
        {availableActions(a.type, a.status).map((k, i) => (
          <Button key={k} size="sm" variant={i === 0 ? 'primary' : 'ghost'} disabled={act.isPending} onClick={() => run(k)}>
            {ACTION_LABEL[k]}
          </Button>
        ))}
      </div>
      <label className="flex flex-col gap-1">
        <span className="font-mono text-[10px] text-muted">NOTA (OPCIONAL)</span>
        <textarea
          value={note}
          maxLength={2000}
          onChange={(e) => setNote(e.target.value)}
          rows={2}
          placeholder="Qué se revisó, quién va a sitio…"
          className="resize-y rounded border border-line-2 bg-panel-2 px-2 py-1.5 text-[13px] text-ink placeholder:text-dim focus:border-accent focus:outline-none"
        />
      </label>
      <Button size="sm" variant="link" className="self-start" disabled={act.isPending || !note.trim()} onClick={() => run('note')}>
        + AGREGAR NOTA
      </Button>
      {act.error && <span className="text-[12px] text-real">{(act.error as Error).message}</span>}
      {history.length > 0 && (
        <ol className="m-0 flex list-none flex-col gap-1 border-t border-line p-0 pt-2" aria-label="Historial de acciones">
          {history.map((h) => (
            <li key={h.id} className="flex flex-col text-[12px]">
              <span className="font-mono text-[10px] text-muted">
                {fmtDateTime(h.at)} · {h.actor} · {ACTION_LABEL[h.action] ?? h.action}
                {h.status ? ` → ${labels[h.status]}` : ''}
              </span>
              {h.note && <span className="text-ink-2">{h.note}</span>}
            </li>
          ))}
        </ol>
      )}
    </div>
  )
}
