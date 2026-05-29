import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/services/api', () => ({
  api: {
    checkForUpdate: vi.fn(),
    openUpdateDownload: vi.fn(),
    ensureAuthenticated: vi.fn(),
    listAuthDevices: vi.fn(),
  },
}))

import { api } from '@/services/api'
import { useAuthStore } from '@/stores/auth'
import { useUpdaterStore } from '@/stores/updater'

const DISMISSED_STORAGE_KEY = 'futrix.updater.dismissedVersion'

const baseResult = {
  current: '1.0.17',
  latest: '',
  hasUpdate: false,
  downloadUrl: '',
  platformKey: 'macos-arm64',
  platformLabel: 'macOS (Apple Silicon)',
  releaseNotesUrl: 'https://futrixdata.com/#download',
  authenticated: true,
  lastCheckedAt: 1_700_000_000,
}

beforeEach(() => {
  // Other test files (e.g. ai-chat-store, app-i18n) replace localStorage on
  // globalThis with a non-Storage stub. Reinstall a Map-backed Storage here so
  // updater persistence is exercised reliably regardless of file order.
  const data = new Map<string, string>()
  const storage: Storage = {
    getItem: (key: string) => data.get(key) ?? null,
    setItem: (key: string, value: string) => { data.set(key, value) },
    removeItem: (key: string) => { data.delete(key) },
    clear: () => { data.clear() },
    key: (index: number) => Array.from(data.keys())[index] ?? null,
    get length() { return data.size },
  }
  vi.stubGlobal('localStorage', storage)
  Object.defineProperty(window, 'localStorage', { value: storage, configurable: true })
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

describe('updater store', () => {
  it('reports an available update when latest is newer and authenticated', async () => {
    ;(api as any).checkForUpdate.mockResolvedValue({
      ...baseResult,
      latest: '1.0.18',
      hasUpdate: true,
      downloadUrl: 'https://futrixdata.com/api/download/macos-arm64',
    })
    const store = useUpdaterStore()
    await store.check()
    expect(store.hasUpdate).toBe(true)
    expect(store.canOpenDownload).toBe(true)
    expect(store.error).toBe('')
  })

  it('reports up-to-date when latest equals current', async () => {
    ;(api as any).checkForUpdate.mockResolvedValue({
      ...baseResult,
      latest: '1.0.17',
      hasUpdate: false,
    })
    const store = useUpdaterStore()
    await store.check()
    expect(store.hasUpdate).toBe(false)
    expect(store.result.lastCheckedAt).toBe(1_700_000_000)
  })

  it('treats unauthenticated response as no update without surfacing an error', async () => {
    ;(api as any).checkForUpdate.mockResolvedValue({
      ...baseResult,
      authenticated: false,
      latest: '1.0.18',
      hasUpdate: false,
    })
    const store = useUpdaterStore()
    await store.check()
    expect(store.hasUpdate).toBe(false)
    expect(store.error).toBe('')
    expect(store.result.authenticated).toBe(false)
  })

  it('captures network errors on the store error field', async () => {
    ;(api as any).checkForUpdate.mockRejectedValue(new Error('network down'))
    const store = useUpdaterStore()
    await store.check()
    expect(store.error).toBe('network down')
    expect(store.hasUpdate).toBe(false)
  })

  it('re-syncs auth store when check returns authenticated=false while signed in', async () => {
    ;(api as any).checkForUpdate.mockResolvedValue({
      ...baseResult,
      authenticated: false,
    })
    ;(api as any).ensureAuthenticated.mockResolvedValue({
      deviceId: 'dev_1',
      pendingLogin: null,
      session: null,
    })
    const auth = useAuthStore()
    auth.state = {
      deviceId: 'dev_1',
      pendingLogin: null,
      session: { accessToken: 't', refreshToken: 'r', expiresAt: 0, user: {}, license: {} },
    } as any
    expect(auth.isAuthenticated).toBe(true)
    const store = useUpdaterStore()
    await store.check()
    expect((api as any).ensureAuthenticated).toHaveBeenCalled()
    expect(auth.isAuthenticated).toBe(false)
  })

  it('persists dismissal per version so the same release does not re-nag on restart', async () => {
    ;(api as any).checkForUpdate.mockResolvedValue({
      ...baseResult,
      latest: '1.0.18',
      hasUpdate: true,
      downloadUrl: 'https://futrixdata.com/api/download/macos-arm64',
    })
    const store = useUpdaterStore()
    await store.check()
    expect(store.dismissed).toBe(false)
    store.dismiss()
    expect(store.dismissed).toBe(true)
    expect(window.localStorage.getItem(DISMISSED_STORAGE_KEY)).toBe('1.0.18')

    // Simulate restart: fresh store re-reads the persisted dismissed version.
    setActivePinia(createPinia())
    const restarted = useUpdaterStore()
    await restarted.check()
    expect(restarted.dismissed).toBe(true)
  })

  it('un-dismisses automatically when a newer version becomes available', async () => {
    window.localStorage.setItem(DISMISSED_STORAGE_KEY, '1.0.18')
    ;(api as any).checkForUpdate.mockResolvedValue({
      ...baseResult,
      latest: '1.0.19',
      hasUpdate: true,
      downloadUrl: 'https://futrixdata.com/api/download/macos-arm64',
    })
    const store = useUpdaterStore()
    await store.check()
    expect(store.hasUpdate).toBe(true)
    expect(store.dismissed).toBe(false)
  })

  it('opens the download URL through the API', async () => {
    ;(api as any).checkForUpdate.mockResolvedValue({
      ...baseResult,
      latest: '1.0.18',
      hasUpdate: true,
      downloadUrl: 'https://futrixdata.com/api/download/macos-arm64',
    })
    ;(api as any).openUpdateDownload.mockResolvedValue(undefined)
    const store = useUpdaterStore()
    await store.check()
    await store.openDownload()
    expect((api as any).openUpdateDownload).toHaveBeenCalledWith(
      'https://futrixdata.com/api/download/macos-arm64',
    )
  })
})
