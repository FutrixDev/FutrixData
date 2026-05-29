<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import { tApp } from '@/modules/i18n/appI18n'
import { api } from '@/services/api'
import type { ManualInstallInfo, SkillTemplate, MCPSnippet } from '@/services/api/skill'
import { useAppStore } from '@/stores/app'

const props = defineProps<{ initialSection?: 'skill' | 'mcp'; accessKey?: string }>()
const emit = defineEmits<{ close: [] }>()
const store = useAppStore()

const info = ref<ManualInstallInfo | null>(null)
const loading = ref(true)
const activeSkillId = ref<string>('claude')
const copiedKey = ref<string>('')
const agentName = ref('')
const savingAgentName = ref(false)

onMounted(async () => {
  try {
    info.value = props.accessKey
      ? await api.getManualInstallInfoForKey(props.accessKey)
      : await api.getManualInstallInfo()
    agentName.value = info.value.agentName || ''
    if (info.value.skillTemplates.length > 0) {
      activeSkillId.value = info.value.skillTemplates[0].id
    }
  } finally {
    loading.value = false
  }
})

const activeSkill = computed<SkillTemplate | null>(() => {
  if (!info.value) return null
  return info.value.skillTemplates.find((t: SkillTemplate) => t.id === activeSkillId.value) ?? info.value.skillTemplates[0] ?? null
})

const mcpSnippets = computed<MCPSnippet[]>(() => info.value?.mcpSnippets ?? [])

const snippetLabel = (snippet: MCPSnippet): string => {
  const key = `skill.manualInstall.snippets.${snippet.id}.label`
  const translated = tApp(key)
  // tApp returns the key itself when the translation is missing.
  return translated === key ? snippet.label : translated
}

const snippetNotes = (snippet: MCPSnippet): string => {
  if (!snippet.notes) return ''
  const key = `skill.manualInstall.snippets.${snippet.id}.notes`
  const translated = tApp(key)
  return translated === key ? snippet.notes : translated
}

const onClose = () => emit('close')

const persistAgentName = async () => {
  if (!info.value?.accessKey) return
  const nextName = agentName.value.trim()
  if (!nextName || nextName === info.value.agentName) return
  savingAgentName.value = true
  try {
    const updated = await api.renameAgentIdentity(info.value.accessKey, nextName)
    agentName.value = updated.name
    info.value = { ...info.value, agentName: updated.name }
  } catch (err) {
    store.setNotice(err instanceof Error ? err.message : String(err), 'error')
  } finally {
    savingAgentName.value = false
  }
}

const copy = async (key: string, text: string) => {
  await persistAgentName()
  try {
    await navigator.clipboard.writeText(text)
    copiedKey.value = key
    window.setTimeout(() => {
      if (copiedKey.value === key) copiedKey.value = ''
    }, 1500)
  } catch {
    copiedKey.value = ''
  }
}

const onBackdrop = (e: MouseEvent) => {
  if (e.target === e.currentTarget) onClose()
}
</script>

