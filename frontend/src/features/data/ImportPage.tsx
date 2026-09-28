import { useRef, useState, type DragEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useImportReadings, useMeters } from '@/shared/api/queries'
import type { ImportResult } from '@/shared/api/types'
import { ApiError } from '@/shared/api/client'
import { fmtDateTime, fmtNum } from '@/shared/lib/format'
import { cx } from '@/shared/lib/cx'
import { Button, Label, Panel } from '@/shared/ui/primitives'
import { useAnalysis } from '@/features/analysis/useAnalysis'

/** Same limit as the API (10 MB ≈ 150 000 hourly readings). */
export const MAX_BYTES = 10 * 1024 * 1024

const COLUMNS: [string, string, string][] = [
  ['meter_id', 'Medidor existente en la planta', 'M-109'],
  ['timestamp', 'Fecha y hora UTC, una lectura por hora', '2026-09-15 00:00:00'],
  ['consumption_kwh', 'Consumo de la hora (kWh)', '312.4'],
  ['voltage_v', 'Voltaje (V)', '219.8'],
  ['current_a', 'Corriente (A)', '421.6'],
  ['power_factor', 'Factor de potencia (0–1)', '0.74'],
  ['status', 'Opcional; el análisis no lo usa', 'OK'],
]

function template(meters: string[]) {
  const ids = meters.length ? meters.slice(0, 2) : ['M-101', 'M-102']
  const rows = ids.flatMap((id) => [0, 1].map((h) => `${id},2026-09-15 0${h}:00:00,24.5,220.4,110.2,0.95,OK`))
  return ['meter_id,timestamp,consumption_kwh,voltage_v,current_a,power_factor,status', ...rows].join('\n') + '\n'
}

function download(name: string, text: string) {
  const url = URL.createObjectURL(new Blob([text], { type: 'text/csv' }))
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  URL.revokeObjectURL(url)
}

const message = (e: unknown) => (e instanceof ApiError ? e.message : 'No se pudo leer el archivo. Revisa la conexión e inténtalo de nuevo.')

function Summary({ r }: { r: ImportResult }) {
  const items = [
    { l: 'FILAS', v: fmtNum(r.rows) },
    { l: 'NUEVAS', v: fmtNum(r.added), c: 'var(--color-ok)' },
    { l: 'REEMPLAZAN', v: fmtNum(r.replaced), c: r.replaced ? 'var(--color-expl)' : undefined },
    { l: 'MEDIDORES', v: String(r.meters.length) },
  ]
  return (
    <div className="flex flex-col gap-3">
      <div className="grid grid-cols-2 gap-px overflow-hidden rounded-md border border-line bg-line sm:grid-cols-4">
        {items.map((k) => (
          <div key={k.l} className="flex flex-col gap-0.5 bg-panel-2 px-4 py-3">
            <Label>{k.l}</Label>
            <span className="font-mono text-xl font-semibold" style={{ color: k.c }}>
              {k.v}
            </span>
          </div>
        ))}
      </div>
      <span className="font-mono text-[12px] text-ink-2">
        {fmtDateTime(r.from)} → {fmtDateTime(r.to)} · {r.meters.join(', ')}
      </span>
      {r.warnings.map((w) => (
        <span key={w} className="text-[13px] text-expl">
          ⚠ {w}
        </span>
      ))}
    </div>
  )
}

