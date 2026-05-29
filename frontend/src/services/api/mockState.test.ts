import { afterEach, describe, expect, it, vi } from 'vitest'

describe('loadMockState', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
    vi.resetModules()
  })

  it('loads fixture JSON in dev browser mode', async () => {
    vi.stubEnv('DEV', 'true')
    vi.stubEnv('MODE', 'development')

    const { loadMockState } = await import('./mockState')
    const state = await loadMockState()

    expect(state.datasources.some((item) => item.id === 'ds_mysql')).toBe(true)
    expect(Array.isArray(state.aiConfigs)).toBe(true)
  })

  it('resolves fixture loaders by normalized suffix when exact key lookup misses', async () => {
    const { findRuntimeJsonLoader } = await import('./mockState')

    const direct = async () => ({})
    const nested = async () => ({})
    const loader = findRuntimeJsonLoader(
      {
        '/@fs/Users/test/repo/data/datasources.json': nested,
        '../../../../data/entities.json': direct,
      },
      '../../../../data/datasources.json',
    )

    expect(loader).toBe(nested)
  })
})
