<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'

import { tApp } from '@/modules/i18n/appI18n'
import { api } from '@/services/api'
import type { AgentIdentity, ManualInstallInfo, MCPSnippet, SkillTemplate } from '@/services/api/skill'
import { useAppStore } from '@/stores/app'

type Stage = 'name' | 'snippet'

const emit = defineEmits<{
  close: []
  created: [identity: AgentIdentity]
}>()

const store = useAppStore()

const stage = ref<Stage>('name')
const name = ref('')
const grantSensitivity = ref(false)
const grantDatasourceManagement = ref(false)
const submitBusy = ref(false)
const submitError = ref('')
const nameInput = ref<HTMLInputElement | null>(null)

const createdIdentity = ref<AgentIdentity | null>(null)
// Holds the user-facing error when CreateManualAgent succeeded but the
// post-create grant write failed. Stage 2 surfaces it as a banner so the
// user does not assume a selected grant landed silently.
const grantError = ref('')
const info = ref<ManualInstallInfo | null>(null)
const infoLoading = ref(false)
const infoError = ref('')
const activeSkillId = ref('claude')
const copiedKey = ref('')
let copyResetTimer: ReturnType<typeof setTimeout> | null = null

onMounted(async () => {
  await nextTick()
  nameInput.value?.focus()
})

onBeforeUnmount(() => {
  if (copyResetTimer) clearTimeout(copyResetTimer)
})

const skillTemplates = computed<SkillTemplate[]>(() => info.value?.skillTemplates ?? [])
const mcpSnippets = computed<MCPSnippet[]>(() => info.value?.mcpSnippets ?? [])
const activeSkill = computed<SkillTemplate | null>(() => {
  const list = skillTemplates.value
  return list.find((t) => t.id === activeSkillId.value) ?? list[0] ?? null
})

const snippetLabel = (snippet: MCPSnippet): string => {
  const key = `skill.manualInstall.snippets.${snippet.id}.label`
  const translated = tApp(key)
  return translated === key ? snippet.label : translated
}

const snippetNotes = (snippet: MCPSnippet): string => {
  if (!snippet.notes) return ''
  const key = `skill.manualInstall.snippets.${snippet.id}.notes`
  const translated = tApp(key)
  return translated === key ? snippet.notes : translated
}

const onSubmit = async () => {
  if (submitBusy.value || stage.value !== 'name') return
  submitError.value = ''
  grantError.value = ''
  submitBusy.value = true

  let identity: AgentIdentity | null = null
  try {
    const trimmed = name.value.trim()
    identity = await api.createManualAgent(trimmed)
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err || '')
    submitError.value = tApp('skill.newManual.errorPrefix', { message })
  }
  if (!identity) {
    submitBusy.value = false
    return
  }

  // Grant write is a separate try/catch so a failure here does NOT pretend
  // the agent was granted. The agent is already minted, so we still advance
  // to stage 2 (the user needs the snippets) but the snippet stage shows
  // a visible "agent created, but grant did not apply" banner.
  const appendGrantError = (message: string) => {
    grantError.value = grantError.value ? `${grantError.value} ${message}` : message
  }
  if (grantSensitivity.value) {
    try {
      identity = await api.setAgentSensitivityGrant(identity.accessKey, true)
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err || '')
      appendGrantError(tApp('skill.newManual.sensitivityGrantErrorPrefix', { message }))
    }
  }
  if (grantDatasourceManagement.value) {
    try {
      identity = await api.setAgentDatasourceManagementGrant(identity.accessKey, true)
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err || '')
      appendGrantError(tApp('skill.newManual.datasourceGrantErrorPrefix', { message }))
    }
  }

  submitBusy.value = false

  createdIdentity.value = identity
  emit('created', identity)
  stage.value = 'snippet'

  infoLoading.value = true
  infoError.value = ''
  try {
    info.value = await api.getManualInstallInfoForKey(identity.accessKey)
    if (info.value.skillTemplates.length > 0) {
      activeSkillId.value = info.value.skillTemplates[0].id
    }
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err || '')
    infoError.value = tApp('skill.newManual.infoErrorPrefix', { message })
  } finally {
    infoLoading.value = false
  }
}

