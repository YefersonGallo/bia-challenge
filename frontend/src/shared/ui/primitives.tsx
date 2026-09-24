import type { ButtonHTMLAttributes, CSSProperties, ReactNode } from 'react'
import { cx } from '@/shared/lib/cx'

/** Status lamp: a small glowing dot. */
export function Lamp({ color, glow = true, size = 9 }: { color: string; glow?: boolean; size?: number }) {
  return (
    <span
      aria-hidden
      className="inline-block shrink-0 rounded-full"
      style={{ width: size, height: size, background: color, boxShadow: glow ? `0 0 8px ${color}` : undefined }}
    />
  )
}

/** Solid tag with dark text, used for P1/P2… and classification chips. */
export function Tag({ color, children, outline = false }: { color: string; children: ReactNode; outline?: boolean }) {
  return (
    <span
      className="inline-flex items-center rounded-[3px] px-2 py-0.5 font-mono text-[11px] font-semibold whitespace-nowrap"
      style={outline ? { border: `1px solid ${color}`, color } : { background: color, color: 'var(--color-bg)' }}
    >
      {children}
    </span>
  )
}

type Variant = 'primary' | 'ghost' | 'link' | 'solid'
interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant
  size?: 'sm' | 'md'
  /** Background for the `solid` variant (e.g. the anomaly color). */
  tone?: string
}

/** Mono uppercase button of the control room. */
export function Button({ variant = 'ghost', size = 'md', tone, className, style, ...rest }: ButtonProps) {
  const base = 'inline-flex items-center justify-center gap-2 rounded font-mono font-semibold tracking-[0.06em] transition-colors disabled:cursor-not-allowed disabled:opacity-50'
  const sizes = { sm: 'h-[30px] px-2.5 text-[10px]', md: 'h-[34px] px-3.5 text-[11px]' }
  const variants: Record<Variant, string> = {
    primary: 'bg-ink text-bg hover:bg-white',
    ghost: 'border border-line-2 bg-transparent text-ink hover:border-muted',
    link: 'bg-transparent text-accent hover:underline px-1',
    solid: 'text-bg',
  }
  const s: CSSProperties = variant === 'solid' ? { background: tone, ...style } : { ...style }
  return <button type="button" className={cx(base, sizes[size], variants[variant], className)} style={s} {...rest} />
}

/** Section label (mono, 10px, uppercase). */
export function Label({ children, className }: { children: ReactNode; className?: string }) {
  return <span className={cx('label', className)}>{children}</span>
}

/** Bordered surface. */
export function Panel({
  children,
  className,
  accent,
  as: As = 'section',
  ...rest
}: {
  children: ReactNode
  className?: string
  accent?: string
  as?: 'section' | 'div' | 'aside'
  'aria-label'?: string
}) {
  return (
    <As
      className={cx('rounded-md border bg-panel', !accent && 'border-line', className)}
      style={accent ? { borderColor: accent } : undefined}
      {...rest}
    >
      {children}
    </As>
  )
}

/** A row of KPIs separated by hairlines. */
export function KpiStrip({ items }: { items: { label: string; value: ReactNode; sub?: ReactNode; color?: string }[] }) {
  return (
    <div
      className="grid gap-px overflow-hidden rounded-md border border-line bg-line"
      style={{ gridTemplateColumns: `repeat(${items.length}, minmax(0, 1fr))` }}
    >
      {items.map((k) => (
        <div key={k.label} className="flex flex-col gap-0.5 bg-panel-2 px-4 py-3">
          <Label>{k.label}</Label>
          <span className="font-mono text-2xl font-medium" style={{ color: k.color }}>
            {k.value}
          </span>
          {k.sub && <span className="text-[11px] text-muted">{k.sub}</span>}
        </div>
      ))}
    </div>
  )
}

/** Dashed empty state with an optional call to action. */
export function EmptyState({ title, children, action }: { title: string; children?: ReactNode; action?: ReactNode }) {
  return (
    <div className="flex flex-col gap-2.5 rounded-md border border-dashed border-line-2 p-5">
      <span className="text-[15px] text-ink">{title}</span>
      {children && <span className="text-[13px] text-muted">{children}</span>}
      {action && <div className="pt-1">{action}</div>}
    </div>
  )
}

/** Toggle-button group used for filters and sort. */
export function Segmented<T extends string>({
  label,
  value,
  options,
  onChange,
}: {
  label: string
  value: T
  options: { value: T; label: ReactNode; dot?: string }[]
  onChange: (v: T) => void
}) {
  return (
    <div role="group" aria-label={label} className="flex gap-1">
      {options.map((o) => {
        const active = o.value === value
        return (
          <button
            key={o.value}
            type="button"
            aria-pressed={active}
            onClick={() => onChange(o.value)}
            className={cx(
              'flex h-8 items-center gap-1.5 rounded border px-2.5 font-mono text-[11px] font-semibold',
              active ? 'border-ink bg-ink text-bg' : 'border-line-2 bg-transparent text-ink-2 hover:border-muted',
            )}
          >
            {o.dot && <span className="h-[7px] w-[7px] rounded-full" style={{ background: o.dot }} />}
            {o.label}
          </button>
        )
      })}
    </div>
  )
}

/** Mono search box for meter ids. */
export function SearchBox({
  value,
  onChange,
  placeholder,
  label,
  className,
}: {
  value: string
  onChange: (v: string) => void
  placeholder: string
  label: string
  className?: string
}) {
  return (
    <label
      className={cx(
        'flex h-8 items-center gap-2 rounded border bg-panel-2 px-2.5',
        value ? 'border-accent' : 'border-line-2',
        className,
      )}
    >
      <svg width="13" height="13" viewBox="0 0 24 24" aria-hidden className="fill-none stroke-muted" strokeWidth={2.2} strokeLinecap="round">
        <circle cx="11" cy="11" r="7" />
        <path d="M20 20l-4-4" />
      </svg>
      <span className="sr-only">{label}</span>
      <input
        type="search"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="min-w-0 flex-1 border-0 bg-transparent font-mono text-xs text-ink outline-none placeholder:text-dim"
      />
    </label>
  )
}

/** Loading and error placeholders shared by pages. */
export function Loading({ what = 'datos' }: { what?: string }) {
  return (
    <div role="status" className="p-6 font-mono text-xs text-muted">
      Cargando {what}…
    </div>
  )
}

export function ErrorBox({ error }: { error: unknown }) {
  const msg = error instanceof Error ? error.message : 'Error inesperado'
  return (
    <div role="alert" className="m-6 rounded-md border border-real/50 bg-real/10 p-4 text-sm text-ink">
      No se pudo cargar la información: {msg}
    </div>
  )
}
