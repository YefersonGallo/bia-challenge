import { useUpdateAnomaly } from '@/shared/api/queries'
import type { AnomalyStatus, AnomalyType } from '@/shared/api/types'
import { LIFECYCLE, lifecycleLabels, nextAction, TYPE_META } from '@/shared/lib/labels'
import { Button } from '@/shared/ui/primitives'

/** Four-segment progress of an alarm through its lifecycle. */
export function LifecycleBar({ type, status }: { type: AnomalyType; status: AnomalyStatus }) {
  const labels = lifecycleLabels(type)
  const color = TYPE_META[type].color
  const reached = LIFECYCLE.indexOf(status)
  // A false positive jumps from new to closed: the middle states do not apply.
  const steps = type === 'FALSE_POSITIVE' ? LIFECYCLE.filter((s) => s === 'OPEN' || s === 'RESOLVED') : LIFECYCLE
  return (
    <ol className="grid gap-[3px]" style={{ gridTemplateColumns: `repeat(${steps.length}, minmax(0, 1fr))` }} aria-label="Ciclo de vida">
      {steps.map((s) => {
        const on = LIFECYCLE.indexOf(s) <= reached
        return (
          <li key={s} className="flex flex-col gap-[3px]" aria-current={s === status ? 'step' : undefined}>
            <span className="h-[3px] rounded-sm" style={{ background: on ? color : 'var(--color-line-2)' }} />
            <span className="font-mono text-[9px] uppercase" style={{ color: on ? 'var(--color-ink-2)' : 'var(--color-dim)' }}>
              {labels[s]}
            </span>
          </li>
        )
      })}
    </ol>
  )
}

/** Button that moves the anomaly to its next lifecycle state (or a "resolved" mark). */
export function NextActionButton({
  id,
  type,
  status,
  size = 'sm',
  className,
  colored = false,
}: {
  id: string
  type: AnomalyType
  status: AnomalyStatus
  size?: 'sm' | 'md'
  className?: string
  /** Paint the button with the anomaly color instead of the neutral primary. */
  colored?: boolean
}) {
  const update = useUpdateAnomaly()
  const next = nextAction(type, status)
  if (!next) {
    return <span className="self-center font-mono text-[11px] text-ok">✓ {lifecycleLabels(type).RESOLVED.toLowerCase()}</span>
  }
  return (
    <Button
      variant={colored ? 'solid' : 'primary'}
      tone={colored ? TYPE_META[type].color : undefined}
      size={size}
      className={className}
      disabled={update.isPending}
      onClick={() => update.mutate({ id, status: next.to })}
    >
      {next.label}
    </Button>
  )
}