const close = () => {
  // Block close paths while the create call is in flight: a stale dismissal
  // here would orphan the minted identity (parent refresh runs before the
  // accessKey lands, snippets never render).
  if (submitBusy.value) return
  emit('close')
}

const onBackdrop = (e: MouseEvent) => {
  if (e.target !== e.currentTarget) return
  close()
}

const copy = async (key: string, text: string) => {
  try {
    await navigator.clipboard.writeText(text)
    copiedKey.value = key
    if (copyResetTimer) clearTimeout(copyResetTimer)
    copyResetTimer = setTimeout(() => {
      if (copiedKey.value === key) copiedKey.value = ''
    }, 1500)
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err || '')
    store.setNotice(message, 'error')
  }
}

const onKeydown = (e: KeyboardEvent) => {
  if (e.key === 'Escape') close()
}
</script>

<template>
  <div
    class="dialog-backdrop"
    role="dialog"
    aria-modal="true"
    data-testid="new-manual-agent-dialog"
    tabindex="-1"
    @click="onBackdrop"
    @keydown="onKeydown"
  >
    <div class="dialog-card new-manual-dialog">
      <div class="dialog-head">
        <div class="dialog-head-main">
          <div class="dialog-icon">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
          </div>
          <div>
            <h4>{{ tApp('skill.newManual.dialogTitle') }}</h4>
            <div class="meta">
              <span v-if="stage === 'name'">{{ tApp('skill.newManual.stage1Desc') }}</span>
              <span v-else>{{ tApp('skill.newManual.stage2Desc', { name: createdIdentity?.name ?? '' }) }}</span>
            </div>
          </div>
        </div>
        <button
          class="dialog-close"
          type="button"
          :aria-label="tApp('skill.manualInstall.close')"
          :disabled="submitBusy"
          data-testid="new-manual-agent-close"
          @click="close"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
        </button>
      </div>

      <form
        v-if="stage === 'name'"
        class="new-manual-form"
        data-testid="new-manual-agent-form"
        @submit.prevent="onSubmit"
      >
        <label class="new-manual-form__label" for="new-manual-agent-name">
          {{ tApp('skill.newManual.nameLabel') }}
        </label>
        <input
          id="new-manual-agent-name"
          ref="nameInput"
          v-model="name"
          class="new-manual-form__input"
          type="text"
          :placeholder="tApp('skill.newManual.namePlaceholder')"
          :disabled="submitBusy"
          data-testid="new-manual-agent-name-input"
          autocapitalize="off"
          autocorrect="off"
          spellcheck="false"
        />
        <div class="new-manual-approval-policy" data-testid="new-manual-agent-approval-policy">
          {{ tApp('skill.agentApprovalPolicyNotice') }}
        </div>
        <label class="new-manual-form__grant" for="new-manual-agent-grant">
          <input
            id="new-manual-agent-grant"
            v-model="grantSensitivity"
            type="checkbox"
            :disabled="submitBusy"
            data-testid="new-manual-agent-grant-input"
          />
          <span class="new-manual-form__grant-label">{{ tApp('skill.newManual.sensitivityGrantLabel') }}</span>
        </label>
        <p class="new-manual-form__grant-hint">{{ tApp('skill.newManual.sensitivityGrantHint') }}</p>
        <label class="new-manual-form__grant" for="new-manual-agent-datasource-grant">
          <input
            id="new-manual-agent-datasource-grant"
            v-model="grantDatasourceManagement"
            type="checkbox"
            :disabled="submitBusy"
            data-testid="new-manual-agent-datasource-grant-input"
          />
          <span class="new-manual-form__grant-label">{{ tApp('skill.newManual.datasourceGrantLabel') }}</span>
        </label>
        <p class="new-manual-form__grant-hint">{{ tApp('skill.newManual.datasourceGrantHint') }}</p>
        <div v-if="submitError" class="new-manual-form__error" data-testid="new-manual-agent-error">
          {{ submitError }}
        </div>
        <div class="dialog-actions">
          <button
            class="btn ghost"
            type="button"
            :disabled="submitBusy"
            data-testid="new-manual-agent-cancel"
            @click="close"
          >
            {{ tApp('skill.newManual.cancel') }}
          </button>
          <button
            class="btn primary"
            type="submit"
            :disabled="submitBusy"
            data-testid="new-manual-agent-submit"
          >
            {{ submitBusy ? tApp('skill.newManual.creating') : tApp('skill.newManual.create') }}
          </button>
        </div>
      </form>

      <template v-else>
        <div
          v-if="grantError"
          class="new-manual-grant-warning"
          data-testid="new-manual-agent-grant-error"
          role="alert"
        >
          {{ grantError }}
        </div>
        <div v-if="createdIdentity" class="new-manual-summary" data-testid="new-manual-agent-summary">
          <div class="new-manual-summary__row">
            <span class="new-manual-summary__label">{{ tApp('skill.manage.agentNameLabel') }}</span>
            <span class="new-manual-summary__value">{{ createdIdentity.name }}</span>
          </div>
          <div class="new-manual-summary__row">
            <span class="new-manual-summary__label">{{ tApp('skill.manage.accessKeyLabel') }}</span>
            <code class="new-manual-summary__key" data-testid="new-manual-agent-key">{{ createdIdentity.accessKey }}</code>
            <button
              class="manual-copy-btn mini"
              type="button"
              data-testid="new-manual-agent-copy-key"
              @click="copy('access-key', createdIdentity!.accessKey)"
            >
              {{ copiedKey === 'access-key' ? tApp('skill.manualInstall.copied') : tApp('skill.manage.copyKey') }}
            </button>
          </div>
        </div>

        <div v-if="infoLoading" class="manual-loading">{{ tApp('skill.manualInstall.loading') }}</div>

        <div
          v-else-if="infoError"
          class="new-manual-form__error"
          data-testid="new-manual-agent-info-error"
        >
          {{ infoError }}
        </div>

        <template v-else-if="info">
          <div class="new-manual-approval-policy" data-testid="new-manual-agent-snippet-approval-policy">
            {{ tApp('skill.agentApprovalPolicyNotice') }}
          </div>

          <div v-if="info.cliBinaryPath" class="manual-cli-path">
            <span class="manual-cli-path__label">{{ tApp('skill.manualInstall.cliPathLabel') }}</span>
            <code class="manual-cli-path__value">{{ info.cliBinaryPath }}</code>
            <button
              class="manual-copy-btn mini"
              type="button"
              data-testid="new-manual-agent-copy-cli"
              @click="copy('cli-path', info.cliBinaryPath)"
            >
              {{ copiedKey === 'cli-path' ? tApp('skill.manualInstall.copied') : tApp('skill.manualInstall.copy') }}
            </button>
          </div>

          <section class="manual-section" data-testid="new-manual-agent-skill-section">
            <header class="manual-section__head">
              <h5 class="manual-section__title">{{ tApp('skill.manualInstall.skillHeading') }}</h5>
              <p class="manual-section__desc">{{ tApp('skill.manualInstall.skillDesc') }}</p>
            </header>

            <div class="manual-skill-tabs" role="tablist">
              <button
                v-for="tpl in skillTemplates"
                :key="tpl.id"
                type="button"
                class="manual-skill-tab"
                :class="{ 'manual-skill-tab--active': activeSkillId === tpl.id }"
                role="tab"
                :aria-selected="activeSkillId === tpl.id"
                @click="activeSkillId = tpl.id"
              >
                {{ tpl.name }}
              </button>
            </div>

            <div v-if="activeSkill" class="manual-snippet">
              <div class="manual-snippet__head">
                <div class="manual-snippet__path">
                  <span class="manual-snippet__path-label">{{ tApp('skill.manualInstall.suggestedPath') }}</span>
                  <code>{{ activeSkill.suggestedPath }}</code>
                </div>
                <button
                  class="manual-copy-btn"
                  type="button"
                  :data-testid="`new-manual-copy-skill-${activeSkill.id}`"
                  @click="copy('skill-' + activeSkill.id, activeSkill.content)"
                >
                  {{ copiedKey === 'skill-' + activeSkill.id ? tApp('skill.manualInstall.copied') : tApp('skill.manualInstall.copyContent') }}
                </button>
              </div>
              <pre class="manual-snippet__code"><code>{{ activeSkill.content }}</code></pre>
            </div>
          </section>

          <section class="manual-section" data-testid="new-manual-agent-mcp-section">
            <header class="manual-section__head">
              <h5 class="manual-section__title">{{ tApp('skill.manualInstall.mcpHeading') }}</h5>
              <p class="manual-section__desc">{{ tApp('skill.manualInstall.mcpDesc') }}</p>
            </header>

            <div
              v-for="snippet in mcpSnippets"
              :key="snippet.id"
              class="manual-snippet"
              :data-testid="`new-manual-mcp-${snippet.id}`"
            >
              <div class="manual-snippet__head">
                <div class="manual-snippet__title-wrap">
                  <span class="manual-snippet__title">{{ snippetLabel(snippet) }}</span>
                  <span class="manual-snippet__format-badge">{{ snippet.format.toUpperCase() }}</span>
                </div>
                <button
                  class="manual-copy-btn"
                  type="button"
                  :data-testid="`new-manual-copy-mcp-${snippet.id}`"
                  @click="copy('mcp-' + snippet.id, snippet.content)"
                >
                  {{ copiedKey === 'mcp-' + snippet.id ? tApp('skill.manualInstall.copied') : tApp('skill.manualInstall.copy') }}
                </button>
              </div>
              <div class="manual-snippet__path">
                <span class="manual-snippet__path-label">{{ tApp('skill.manualInstall.suggestedPath') }}</span>
                <code>{{ snippet.suggestedPath }}</code>
              </div>
              <p v-if="snippet.notes" class="manual-snippet__notes">{{ snippetNotes(snippet) }}</p>
              <pre class="manual-snippet__code"><code>{{ snippet.content }}</code></pre>
            </div>
          </section>
        </template>

        <div class="dialog-actions">
          <button
            class="btn primary"
            type="button"
            data-testid="new-manual-agent-done"
            @click="close"
          >
            {{ tApp('skill.newManual.done') }}
          </button>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.new-manual-dialog {
  width: min(720px, 94vw);
  max-height: 88vh;
  display: flex;
  flex-direction: column;
  gap: 16px;
  overflow-y: auto;
}

