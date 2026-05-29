import { beforeEach, describe, expect, it } from 'vitest'
import { ensureMonacoEnvironment, monacoWorkerKindForLabel } from '@/components/consoleMonacoEnvironment'

type MonacoEnvironmentTestHost = typeof globalThis & {
  MonacoEnvironment?: {
    getWorker?: unknown
    customFlag?: boolean
  }
  __futrixMonacoEnvironmentReady?: boolean
}

const host = globalThis as MonacoEnvironmentTestHost

describe('console Monaco environment', () => {
  beforeEach(() => {
    delete host.MonacoEnvironment
    delete host.__futrixMonacoEnvironmentReady
  })

  it('maps Monaco language labels to Vite worker bundles', () => {
    expect(monacoWorkerKindForLabel('json')).toBe('json')
    expect(monacoWorkerKindForLabel('scss')).toBe('css')
    expect(monacoWorkerKindForLabel('handlebars')).toBe('html')
    expect(monacoWorkerKindForLabel('javascript')).toBe('typescript')
    expect(monacoWorkerKindForLabel('sql')).toBe('editor')
  })

  it('installs a worker factory before Monaco is loaded', () => {
    host.MonacoEnvironment = { customFlag: true }

    ensureMonacoEnvironment()

    expect(host.MonacoEnvironment?.customFlag).toBe(true)
    expect(typeof host.MonacoEnvironment?.getWorker).toBe('function')
    expect(host.__futrixMonacoEnvironmentReady).toBe(true)
  })
})
