export type ClientErrorReporter = (kind: string, message: string, detail: string) => Promise<unknown>

const stringifyDetail = (value: unknown) => {
  if (value instanceof Error) return value.stack || value.message
  if (typeof value === 'string') return value
  if (value === null || value === undefined) return ''
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

export const installClientErrorLogging = (report: ClientErrorReporter) => {
  const onError = (event: ErrorEvent) => {
    void report('error', String(event.message || 'Unknown error'), stringifyDetail(event.error || event.message)).catch((err) => {
      console.error('report client error failed', err)
    })
  }

  const onUnhandledRejection = (event: PromiseRejectionEvent) => {
    void report('unhandledrejection', 'Unhandled promise rejection', stringifyDetail(event.reason)).catch((err) => {
      console.error('report unhandled rejection failed', err)
    })
  }

  window.addEventListener('error', onError)
  window.addEventListener('unhandledrejection', onUnhandledRejection)

  return () => {
    window.removeEventListener('error', onError)
    window.removeEventListener('unhandledrejection', onUnhandledRejection)
  }
}