/** Upload of hourly readings in the format of readings.csv: validate, preview, import. */
export function ImportPage() {
  const navigate = useNavigate()
  const input = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [tooBig, setTooBig] = useState(false)
  const [over, setOver] = useState(false)
  const check = useImportReadings()
  const apply = useImportReadings()
  const { data: meterList = [] } = useMeters()
  const meters = [...meterList].sort((a, b) => a.id.localeCompare(b.id))
  const analysis = useAnalysis()

  const choose = (f: File | undefined) => {
    if (!f) return
    apply.reset()
    setFile(f)
    setTooBig(f.size > MAX_BYTES)
    if (f.size > MAX_BYTES) return check.reset()
    check.mutate({ file: f, dryRun: true })
  }
  const onDrop = (e: DragEvent) => {
    e.preventDefault()
    setOver(false)
    choose(e.dataTransfer.files[0])
  }
  const reset = () => {
    setFile(null)
    setTooBig(false)
    check.reset()
    apply.reset()
    if (input.current) input.current.value = ''
  }

  const preview = check.data
  const done = apply.data

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
      <div className="mx-auto flex w-full max-w-[1180px] flex-col gap-6 px-6 pt-5 pb-10">
        <div className="flex flex-col gap-1">
          <Label>DATOS</Label>
          <h1 className="m-0 font-display text-2xl font-semibold">Cargar lecturas</h1>
          <p className="m-0 max-w-[760px] text-[14px] text-soft">
            Sube un CSV con el mismo formato de <span className="font-mono text-ink-2">readings.csv</span>. Las lecturas nuevas se agregan y las de un
            medidor y hora que ya existen se reemplazan. Antes de guardar nada verás qué cambia; después, ejecuta el análisis para incluirlas.
          </p>
        </div>

        <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)]">
          <Panel className="flex flex-col gap-3 p-4">
            <div className="flex items-center justify-between gap-3">
              <Label>FORMATO</Label>
              <Button size="sm" onClick={() => download('vatio-plantilla-lecturas.csv', template(meters.map((m) => m.id)))}>
                DESCARGAR PLANTILLA
              </Button>
            </div>
            <table className="w-full border-collapse text-left text-[13px]">
              <thead>
                <tr className="font-mono text-[10px] tracking-[0.08em] text-muted">
                  <th className="border-b border-line py-1.5 pr-3 font-medium">COLUMNA</th>
                  <th className="border-b border-line py-1.5 pr-3 font-medium">QUÉ ES</th>
                  <th className="border-b border-line py-1.5 font-medium">EJEMPLO</th>
                </tr>
              </thead>
              <tbody>
                {COLUMNS.map(([c, d, ex]) => (
                  <tr key={c}>
                    <td className="border-b border-line py-1.5 pr-3 font-mono text-[12px] font-semibold text-ink">{c}</td>
                    <td className="border-b border-line py-1.5 pr-3 text-ink-2">{d}</td>
                    <td className="border-b border-line py-1.5 font-mono text-[12px] text-muted">{ex}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            <ul className="m-0 flex list-disc flex-col gap-1 pl-4 text-[12px] text-muted marker:text-line-2">
              <li>Separador coma, o punto y coma con coma decimal (Excel en español).</li>
              <li>Todas las columnas son obligatorias salvo status; una fila vacía o mal escrita rechaza el archivo y se indica la línea.</li>
              <li>
                Medidores válidos: <span className="font-mono">{meters.map((m) => m.id).join(', ') || '…'}</span>
              </li>
              <li>Máximo 10 MB.</li>
            </ul>
          </Panel>

          <div className="flex flex-col gap-4">
            {!done && (
              <label
                onDragOver={(e) => {
                  e.preventDefault()
                  setOver(true)
                }}
                onDragLeave={() => setOver(false)}
                onDrop={onDrop}
                className={cx(
                  'flex cursor-pointer flex-col items-center justify-center gap-2 rounded-md border border-dashed px-6 py-10 text-center transition-colors',
                  over ? 'border-accent bg-raise' : 'border-line-2 bg-panel hover:border-muted',
                )}
              >
                <span className="font-mono text-[12px] font-semibold tracking-[0.08em] text-ink">{file ? file.name : 'ARRASTRA EL CSV AQUÍ'}</span>
                <span className="text-[13px] text-muted">{file ? `${fmtNum(file.size / 1024, 1)} KB · elegir otro archivo` : 'o haz clic para elegirlo'}</span>
                <input
                  ref={input}
                  type="file"
                  accept=".csv,text/csv"
                  aria-label="Archivo CSV de lecturas"
                  className="sr-only"
                  onChange={(e) => choose(e.target.files?.[0])}
                />
              </label>
            )}

            {tooBig && (
              <span role="alert" className="text-[13px] text-real">
                El archivo pesa más de 10 MB. Divídelo en varios archivos.
              </span>
            )}
            {check.isPending && <span className="font-mono text-[12px] text-accent">VALIDANDO…</span>}
            {check.isError && (
              <Panel className="flex flex-col gap-1 p-4" accent="var(--color-real)">
                <span className="font-mono text-[11px] font-semibold text-real">EL ARCHIVO NO SE PUEDE CARGAR</span>
                <span role="alert" className="text-[14px] text-ink">
                  {message(check.error)}
                </span>
                <span className="text-[12px] text-muted">No se guardó nada. Corrige el archivo y vuelve a subirlo.</span>
              </Panel>
            )}

            {preview && !done && (
              <Panel className="flex flex-col gap-4 p-4" accent="var(--color-accent)">
                <span className="font-mono text-[11px] font-semibold text-accent">VISTA PREVIA · TODAVÍA NO SE GUARDA</span>
                <Summary r={preview} />
                {apply.isError && (
                  <span role="alert" className="text-[13px] text-real">
                    {message(apply.error)}
                  </span>
                )}
                <div className="flex flex-wrap gap-2">
                  <Button variant="primary" disabled={apply.isPending} onClick={() => file && apply.mutate({ file, dryRun: false })}>
                    {apply.isPending ? 'IMPORTANDO…' : `IMPORTAR ${fmtNum(preview.added + preview.replaced)} LECTURAS`}
                  </Button>
                  <Button onClick={reset} disabled={apply.isPending}>
                    CANCELAR
                  </Button>
                </div>
              </Panel>
            )}

            {done && (
              <Panel className="flex flex-col gap-4 p-4" accent="var(--color-ok)">
                <span role="status" className="font-mono text-[11px] font-semibold text-ok">
                  LECTURAS IMPORTADAS
                </span>
                <Summary r={done} />
                <span className="text-[13px] text-ink-2">El último análisis todavía no las incluye. Ejecútalo para ver las anomalías con los datos nuevos.</span>
                <div className="flex flex-wrap gap-2">
                  <Button
                    variant="primary"
                    disabled={analysis.running}
                    onClick={() => analysis.start(() => navigate('/'))}
                  >
                    {analysis.running ? 'ANALIZANDO…' : 'EJECUTAR ANÁLISIS'}
                  </Button>
                  <Button onClick={reset}>CARGAR OTRO ARCHIVO</Button>
                </div>
              </Panel>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
