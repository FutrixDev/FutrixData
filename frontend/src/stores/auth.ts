import { computed, getCurrentScope, onScopeDispose, ref } from 'vue'
import { defineStore } from 'pinia'

import { tApp } from '@/modules/i18n/appI18n'
import { evaluateLicense, resolvePlanLimitMessage } from '@/modules/plan/limits'
import { api } from '@/services/api'
import type { AuthDeviceInfo, AuthLicense, AuthLoginStart, AuthState } from '@/types'

const normalizeAuthState = (next?: Partial<AuthState> | null): AuthState => ({
  deviceId: String(next?.deviceId || ''),
  pendingLogin: next?.pendingLogin ?? null,
  session: next?.session ?? null,
  trial: next?.trial ?? null,
})

const errorMessage = (err: unknown) => {
  const raw = err instanceof Error ? err.message : String(err || '')
  return resolvePlanLimitMessage(raw, undefined) || raw
}

export const useAuthStore = defineStore('auth', () => {
  const state = ref<AuthState>(normalizeAuthState())
  const ready = ref(false)
  const restoring = ref(false)
  const loginBusy = ref(false)
  const error = ref('')
  const loginUrl = ref('')
  const manualCode = ref('')
  const devices = ref<AuthDeviceInfo[]>([])
  const deviceLimit = ref(0)
  const devicesLoading = ref(false)
  let pollTimer: number | null = null

  const isAuthenticated = computed(() => Boolean(state.value.session))
  const currentUser = computed(() => state.value.session?.user ?? null)
  const currentLicense = computed(() => state.value.session?.license ?? null)
  const currentTrial = computed(() => state.value.trial ?? null)
  // nowMs is a reactive clock. evaluateLicense() reads the current time, so
  // without a reactive `now` dependency the effective entitlement would only
  // recompute on a license/state mutation — a session active at render time
  // would stay "active" in the UI after expiresAt passes, while backend gates
  // (which call planlimits.EvaluateLicense per request) already treat it as
  // expired. Ticking nowMs forces effectiveLicense/effectivePlan to invalidate
  // around the expiry boundary so MyView, gate copy, and Pro affordances flip
  // in step with the backend.
  const nowMs = ref(Date.now())
  // 30s is well below typical Pro→Free transition latencies the user would
  // notice, and re-evaluating the cheap evaluateLicense() at this cadence has
  // no measurable cost.
  const NOW_TICK_MS = 30_000
  let nowTimer: ReturnType<typeof setInterval> | null = null
  const tickNow = () => { nowMs.value = Date.now() }
  // __setNowForTest lets tests simulate clock progression deterministically
  // (vitest fake timers don't drive computed re-evaluation unless the ref
  // value itself changes). Production callers should never invoke this.
  const __setNowForTest = (value: number) => { nowMs.value = value }
  if (typeof setInterval !== 'undefined') {
    nowTimer = setInterval(tickNow, NOW_TICK_MS)
  }
  const stopNowTicker = () => {
    if (nowTimer != null) {
      clearInterval(nowTimer)
      nowTimer = null
    }
  }
  // Pinia setup stores run inside an effect scope; tying cleanup to the scope
  // means HMR replacements, vitest re-`createPinia()` between tests, and any
  // future multi-mount setup all dispose the ticker instead of orphaning a
  // setInterval that keeps mutating a stale ref.
  if (getCurrentScope()) {
    onScopeDispose(stopNowTicker)
  }
  // effectiveLicense reconciles the raw stored license with the current time
  // so callers see an expired Pro session as Free with pro_expired status.
  const effectiveLicense = computed(() => evaluateLicense(currentLicense.value, nowMs.value, currentTrial.value))
  // No signed-in session still resolves through the same local entitlement:
  // active local trial behaves like Pro, otherwise the app falls back to Free.
  const effectivePlan = computed(() => effectiveLicense.value.effectivePlan)

  const applyLicenseUpdate = (next: AuthLicense | null | undefined) => {
    if (!next) return
    if (!state.value.session) return
    const current = state.value.session.license
    if (
      current
      && current.plan === next.plan
      && current.status === next.status
      && current.expiresAt === next.expiresAt
    ) {
      return
    }
    state.value = {
      ...state.value,
      session: {
        ...state.value.session,
        license: {
          plan: String(next.plan ?? ''),
          status: String(next.status ?? ''),
          expiresAt: Number(next.expiresAt ?? 0) || 0,
        },
      },
    }
  }

  const applyState = (next?: Partial<AuthState> | null) => {
    state.value = normalizeAuthState(next)
    loginUrl.value = state.value.pendingLogin?.loginUrl || ''
    if (!state.value.session) {
      devices.value = []
      deviceLimit.value = 0
    }
  }

  const stopPolling = () => {
    if (pollTimer) {
      window.clearInterval(pollTimer)
      pollTimer = null
    }
  }

  const loadDevices = async () => {
    if (!isAuthenticated.value) {
      devices.value = []
      deviceLimit.value = 0
      return
    }
    devicesLoading.value = true
    try {
      const result = await api.listAuthDevices()
      devices.value = result.devices || []
      deviceLimit.value = result.limit || 0
      applyLicenseUpdate(result.license)
    } finally {
      devicesLoading.value = false
    }
  }

  const restore = async () => {
    restoring.value = true
    try {
      const next = await api.ensureAuthenticated()
      applyState(next)
      error.value = ''
    } catch (err) {
      const message = errorMessage(err)
      if (message.toLowerCase().includes('login required')) {
        let current: Partial<AuthState> | null = null
        try {
          current = await api.currentAuth()
        } catch {
          current = state.value
        }
        applyState({ ...current, session: null, pendingLogin: null })
        error.value = ''
      } else {
        applyState({ ...state.value, session: null })
        error.value = message
      }
    } finally {
      ready.value = true
      restoring.value = false
    }
  }

  const pollOnce = async () => {
    const result = await api.pollAuthLogin()
    if (result.status === 'completed' && result.code) {
      stopPolling()
      const next = await api.completeAuthLogin(result.code)
      applyState(next)
      error.value = ''
      await loadDevices()
      return true
    }
    if (result.status === 'expired') {
      stopPolling()
      error.value = tApp('auth.login.expired')
      applyState({ ...state.value, pendingLogin: null })
      return true
    }
    return false
  }

  const startLogin = async (input: { noBrowser?: boolean } = {}): Promise<AuthLoginStart> => {
    loginBusy.value = true
    error.value = ''
    stopPolling()
    try {
      const started = await api.startAuthLogin(input)
      loginUrl.value = started.loginUrl
      state.value = {
        ...state.value,
        pendingLogin: {
          sessionId: started.sessionId,
          codeVerifier: '',
          loginUrl: started.loginUrl,
        },
      }
      pollTimer = window.setInterval(() => {
        void pollOnce().catch((err) => {
          stopPolling()
          error.value = errorMessage(err)
        })
      }, 2000)
      return started
    } catch (err) {
      error.value = errorMessage(err)
      throw err
    } finally {
      loginBusy.value = false
    }
  }

  const completeManualCode = async (code: string) => {
    loginBusy.value = true
    error.value = ''
    stopPolling()
    try {
      const next = await api.completeAuthLogin(code)
      applyState(next)
      manualCode.value = ''
      await loadDevices()
    } catch (err) {
      error.value = errorMessage(err)
      throw err
    } finally {
      loginBusy.value = false
    }
  }

  const logout = async () => {
    stopPolling()
    loginBusy.value = true
    try {
      const next = await api.logoutAuth()
      applyState(next)
      error.value = ''
    } finally {
      loginBusy.value = false
    }
  }

  const removeDevice = async (deviceId: string) => {
    const result = await api.removeAuthDevice(deviceId)
    devices.value = result.devices || []
    deviceLimit.value = result.limit || 0
    applyLicenseUpdate(result.license)
  }

  const applyRuntimeState = async (next?: Partial<AuthState> | null) => {
    stopPolling()
    applyState(next)
    error.value = ''
    await loadDevices()
  }

  const applyRuntimeError = (message: string) => {
    stopPolling()
    error.value = String(message || '')
  }

  return {
    state,
    ready,
    restoring,
    loginBusy,
    error,
    loginUrl,
    manualCode,
    devices,
    deviceLimit,
    devicesLoading,
    isAuthenticated,
    currentUser,
    currentLicense,
    effectiveLicense,
    effectivePlan,
    __setNowForTest,
    stopNowTicker,
    restore,
    startLogin,
    completeManualCode,
    loadDevices,
    logout,
    removeDevice,
    applyRuntimeState,
    applyRuntimeError,
    stopPolling,
  }
})
