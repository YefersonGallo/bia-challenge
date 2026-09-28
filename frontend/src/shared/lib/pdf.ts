/** A4 in millimetres and the blank band kept at the top and bottom of every page. */
const PAGE_W = 210
const PAGE_H = 297
const MARGIN = 10
const SCALE = 2 // render at 2× for sharp text

/**
 * Where a page may end, in CSS px from the top of `root`: the bottom edge of
 * every block (sections, paragraphs, table rows, list items, cards). Cutting
 * there never splits a line of text in half.
 */
export function breakPoints(root: HTMLElement): number[] {
  const top = root.getBoundingClientRect().top
  const out = new Set<number>()
  root.querySelectorAll<HTMLElement>('header, section, section > *, p, tr, li, h2, h3, table, [data-pdf-block]').forEach((n) => {
    const b = Math.round(n.getBoundingClientRect().bottom - top)
    if (b > 0) out.add(b)
  })
  return [...out].sort((a, b) => a - b)
}

/**
 * Splits a document `total` px tall into pages of at most `page` px, ending each
 * page at the last break point that fits. Only when no break point falls in the
 * lower half of a page is the page cut at its full height.
 */
export function paginate(total: number, page: number, breaks: number[]): [number, number][] {
  const out: [number, number][] = []
  let y = 0
  while (y < total - 1) {
    const limit = y + page
    if (limit >= total) {
      out.push([y, total])
      break
    }
    let end = limit
    for (const b of breaks) {
      if (b > limit) break
      if (b > y + page / 2) end = b
    }
    out.push([y, end])
    y = end
  }
  return out
}

/**
 * Renders `root` as it looks on screen and downloads it as an A4 PDF, paginated
 * at block boundaries and with a page footer. Nodes marked `data-pdf-skip` are
 * left out. The libraries load on demand, only when the user asks for a PDF.
 */
export async function downloadPdf(root: HTMLElement, filename: string, footer: string) {
  const [{ domToCanvas }, { jsPDF }] = await Promise.all([import('modern-screenshot'), import('jspdf')])
  const width = root.offsetWidth
  const breaks = breakPoints(root)
  const bg = getComputedStyle(root).backgroundColor || '#ffffff'
  const canvas = await domToCanvas(root, {
    scale: SCALE,
    backgroundColor: bg,
    filter: (n) => !(n instanceof HTMLElement && 'pdfSkip' in n.dataset),
  })

  const pxPerMm = width / PAGE_W
  const pages = paginate(canvas.height / SCALE, (PAGE_H - 2 * MARGIN) * pxPerMm, breaks)
  const pdf = new jsPDF({ unit: 'mm', format: 'a4', orientation: 'portrait', compress: true })
  pages.forEach(([y0, y1], i) => {
    if (i > 0) pdf.addPage()
    const slice = document.createElement('canvas')
    slice.width = canvas.width
    slice.height = Math.ceil((y1 - y0) * SCALE)
    slice.getContext('2d')!.drawImage(canvas, 0, -y0 * SCALE)
    pdf.addImage(slice.toDataURL('image/jpeg', 0.92), 'JPEG', 0, MARGIN, PAGE_W, (y1 - y0) / pxPerMm, undefined, 'FAST')
    pdf.setFontSize(7)
    pdf.setTextColor(120)
    pdf.text(`${footer} · página ${i + 1} de ${pages.length}`, PAGE_W / 2, PAGE_H - 4, { align: 'center' })
  })
  pdf.save(filename)
}
