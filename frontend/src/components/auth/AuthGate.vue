<template>
  <section class="auth-gate" data-testid="auth-gate">
    <div class="auth-drag-region" style="--wails-draggable: drag"></div>
    <div class="auth-card">
      <!-- Logo -->
      <div class="auth-card__logo">
        <img src="@/assets/svgs/logo.png" alt="FutrixData" />
      </div>

      <!-- Stage: idle — waiting to start login -->
      <template v-if="stage === 'idle'">
        <div class="auth-card__header">
          <h1 class="auth-card__title">{{ tApp('auth.login.title') }}</h1>
          <p class="auth-card__subtitle">{{ tApp('auth.login.description') }}</p>
        </div>

        <div class="auth-card__body">
          <button
            class="auth-card__btn auth-card__btn--primary"
            data-testid="auth-login-button"
            type="button"
            :disabled="authStore.loginBusy"
            @click="onStartLogin"
          >
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>
            {{ tApp('auth.login.start') }}
          </button>

        </div>
      </template>

      <!-- Stage: waiting — browser opened, polling for completion -->
      <template v-else-if="stage === 'waiting'">
        <button
          class="auth-card__back"
          type="button"
          @click="onCancel"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg>
          {{ tApp('auth.login.back') }}
        </button>

        <div class="auth-card__header">
          <div class="auth-card__spinner-ring">
            <svg class="auth-card__spinner" viewBox="0 0 50 50">
              <circle cx="25" cy="25" r="20" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-dasharray="90 150" stroke-dashoffset="0" />
            </svg>
          </div>
          <h1 class="auth-card__title">{{ tApp('auth.login.waitingTitle') }}</h1>
          <p class="auth-card__subtitle">{{ tApp('auth.login.waitingDescription') }}</p>
        </div>

        <div class="auth-card__body">
          <!-- Login URL -->
          <div v-if="authStore.loginUrl" class="auth-card__url-box">
            <span class="auth-card__url-label">{{ tApp('auth.login.urlLabel') }}</span>
            <a href="#" class="auth-card__url-link" @click.prevent="openLoginUrl">{{ authStore.loginUrl }}</a>
          </div>

          <div class="auth-card__divider">
            <span>{{ tApp('auth.login.manualHint') }}</span>
          </div>

          <!-- Manual code input -->
          <div class="auth-card__code-input">
            <input
              id="auth-manual-code"
              v-model="authStore.manualCode"
              data-testid="auth-manual-code"
              type="text"
              autocapitalize="off"
              autocorrect="off"
              spellcheck="false"
              :placeholder="tApp('auth.login.codePlaceholder')"
              @keydown.enter="manualCodeReady && onSubmitManualCode()"
            >
            <button
              class="auth-card__btn auth-card__btn--secondary"
              data-testid="auth-manual-submit"
              type="button"
              :disabled="authStore.loginBusy || !manualCodeReady"
              @click="onSubmitManualCode"
            >
              {{ tApp('auth.login.submitCode') }}
            </button>
          </div>

        </div>
      </template>

      <!-- Error message (shown in any stage) -->
      <div v-if="authStore.error" class="auth-card__error" data-testid="auth-login-error">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>
        {{ authStore.error }}
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { BrowserOpenURL } from '@wailsjs/runtime/runtime'
import { tApp } from '@/modules/i18n/appI18n'
import { useAuthStore } from '@/stores/auth'

const authStore = useAuthStore()

const stage = computed(() => {
  if (authStore.state.pendingLogin) return 'waiting'
  return 'idle'
})

const manualCodeReady = computed(() => String(authStore.manualCode || '').trim().length >= 6)

const openLoginUrl = () => {
  if (authStore.loginUrl) {
    BrowserOpenURL(authStore.loginUrl)
  }
}

const onStartLogin = async () => {
  await authStore.startLogin()
}

const onSubmitManualCode = async () => {
  await authStore.completeManualCode(authStore.manualCode)
}

const onCancel = () => {
  authStore.stopPolling()
  authStore.state.pendingLogin = null
  authStore.error = ''
  authStore.loginUrl = ''
  authStore.manualCode = ''
}
</script>
