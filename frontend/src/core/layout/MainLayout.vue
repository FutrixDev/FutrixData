<template>
  <div v-if="!authStore.ready" class="auth-gate" data-testid="auth-loading">
    <div class="auth-drag-region" style="--wails-draggable: drag"></div>
    <div class="auth-card">
      <div class="auth-card__logo">
        <img src="@/assets/svgs/logo.png" alt="FutrixData" />
      </div>
      <div class="auth-card__header">
        <div class="auth-card__spinner-ring">
          <svg class="auth-card__spinner" viewBox="0 0 50 50">
            <circle cx="25" cy="25" r="20" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-dasharray="90 150" stroke-dashoffset="0" />
          </svg>
        </div>
        <h1 class="auth-card__title">{{ tApp('auth.login.loadingTitle') }}</h1>
        <p class="auth-card__subtitle">{{ tApp('auth.login.loadingDescription') }}</p>
      </div>
    </div>
  </div>

  <div v-else class="app-shell app-shell-grid text-foreground" :class="{ 'ai-collapsed': !aiStore.isOpen }">
    <Sidebar class="app-nav" />

    <div class="app-main">
      <TitleBar />

      <div class="app-content">
        <NoticeBanner :message="store.notice.message" :type="store.notice.type" />

        <div
          v-if="updaterStore.hasUpdate && !updaterStore.dismissed"
          class="update-launch-banner"
          data-testid="update-launch-banner"
        >
          <span class="update-launch-banner__text">
            {{ tApp('my.account.update.availableNotice', { latest: updaterStore.result.latest }) }}
          </span>
          <div class="update-launch-banner__actions">
            <button
              v-if="updaterStore.canOpenDownload"
              type="button"
              class="update-launch-banner__update"
              data-testid="update-launch-banner-update"
              @click="onLaunchBannerUpdate"
            >
              {{ tApp('my.account.update.updateNow') }}
            </button>
            <button
              type="button"
              class="update-launch-banner__dismiss"
              data-testid="update-launch-banner-dismiss"
              @click="updaterStore.dismiss"
            >
              {{ tApp('my.account.update.dismiss') }}
            </button>
          </div>
        </div>

        <router-view v-slot="{ Component }">
          <component :is="Component" />
        </router-view>
      </div>
    </div>

    <AiSidebar class="app-ai" />

    <SkillInstallDialog v-if="showSkillDialog" @close="showSkillDialog = false" />

    <div
      v-if="showCodexConnectDialog"
      class="codex-connect-overlay"
      data-testid="codex-connect-dialog"
      role="dialog"
      aria-modal="true"
      aria-labelledby="codex-connect-title"
    >
      <section class="codex-connect-dialog">
        <h2 id="codex-connect-title">{{ tApp('codex.connect.title') }}</h2>
        <p>{{ tApp('codex.connect.desc') }}</p>
        <div class="codex-connect-dialog__actions">
          <button
            type="button"
            class="codex-connect-dialog__cancel"
            :disabled="codexConnectBusy"
            data-testid="codex-connect-cancel"
            @click="closeCodexConnectDialog"
          >
            {{ tApp('codex.connect.cancel') }}
          </button>
          <button
            type="button"
            class="codex-connect-dialog__confirm"
            :disabled="codexConnectBusy"
            data-testid="codex-connect-confirm"
            @click="confirmCodexConnect"
          >
            {{ codexConnectBusy ? tApp('codex.connect.authorizing') : tApp('codex.connect.confirm') }}
          </button>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { EventsOn } from '@wailsjs/runtime/runtime'
import { tApp } from '@/modules/i18n/appI18n'
import { useAppStore } from '@/stores/app'
import { useAiChatStore } from '@/stores/ai-chat'
import { useAuthStore } from '@/stores/auth'
import { useUpdaterStore } from '@/stores/updater'
import { api } from '@/services/api'
import TitleBar from '@/components/TitleBar.vue'
import NoticeBanner from '@/components/NoticeBanner.vue'
import SkillInstallDialog from '@/components/skill/SkillInstallDialog.vue'
import Sidebar from './Sidebar.vue'
import AiSidebar from '@/components/ai/AiSidebar.vue'

