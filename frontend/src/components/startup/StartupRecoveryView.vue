<template>
  <main class="startup-recovery" data-testid="startup-recovery">
    <div class="startup-recovery__drag" style="--wails-draggable: drag"></div>
    <section class="startup-recovery__panel">
      <div class="startup-recovery__brand">
        <img src="@/assets/svgs/logo.png" alt="FutrixData" />
      </div>

      <div class="startup-recovery__content">
        <p class="startup-recovery__eyebrow">{{ tApp('startupRecovery.eyebrow') }}</p>
        <h1>{{ tApp('startupRecovery.title') }}</h1>
        <p class="startup-recovery__subtitle">{{ reasonText }}</p>

        <dl class="startup-recovery__details">
          <div v-if="status.error?.dataPath">
            <dt>{{ tApp('startupRecovery.dataPath') }}</dt>
            <dd>{{ status.error.dataPath }}</dd>
          </div>
          <div v-if="status.error?.writerAppVersion || status.error?.minReaderAppVersion">
            <dt>{{ tApp('startupRecovery.versionInfo') }}</dt>
            <dd>{{ versionInfo }}</dd>
          </div>
          <div v-if="status.movedAside?.retentionDir || status.error?.retentionDir">
            <dt>{{ tApp('startupRecovery.retentionPath') }}</dt>
            <dd>{{ status.movedAside?.retentionDir || status.error?.retentionDir }}</dd>
          </div>
        </dl>

        <p class="startup-recovery__note">{{ tApp('startupRecovery.note') }}</p>

        <label v-if="canMoveAside" class="startup-recovery__confirm">
          <input
            v-model="confirmMove"
            data-testid="startup-recovery-confirm"
            type="checkbox"
          />
          <span>{{ tApp('startupRecovery.confirmMoveAside') }}</span>
        </label>

        <div class="startup-recovery__actions">
          <button
            v-if="hasAction('retry')"
            class="btn"
            data-testid="startup-recovery-retry"
            type="button"
            :disabled="busy"
            @click="retry"
          >
            {{ busyAction === 'retry' ? tApp('startupRecovery.retrying') : tApp('startupRecovery.action.retry') }}
          </button>
          <button
            v-if="hasAction('update_app')"
            class="btn secondary"
            type="button"
            :disabled="busy"
            @click="openUpdatePage"
          >
            {{ tApp('startupRecovery.action.updateApp') }}
          </button>
          <button
            v-if="hasAction('open_logs')"
            class="btn secondary"
            type="button"
            :disabled="busy"
            @click="openLogs"
          >
            {{ tApp('startupRecovery.action.openLogs') }}
          </button>
          <button
            v-if="canMoveAside"
            class="btn danger"
            data-testid="startup-recovery-move-aside"
            type="button"
            :disabled="busy || !confirmMove"
            @click="moveAside"
          >
            {{ busyAction === 'move' ? tApp('startupRecovery.moving') : tApp('startupRecovery.action.moveAside') }}
          </button>
        </div>

        <p v-if="errorText" class="startup-recovery__error" role="alert">{{ errorText }}</p>
      </div>
    </section>
  </main>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { tApp } from '@/modules/i18n/appI18n'
import { api } from '@/services/api'
import type { StartupRecoveryStatus, StartupRecoveryAction } from '@/services/api/startupRecovery'

const props = defineProps<{
  status: StartupRecoveryStatus
}>()

const emit = defineEmits<{
  status: [StartupRecoveryStatus]
}>()

const busyAction = ref('')
const confirmMove = ref(false)
const errorText = ref('')

const busy = computed(() => busyAction.value !== '')
const actions = computed(() => props.status.error?.actions || [])
const canMoveAside = computed(() => hasAction('move_aside_and_restart'))
const reasonText = computed(() => {
  const reason = props.status.error?.reason || 'unknown'
  const key = `startupRecovery.reason.${reason}`
  const translated = tApp(key)
  return translated === key ? tApp('startupRecovery.reason.unknown') : translated
})
const versionInfo = computed(() => {
  const writer = props.status.error?.writerAppVersion || tApp('startupRecovery.versionUnknown')
  const minimum = props.status.error?.minReaderAppVersion || tApp('startupRecovery.versionUnknown')
  return tApp('startupRecovery.versionValue', { writer, minimum })
})

