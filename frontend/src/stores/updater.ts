import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import { api } from '@/services/api'
import type { UpdaterResult } from '@/services/api/updater'
import { useAuthStore } from '@/stores/auth'

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

const DISMISSED_STORAGE_KEY = 'futrix.updater.dismissedVersion'

const safeStorage = (): Storage | null => {
  try {
    return typeof window !== 'undefined' ? window.localStorage : null
  } catch {
    return null
  }
}

const loadDismissedVersion = (): string => {
  const storage = safeStorage()
  if (!storage) return ''
  try {
    return storage.getItem(DISMISSED_STORAGE_KEY) || ''
  } catch {
    return ''
  }
}

const persistDismissedVersion = (version: string) => {
  const storage = safeStorage()
  if (!storage) return
  try {
    if (version) storage.setItem(DISMISSED_STORAGE_KEY, version)
    else storage.removeItem(DISMISSED_STORAGE_KEY)
  } catch {
    // localStorage is best-effort; ignore quota / privacy-mode failures.
  }
}

export const useUpdaterStore = defineStore('updater', () => {
  const result = ref<UpdaterResult>(emptyResult())
  const loading = ref(false)
  const error = ref('')
  // Persist the version the user dismissed so we don't re-nag on restart for
  // the same release. A newer `latest` automatically un-dismisses the banner.
  const dismissedVersion = ref<string>(loadDismissedVersion())

  const dismissed = computed(() => {
    const latest = result.value.latest
    return Boolean(latest) && dismissedVersion.value === latest
  })

  const hasUpdate = computed(() => result.value.authenticated && result.value.hasUpdate && Boolean(result.value.latest))
  const canOpenDownload = computed(() => hasUpdate.value && Boolean(result.value.downloadUrl))

  const check = async (): Promise<void> => {
    if (loading.value) return
    loading.value = true
    error.value = ''
    try {
      const next = await api.checkForUpdate()
      result.value = next
      // The Go updater treats 401 as authenticated=false (the backend session
      // has been cleared by auth.GetJSON). If the Pinia auth store still
      // thinks we're signed in, re-hydrate it so the UI doesn't keep a stale
      // signed-in shell.
      if (!next.authenticated) {
        const authStore = useAuthStore()
        if (authStore.isAuthenticated) {
          await authStore.restore()
        }
      }
    } catch (err) {
      error.value = err instanceof Error ? err.message : String(err || '')
    } finally {
      loading.value = false
    }
  }

  const openDownload = async (): Promise<void> => {
    const url = result.value.downloadUrl || result.value.releaseNotesUrl
    if (!url) return
    await api.openUpdateDownload(url)
  }

  const dismiss = () => {
    const latest = result.value.latest
    if (!latest) return
    dismissedVersion.value = latest
    persistDismissedVersion(latest)
  }

  const reset = () => {
    result.value = emptyResult()
    error.value = ''
    // Keep dismissedVersion in localStorage across resets so sign-out / sign-in
    // doesn't re-nag for an already-dismissed release.
  }

  return {
    result,
    loading,
    error,
    dismissedVersion,
    dismissed,
    hasUpdate,
    canOpenDownload,
    check,
    openDownload,
    dismiss,
    reset,
  }
})
