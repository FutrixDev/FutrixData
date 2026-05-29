<template>
  <StartupRecoveryView
    v-if="startupStatus.state === 'failed'"
    :status="startupStatus"
    @status="applyStartupStatus"
  />
  <div v-else-if="startupStatus.state !== 'ready'" class="auth-gate" data-testid="startup-recovery-loading">
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
        <h1 class="auth-card__title">{{ tApp('startupRecovery.loadingTitle') }}</h1>
        <p class="auth-card__subtitle">{{ tApp('startupRecovery.loadingDescription') }}</p>
      </div>
    </div>
  </div>
  <MainLayout v-else />
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { EventsOn } from '@wailsjs/runtime/runtime'
import MainLayout from '@/core/layout/MainLayout.vue'
import StartupRecoveryView from '@/components/startup/StartupRecoveryView.vue'
import { tApp } from '@/modules/i18n/appI18n'
import { api } from '@/services/api'
import type { StartupRecoveryStatus } from '@/services/api/startupRecovery'

const startupStatus = ref<StartupRecoveryStatus>({ state: 'initializing' })
const unsubs: Array<() => void> = []

const applyStartupStatus = (next: StartupRecoveryStatus) => {
  startupStatus.value = next
}

onMounted(async () => {
  if ((window as any).runtime) {
    unsubs.push(EventsOn('startup-recovery:status', (next: StartupRecoveryStatus) => {
      applyStartupStatus(next)
    }))
  }
  try {
    applyStartupStatus(await api.startupRecoveryStatus())
  } catch (err) {
    applyStartupStatus({
      state: 'failed',
      error: {
        reason: 'unknown',
        message: err instanceof Error ? err.message : String(err || ''),
        actions: ['retry', 'open_logs'],
      },
    })
  }
})

onBeforeUnmount(() => {
  while (unsubs.length > 0) {
    const unsub = unsubs.pop()
    if (unsub) unsub()
  }
})
</script>
