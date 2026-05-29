export const normalizeError = (err: unknown): string => {
  if (err instanceof Error) return err.message
  if (typeof err === 'string') return err
  if (err && typeof err === 'object' && 'message' in err) {
    return String((err as { message?: unknown }).message ?? 'Request failed')
  }
  return 'Request failed'
}

export const hasWailsBindings = () => {
  if (typeof window === 'undefined') return false
  const root = (window as { go?: { main?: { App?: unknown } } }).go?.main?.App
  return Boolean(root)
}

export const shouldUseMock = () => import.meta.env.DEV && !hasWailsBindings()

export const call = async <T>(fn: () => Promise<T>): Promise<T> => {
  if (!hasWailsBindings()) {
    throw new Error('Wails runtime is not available. Run via Wails to use backend actions.')
  }
  try {
    return await fn()
  } catch (err) {
    throw new Error(normalizeError(err))
  }
}

export const withMock = async <T>(fn: () => Promise<T>, mockFn: () => Promise<T>): Promise<T> => {
  if (shouldUseMock()) {
    return mockFn()
  }
  return call(fn)
}

export const cloneJson = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T

export const newId = (prefix: string) =>
  `${prefix}_${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`
