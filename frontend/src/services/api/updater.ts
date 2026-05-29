import { cloneJson, hasWailsBindings, normalizeError } from './core'

export interface UpdaterResult {
  current: string
  latest: string
  hasUpdate: boolean
  downloadUrl: string
  platformKey: string
  platformLabel: string
  releaseNotesUrl: string
  authenticated: boolean
  lastCheckedAt: number
}

const emptyResult = (): UpdaterResult => ({
  current: '',
  latest: '',
  hasUpdate: false,
  downloadUrl: '',
  platformKey: '',
  platformLabel: '',
  releaseNotesUrl: '',
  authenticated: false,
  lastCheckedAt: 0,
})

const mockResult: UpdaterResult = {
  current: '1.0.27',
  latest: '1.0.27',
  hasUpdate: false,
  downloadUrl: '',
  platformKey: 'macos-arm64',
  platformLabel: 'macOS (Apple Silicon)',
  releaseNotesUrl: 'https://futrixdata.com/#release-notes',
  authenticated: true,
  lastCheckedAt: Math.floor(Date.now() / 1000),
}

const wailsApp = (): {
  CheckForUpdate?: () => Promise<UpdaterResult>
  OpenUpdateDownload?: (url: string) => Promise<void>
} | null => {
  if (typeof window === 'undefined') return null
  return (window as any).go?.main?.App ?? null
}

export const updaterApi = {
  checkForUpdate: async (): Promise<UpdaterResult> => {
    const app = wailsApp()
    if (!hasWailsBindings() || !app?.CheckForUpdate) {
      return cloneJson(mockResult)
    }
    try {
      const raw = (await app.CheckForUpdate()) as Partial<UpdaterResult> | null
      return { ...emptyResult(), ...(raw || {}) }
    } catch (err) {
      throw new Error(normalizeError(err))
    }
  },
  openUpdateDownload: async (url: string): Promise<void> => {
    const app = wailsApp()
    if (!hasWailsBindings() || !app?.OpenUpdateDownload) {
      if (typeof window !== 'undefined') {
        window.open(url, '_blank', 'noopener,noreferrer')
      }
      return
    }
    try {
      await app.OpenUpdateDownload(url)
    } catch (err) {
      throw new Error(normalizeError(err))
    }
  },
}