const store = useAppStore()
const aiStore = useAiChatStore()
const authStore = useAuthStore()
const updaterStore = useUpdaterStore()
const showSkillDialog = ref(false)
const showCodexConnectDialog = ref(false)
const codexConnectBusy = ref(false)
let aiRefreshTimer: number | null = null
let restoreFinished = false
const runtimeUnsubs: Array<() => void> = []

const stopAIRefresh = () => {
  if (aiRefreshTimer) {
    window.clearInterval(aiRefreshTimer)
    aiRefreshTimer = null
  }
}

const startAIRefresh = () => {
  stopAIRefresh()
  aiRefreshTimer = window.setInterval(() => {
    store.loadAIConfigs().catch(() => {})
  }, 60000)
}

const loadShell = async () => {
  await store.loadDatasources()
  await store.loadAIConfigs()
  startAIRefresh()
  if (authStore.isAuthenticated) {
    void updaterStore.check()
  }
}

const onLaunchBannerUpdate = async () => {
  try {
    await updaterStore.openDownload()
    // Only dismiss after the OS browser launch succeeded. If openDownload
    // throws (e.g. allowlist rejection, browser launch failure) we keep the
    // banner so the user can retry.
    updaterStore.dismiss()
  } catch (err) {
    store.setNotice(err instanceof Error ? err.message : String(err || ''), 'error')
  }
}

const closeCodexConnectDialog = () => {
  if (codexConnectBusy.value) return
  showCodexConnectDialog.value = false
}

const confirmCodexConnect = async () => {
  if (codexConnectBusy.value) return
  codexConnectBusy.value = true
  try {
    const result = await api.authorizeCodexPlugin()
    const outcome = result.installed.find(item => item.id === 'codex')
    if (outcome?.success) {
      store.setNotice(tApp('codex.connect.success'), 'success')
      showCodexConnectDialog.value = false
      return
    }
    store.setNotice(tApp('codex.connect.error', { message: outcome?.error || '' }), 'error')
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err || '')
    store.setNotice(tApp('codex.connect.error', { message }), 'error')
  } finally {
    codexConnectBusy.value = false
  }
}

const promptSkillInstall = async () => {
  try {
    const prompted = await api.skillInstallPrompted()
    if (prompted) return
    const [skillAgents, mcpAgents] = await Promise.all([
      api.detectAIAgents(),
      api.detectMCPAgents(),
    ])
    // Check per-type detection independently (same logic as SkillInstallDialog).
    const mcpById = new Map(mcpAgents.map(m => [m.id, m]))
    const hasInstallable = skillAgents.some(s => {
      const m = mcpById.get(s.id)
      return (s.detected && !s.installed) || (!!m?.detected && !m.installed)
    }) || mcpAgents.some(m => !skillAgents.find(s => s.id === m.id) && m.detected && !m.installed)
    if (hasInstallable) {
      showSkillDialog.value = true
    } else {
      await api.markSkillInstallPrompted()
    }
  } catch {
    // Skill prompt is non-critical; silently ignore errors.
  }
}

watch(
  () => authStore.isAuthenticated,
  (isAuthenticated, wasAuthenticated) => {
    if (!restoreFinished) return
    if (!isAuthenticated) {
      updaterStore.reset()
      return
    }
    if (!wasAuthenticated) {
      void updaterStore.check()
      void promptSkillInstall()
    }
  },
)

