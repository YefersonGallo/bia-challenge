import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useAuthStore } from '@/features/auth/authStore'

/** Avatar that opens a menu with the session, the dev mode link and an explicit "Cerrar sesión". */
export function UserMenu() {
  const user = useAuthStore((s) => s.user)
  const logout = useAuthStore((s) => s.logout)
  const [open, setOpen] = useState(false)
  const root = useRef<HTMLDivElement>(null)
  const first = useRef<HTMLAnchorElement>(null)

  useEffect(() => {
    if (!open) return
    first.current?.focus()
    const onDown = (e: MouseEvent) => {
      if (!root.current?.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  const name = user?.name ?? 'Operador'
  const initials = name
    .split(' ')
    .map((w) => w[0])
    .join('')
    .slice(0, 2)
    .toUpperCase()

  return (
    <div ref={root} className="relative">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={`Menú de usuario (${user?.email ?? name})`}
        className="flex h-8 items-center gap-1.5 rounded-full border border-line-2 bg-line py-0 pr-2 pl-0.5 text-ink hover:border-accent"
      >
        <span className="flex h-7 w-7 items-center justify-center rounded-full bg-raise text-[11px] font-semibold">{initials}</span>
        <span aria-hidden className={`text-[10px] text-muted transition-transform ${open ? 'rotate-180' : ''}`}>
          ▾
        </span>
      </button>
      {open && (
        <div role="menu" aria-label="Sesión" className="absolute top-full right-0 z-50 mt-2 w-64 rounded-md border border-line-2 bg-panel p-1.5 font-sans shadow-2xl whitespace-normal">
          <div className="flex flex-col gap-0.5 border-b border-line px-2.5 pt-1.5 pb-2.5">
            <span className="text-[13px] font-semibold text-ink">{name}</span>
            <span className="truncate font-mono text-[11px] text-muted">{user?.email}</span>
          </div>
          <Link
            ref={first}
            role="menuitem"
            to="/dev"
            onClick={() => setOpen(false)}
            className="mt-1 flex items-center justify-between rounded px-2.5 py-2 text-[13px] text-ink-2 no-underline hover:bg-raise focus:bg-raise focus:outline-none"
          >
            Dev mode · arquitectura
            <span aria-hidden className="font-mono text-[11px] text-muted">
              {'</>'}
            </span>
          </Link>
          <button
            type="button"
            role="menuitem"
            onClick={() => {
              setOpen(false)
              logout('user')
            }}
            className="flex w-full items-center justify-between rounded px-2.5 py-2 text-left text-[13px] font-semibold text-real hover:bg-raise focus:bg-raise focus:outline-none"
          >
            Cerrar sesión
            <span aria-hidden className="font-mono text-[13px]">
              ⏻
            </span>
          </button>
        </div>
      )}
    </div>
  )
}
