/** Fired right before the app opens the print dialog. */
export const BEFORE_PRINT = 'vatio:beforeprint'

/**
 * Opens the print dialog (Exportar PDF).
 *
 * Safari does not print while a request is still open: it waits for the page to
 * finish loading, and a streaming connection (the live replay) never finishes.
 * Listeners of BEFORE_PRINT close such connections first and reopen them on
 * `afterprint`; the short delay lets the closed connection settle.
 */
export function printPage() {
  window.dispatchEvent(new Event(BEFORE_PRINT))
  window.setTimeout(() => window.print(), 50)
}