onMounted(async () => {
  const showCodexConnectPrompt = () => {
    showCodexConnectDialog.value = true
  }
  window.addEventListener('futrixdata:codex-connect-request', showCodexConnectPrompt)
  runtimeUnsubs.push(() => window.removeEventListener('futrixdata:codex-connect-request', showCodexConnectPrompt))
  if ((window as any).runtime) {
    runtimeUnsubs.push(EventsOn('auth:state', (next: any) => {
      void authStore.applyRuntimeState(next)
    }))
    runtimeUnsubs.push(EventsOn('auth:error', (message: string) => {
      authStore.applyRuntimeError(message)
    }))
    runtimeUnsubs.push(EventsOn('codex:connect-request', showCodexConnectPrompt))
  }
  await authStore.restore()
  restoreFinished = true
  await loadShell()
  if (authStore.isAuthenticated) {
    void promptSkillInstall()
  }
})

onBeforeUnmount(() => {
  stopAIRefresh()
  authStore.stopPolling()
  while (runtimeUnsubs.length > 0) {
    const unsub = runtimeUnsubs.pop()
    if (unsub) unsub()
  }
})
</script>

<style scoped>
.update-launch-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 16px;
  background: color-mix(in srgb, var(--color-primary, #2563eb) 12%, transparent);
  border-bottom: 1px solid color-mix(in srgb, var(--color-primary, #2563eb) 25%, transparent);
  font-size: 13px;
  color: var(--color-foreground, #0f172a);
}
.update-launch-banner__text {
  flex: 1;
  min-width: 0;
}
.update-launch-banner__actions {
  display: flex;
  gap: 8px;
}
.update-launch-banner__update,
.update-launch-banner__dismiss {
  border-radius: 6px;
  padding: 4px 10px;
  font-size: 12px;
  cursor: pointer;
  border: 1px solid transparent;
}
.update-launch-banner__update {
  background: var(--color-primary, #2563eb);
  color: #fff;
}
.update-launch-banner__update:hover {
  filter: brightness(0.95);
}
.update-launch-banner__dismiss {
  background: transparent;
  color: var(--color-foreground, #0f172a);
  border-color: color-mix(in srgb, var(--color-foreground, #0f172a) 20%, transparent);
}
.update-launch-banner__dismiss:hover {
  background: color-mix(in srgb, var(--color-foreground, #0f172a) 6%, transparent);
}

.codex-connect-overlay {
  position: fixed;
  inset: 0;
  z-index: 70;
  display: grid;
  place-items: center;
  padding: 20px;
  background: rgba(15, 23, 42, 0.42);
  backdrop-filter: blur(6px);
}
.codex-connect-dialog {
  width: min(420px, 100%);
  padding: 22px;
  border-radius: 8px;
  border: 1px solid color-mix(in srgb, var(--color-border, #d1d5db) 85%, transparent);
  background: var(--color-surface, #ffffff);
  color: var(--color-foreground, #0f172a);
  box-shadow: 0 24px 70px rgba(15, 23, 42, 0.24);
}
.codex-connect-dialog h2 {
  margin: 0 0 10px;
  font-size: 18px;
  line-height: 1.25;
}
.codex-connect-dialog p {
  margin: 0;
  color: var(--color-muted-foreground, #64748b);
  font-size: 14px;
  line-height: 1.55;
}
.codex-connect-dialog__actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 20px;
}
.codex-connect-dialog__cancel,
.codex-connect-dialog__confirm {
  min-height: 44px;
  border-radius: 7px;
  padding: 0 14px;
  font: inherit;
  font-size: 13px;
  font-weight: 700;
  cursor: pointer;
}
.codex-connect-dialog__cancel {
  border: 1px solid color-mix(in srgb, var(--color-foreground, #0f172a) 18%, transparent);
  background: transparent;
  color: var(--color-foreground, #0f172a);
}
.codex-connect-dialog__confirm {
  border: 1px solid transparent;
  background: var(--color-primary, #2563eb);
  color: #ffffff;
}
.codex-connect-dialog__cancel:disabled,
.codex-connect-dialog__confirm:disabled {
  cursor: wait;
  opacity: 0.7;
}
</style>