.dialog-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.dialog-close {
  background: transparent;
  border: none;
  cursor: pointer;
  color: var(--soft-ink);
  padding: 4px;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
}

.dialog-close:hover {
  background: color-mix(in oklab, var(--ink) 8%, transparent);
  color: var(--ink);
}

.new-manual-form {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.new-manual-form__label {
  font-size: 12px;
  font-weight: 700;
  color: var(--ink);
}

.new-manual-form__input {
  min-height: 40px;
  padding: 0 12px;
  border-radius: 8px;
  border: 1px solid var(--edge);
  background: var(--panel);
  font: inherit;
  color: var(--ink);
}

.new-manual-form__input:focus {
  outline: none;
  border-color: color-mix(in oklab, var(--primary) 45%, var(--edge));
}

.new-manual-approval-policy {
  padding: 9px 11px;
  border-radius: 8px;
  border: 1px solid color-mix(in oklab, var(--warn, #f59e0b) 28%, var(--edge));
  background: color-mix(in oklab, var(--warn, #f59e0b) 9%, var(--panel));
  color: var(--ink);
  font-size: 12px;
  line-height: 1.5;
}

.new-manual-form__error {
  color: var(--danger, #c0392b);
  font-size: 12px;
}

.new-manual-form__grant {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  font-size: 13px;
  color: var(--ink);
  cursor: pointer;
}

.new-manual-form__grant-label {
  font-weight: 600;
}

.new-manual-form__grant-hint {
  margin: -4px 0 0;
  font-size: 12px;
  color: var(--soft-ink);
  line-height: 1.5;
}

.new-manual-grant-warning {
  padding: 10px 12px;
  border-radius: 10px;
  background: color-mix(in oklab, var(--danger, #b91c1c) 8%, var(--panel));
  border: 1px solid color-mix(in oklab, var(--danger, #b91c1c) 22%, var(--edge));
  color: var(--danger, #b91c1c);
  font-size: 12px;
  line-height: 1.5;
}

.new-manual-summary {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  border-radius: 10px;
  background: color-mix(in oklab, var(--primary) 6%, var(--panel));
  border: 1px solid color-mix(in oklab, var(--primary) 18%, transparent);
}

.new-manual-summary__row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.new-manual-summary__label {
  font-size: 11px;
  font-weight: 600;
  color: var(--soft-ink);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  flex: 0 0 auto;
}

.new-manual-summary__value {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink);
}

.new-manual-summary__key {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  color: var(--ink);
  word-break: break-all;
  flex: 1 1 auto;
}

.manual-loading {
  padding: 24px 0;
  text-align: center;
  color: var(--soft-ink);
  font-size: 13px;
}

.manual-cli-path {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  border-radius: 10px;
  background: color-mix(in oklab, var(--primary) 6%, var(--panel));
  border: 1px solid color-mix(in oklab, var(--primary) 18%, transparent);
  flex-wrap: wrap;
}

.manual-cli-path__label {
  font-size: 11px;
  font-weight: 600;
  color: var(--soft-ink);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.manual-cli-path__value {
  flex: 1;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  color: var(--ink);
  word-break: break-all;
}

.manual-section {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.manual-section__head {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.manual-section__title {
  margin: 0;
  font-size: 14px;
  font-weight: 700;
  color: var(--ink);
}

.manual-section__desc {
  margin: 0;
  font-size: 12px;
  color: var(--soft-ink);
}

.manual-skill-tabs {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}

.manual-skill-tab {
  padding: 5px 12px;
  border-radius: 8px;
  border: 1px solid var(--edge);
  background: var(--panel);
  cursor: pointer;
  font: inherit;
  font-size: 12px;
  color: var(--soft-ink);
  transition: background 0.15s, border-color 0.15s, color 0.15s;
}

.manual-skill-tab:hover {
  border-color: color-mix(in oklab, var(--primary) 25%, var(--edge));
  color: var(--ink);
}

.manual-skill-tab--active {
  background: color-mix(in oklab, var(--primary) 10%, var(--panel));
  border-color: color-mix(in oklab, var(--primary) 30%, var(--edge));
  color: var(--primary);
  font-weight: 600;
}

.manual-snippet {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  border-radius: 10px;
  border: 1px solid var(--edge);
  background: var(--panel);
}

.manual-snippet__head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.manual-snippet__title-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
}

.manual-snippet__title {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink);
}

.manual-snippet__format-badge {
  font-size: 10px;
  font-weight: 600;
  padding: 1px 6px;
  border-radius: 4px;
  background: color-mix(in oklab, var(--primary) 10%, var(--panel));
  color: var(--primary);
  letter-spacing: 0.04em;
}

.manual-snippet__path {
  display: flex;
  align-items: baseline;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 11px;
}

.manual-snippet__path-label {
  color: var(--soft-ink);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.manual-snippet__path code {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 11px;
  color: var(--ink);
  word-break: break-all;
}

.manual-snippet__notes {
  margin: 0;
  font-size: 11px;
  color: var(--soft-ink);
  line-height: 1.5;
}

.manual-snippet__code {
  margin: 0;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--surface);
  border: 1px solid var(--edge);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  line-height: 1.5;
  color: var(--ink);
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 260px;
  overflow-y: auto;
}

.manual-copy-btn {
  padding: 4px 12px;
  border-radius: 7px;
  border: 1px solid color-mix(in oklab, var(--primary) 30%, var(--edge));
  background: color-mix(in oklab, var(--primary) 8%, var(--panel));
  color: var(--primary);
  font: inherit;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.15s, border-color 0.15s;
  white-space: nowrap;
}

.manual-copy-btn:hover {
  background: color-mix(in oklab, var(--primary) 16%, var(--panel));
}

.manual-copy-btn.mini {
  padding: 3px 8px;
  font-size: 11px;
}

.dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: auto;
}
</style>