<template>
  <div class="dialog-backdrop" role="dialog" aria-modal="true" data-testid="manual-install-dialog" @click="onBackdrop">
    <div class="dialog-card manual-dialog">
      <div class="dialog-head">
        <div class="dialog-head-main">
          <div class="dialog-icon">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>
          </div>
          <div>
            <h4>{{ tApp('skill.manualInstall.title') }}</h4>
            <div class="meta">
              <span>{{ tApp('skill.manualInstall.subtitle') }}</span>
            </div>
          </div>
        </div>
        <button
          class="dialog-close"
          type="button"
          :aria-label="tApp('skill.manualInstall.close')"
          data-testid="manual-install-close"
          @click="onClose"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
        </button>
      </div>

      <div v-if="loading" class="manual-loading">{{ tApp('skill.manualInstall.loading') }}</div>

      <template v-else-if="info">
        <div class="manual-approval-policy" data-testid="manual-install-approval-policy">
          {{ tApp('skill.agentApprovalPolicyNotice') }}
        </div>

        <div class="manual-agent-card">
          <label class="manual-agent-card__label" for="manual-agent-name">{{ tApp('skill.manualInstall.agentNameLabel') }}</label>
          <div class="manual-agent-card__row">
            <input
              id="manual-agent-name"
              name="manual-agent-name"
              v-model="agentName"
              class="manual-agent-card__input"
              type="text"
              :placeholder="tApp('skill.manualInstall.agentNamePlaceholder')"
              data-testid="manual-agent-name-input"
              autocapitalize="off"
              autocorrect="off"
              spellcheck="false"
              @blur="persistAgentName"
              @keydown.enter.prevent="persistAgentName"
            />
            <span class="manual-agent-card__status">
              {{ savingAgentName ? tApp('skill.manualInstall.agentNameSaving') : tApp('skill.manualInstall.boundAccessKey') }}
            </span>
          </div>
        </div>

        <div v-if="info.cliBinaryPath" class="manual-cli-path">
          <span class="manual-cli-path__label">{{ tApp('skill.manualInstall.cliPathLabel') }}</span>
          <code class="manual-cli-path__value">{{ info.cliBinaryPath }}</code>
          <button
            class="manual-copy-btn mini"
            type="button"
            @click="copy('cli-path', info.cliBinaryPath)"
          >
            {{ copiedKey === 'cli-path' ? tApp('skill.manualInstall.copied') : tApp('skill.manualInstall.copy') }}
          </button>
        </div>

        <!-- Skill section -->
        <section class="manual-section" data-testid="manual-install-skill-section">
          <header class="manual-section__head">
            <h5 class="manual-section__title">{{ tApp('skill.manualInstall.skillHeading') }}</h5>
            <p class="manual-section__desc">{{ tApp('skill.manualInstall.skillDesc') }}</p>
          </header>

          <div class="manual-skill-tabs" role="tablist">
            <button
              v-for="tpl in info.skillTemplates"
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
                :data-testid="`manual-copy-skill-${activeSkill.id}`"
                @click="copy('skill-' + activeSkill.id, activeSkill.content)"
              >
                {{ copiedKey === 'skill-' + activeSkill.id ? tApp('skill.manualInstall.copied') : tApp('skill.manualInstall.copyContent') }}
              </button>
            </div>
            <pre class="manual-snippet__code"><code>{{ activeSkill.content }}</code></pre>
          </div>
        </section>

        <!-- MCP section -->
        <section class="manual-section" data-testid="manual-install-mcp-section">
          <header class="manual-section__head">
            <h5 class="manual-section__title">{{ tApp('skill.manualInstall.mcpHeading') }}</h5>
            <p class="manual-section__desc">{{ tApp('skill.manualInstall.mcpDesc') }}</p>
          </header>

          <div
            v-for="snippet in mcpSnippets"
            :key="snippet.id"
            class="manual-snippet"
            :data-testid="`manual-install-mcp-${snippet.id}`"
          >
            <div class="manual-snippet__head">
              <div class="manual-snippet__title-wrap">
                <span class="manual-snippet__title">{{ snippetLabel(snippet) }}</span>
                <span class="manual-snippet__format-badge">{{ snippet.format.toUpperCase() }}</span>
              </div>
              <button
                class="manual-copy-btn"
                type="button"
                :data-testid="`manual-copy-mcp-${snippet.id}`"
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
        <button class="btn primary" type="button" data-testid="manual-install-done" @click="onClose">
          {{ tApp('skill.manualInstall.close') }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.manual-dialog {
  width: min(720px, 94vw);
  max-height: 88vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.manual-dialog > :not(.dialog-head):not(.dialog-actions) {
  overflow: visible;
}

.manual-dialog {
  gap: 16px;
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
}

.dialog-close:hover {
  background: color-mix(in oklab, var(--ink) 8%, transparent);
  color: var(--ink);
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

.manual-sections {
  display: flex;
  flex-direction: column;
  gap: 20px;
  overflow-y: auto;
}

.manual-dialog > .manual-section {
  overflow: visible;
}

.manual-dialog {
  overflow-y: auto;
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

.manual-agent-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.manual-approval-policy {
  padding: 9px 11px;
  border-radius: 8px;
  border: 1px solid color-mix(in oklab, var(--warn, #f59e0b) 28%, var(--edge));
  background: color-mix(in oklab, var(--warn, #f59e0b) 9%, var(--panel));
  color: var(--ink);
  font-size: 12px;
  line-height: 1.5;
}

.manual-agent-card__label {
  font-size: 12px;
  font-weight: 700;
  color: var(--ink);
}

.manual-agent-card__row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.manual-agent-card__input {
  flex: 1 1 auto;
  min-height: 40px;
}

.manual-agent-card__status {
  flex: 0 0 auto;
  white-space: nowrap;
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