function hasAction(action: StartupRecoveryAction) {
  return actions.value.includes(action)
}

async function retry() {
  busyAction.value = 'retry'
  errorText.value = ''
  try {
    emit('status', await api.startupRecoveryRetry())
  } catch (err) {
    errorText.value = tApp('startupRecovery.actionFailed')
  } finally {
    busyAction.value = ''
  }
}

async function openLogs() {
  errorText.value = ''
  try {
    await api.startupRecoveryOpenLogs()
  } catch (err) {
    errorText.value = tApp('startupRecovery.actionFailed')
  }
}

async function openUpdatePage() {
  errorText.value = ''
  try {
    await api.startupRecoveryOpenUpdatePage()
  } catch (err) {
    errorText.value = tApp('startupRecovery.actionFailed')
  }
}

async function moveAside() {
  if (!confirmMove.value) return
  busyAction.value = 'move'
  errorText.value = ''
  try {
    emit('status', await api.startupRecoveryMoveAsideAndRestart(true))
  } catch (err) {
    errorText.value = tApp('startupRecovery.actionFailed')
  } finally {
    busyAction.value = ''
  }
}
</script>

<style scoped>
.startup-recovery {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 32px;
  background: #f6f1e6;
  color: #1f2933;
}

.startup-recovery__drag {
  position: fixed;
  inset: 0 0 auto;
  height: 42px;
}

.startup-recovery__panel {
  width: min(760px, 100%);
  display: grid;
  grid-template-columns: 120px minmax(0, 1fr);
  gap: 28px;
  align-items: start;
  padding: 32px;
  border: 1px solid rgba(31, 41, 51, 0.14);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.78);
}

.startup-recovery__brand img {
  width: 72px;
  height: 72px;
  object-fit: contain;
}

.startup-recovery__content {
  min-width: 0;
}

.startup-recovery__eyebrow {
  margin: 0 0 8px;
  color: #56616f;
  font-size: 0.82rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0;
}

.startup-recovery h1 {
  margin: 0;
  font-size: 1.75rem;
  line-height: 1.2;
}

.startup-recovery__subtitle,
.startup-recovery__note,
.startup-recovery__error {
  margin: 12px 0 0;
  line-height: 1.55;
}

.startup-recovery__subtitle,
.startup-recovery__note {
  color: #4b5563;
}

.startup-recovery__details {
  margin: 20px 0 0;
  display: grid;
  gap: 10px;
}

.startup-recovery__details div {
  display: grid;
  gap: 4px;
}

.startup-recovery__details dt {
  color: #56616f;
  font-size: 0.78rem;
  font-weight: 700;
}

.startup-recovery__details dd {
  margin: 0;
  overflow-wrap: anywhere;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.84rem;
}

.startup-recovery__confirm {
  margin-top: 18px;
  display: flex;
  align-items: flex-start;
  gap: 10px;
  color: #374151;
  line-height: 1.45;
}

.startup-recovery__confirm input {
  width: 16px;
  height: 16px;
  flex: 0 0 auto;
  margin-top: 2px;
}

.startup-recovery__actions {
  margin-top: 22px;
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.startup-recovery__actions .btn {
  flex: 0 0 auto;
  white-space: nowrap;
}

.startup-recovery__error {
  color: #b42318;
}

@media (max-width: 760px) {
  .startup-recovery {
    padding: 20px;
    align-items: stretch;
  }

  .startup-recovery__panel {
    grid-template-columns: 1fr;
    gap: 18px;
    padding: 24px;
  }

  .startup-recovery__brand img {
    width: 56px;
    height: 56px;
  }
}
</style>
