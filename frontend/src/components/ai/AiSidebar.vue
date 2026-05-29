<template>
  <aside class="ai-sidebar" :class="{ open: store.isOpen }">
    <div class="ai-sidebar-header">
      <div class="ai-title-row">
        <div class="ai-title-group">
          <span class="ai-composer-icon ai-header-icon" aria-hidden="true">
            <svg class="ai-composer-glyph" viewBox="0 0 24 24" aria-hidden="true">
              <path
                d="M12 3.5l2.6 5.5 6 .9-4.3 4.2 1 6-5.3-3-5.3 3 1-6L3.4 9.9l6-.9L12 3.5z"
                fill="none"
                stroke="currentColor"
                stroke-width="1.4"
                stroke-linejoin="round"
              />
            </svg>
          </span>
          <div class="ai-title-stack">
            <span class="ai-title">{{ tApp('ai.sidebar.title') }}</span>
          </div>
        </div>
        <button
          class="ai-icon-btn"
          type="button"
          @click="handleNewChat"
          :aria-label="tApp('ai.sidebar.newChat')"
          :title="tApp('ai.sidebar.newChat')"
        >
          <svg class="ai-icon-glyph" viewBox="0 0 24 24" aria-hidden="true">
            <path
              d="M12 5v14M5 12h14"
              fill="none"
              stroke="currentColor"
              stroke-width="1.8"
              stroke-linecap="round"
            />
          </svg>
        </button>
      </div>
      <div v-if="store.conversations.length" class="ai-history-strip">
        <div class="ai-history-scroll">
          <div
            v-for="c in store.conversations"
            :key="c.id"
            class="ai-history-tab"
            :class="{ active: store.activeId === c.id }"
          >
            <button class="ai-history-main" type="button" @click="store.setActive(c.id)">
              <span class="ai-history-title">{{ c.title }}</span>
            </button>
            <button
              class="ai-history-delete"
              type="button"
              @click="store.deleteConversation(c.id)"
              :aria-label="tApp('ai.sidebar.deleteConversation')"
            >
              ×
            </button>
          </div>
        </div>
      </div>
    </div>

    <div class="ai-sidebar-body">
      <div v-if="activeApproval" class="ai-approval-card" :class="[approvalToneClass, { 'is-executing': isApproving }]">
        <div class="ai-approval-header">
          <span class="ai-approval-icon" aria-hidden="true">
            <svg v-if="!isApproving" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
              <path d="M10 2.5a7.5 7.5 0 1 0 0 15 7.5 7.5 0 0 0 0-15Z" />
              <path d="M10 6.5v4M10 13.5h.01" />
            </svg>
            <svg v-else class="ai-approval-spinner" viewBox="0 0 20 20" aria-hidden="true">
              <circle cx="10" cy="10" r="7.5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-dasharray="36 60" />
            </svg>
          </span>
          <div class="ai-approval-title">
            {{ isApproving ? tApp('ai.sidebar.executing') : tApp('ai.sidebar.approvalRequired') }}
          </div>
        </div>
        <div class="ai-approval-summary">{{ activeApproval.summary }}</div>
        <div class="ai-approval-details">
          <template v-if="activeApproval.kind === 'execute_statement'">
            <div class="ai-approval-detail-grid">
              <div class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.datasource') }}</span>
                <span class="ai-approval-detail-value">
                  {{ String(activeApproval.payload?.datasourceName || activeApproval.payload?.datasourceId || '') }}
                </span>
              </div>
              <div v-if="activeApproval.payload?.database" class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.database') }}</span>
                <span class="ai-approval-detail-value">{{ String(activeApproval.payload?.database || '') }}</span>
              </div>
              <div v-if="activeApproval.payload?.risk?.level" class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.risk') }}</span>
                <span
                  class="ai-risk-badge"
                  :class="`ai-risk-${String(activeApproval.payload?.risk?.level || 'low')}`"
                >
                  {{ String(activeApproval.payload?.risk?.level || '').toUpperCase() }}
                </span>
              </div>
              <div v-if="activeApproval.payload?.trustLevel" class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.trustLevel') }}</span>
                <span
                  class="ai-trust-badge"
                  :class="`ai-trust-${String(activeApproval.payload?.trustLevel || 'cautious')}`"
                >
                  {{ tApp(`riskRules.trustLevels.${activeApproval.payload?.trustLevel}.label`) }}
                </span>
              </div>
              <div v-if="activeApproval.payload?.explain?.usesIndex !== undefined" class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.explain') }}</span>
                <span class="ai-approval-detail-value">
                  {{ activeApproval.payload.explain.usesIndex ? tApp('ai.sidebar.explainUsesIndex') : tApp('ai.sidebar.explainNoIndex') }}
                </span>
              </div>
              <div v-else-if="activeApproval.payload?.explainError" class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.explain') }}</span>
                <span class="ai-approval-detail-value">{{ String(activeApproval.payload?.explainError || '') }}</span>
              </div>
            </div>

            <div v-if="activeApproval.payload?.risk?.reasons?.length" class="ai-approval-risk-notes">
              {{
                Array.isArray(activeApproval.payload?.risk?.reasons)
                  ? activeApproval.payload.risk.reasons.join('; ')
                  : String(activeApproval.payload?.risk?.reasons || '')
              }}
            </div>

            <div v-if="executeGateReason" class="ai-approval-gate-note">
              {{ executeGateReason }}
            </div>

            <div v-if="activeApproval.payload?.statement" class="ai-approval-code">
              <div class="ai-approval-code-label">{{ tApp('ai.sidebar.label.statement') }}</div>
              <pre><code>{{ String(activeApproval.payload?.statement || '') }}</code></pre>
            </div>
          </template>

          <template v-else-if="activeApproval.kind === 'analyze_result'">
            <div class="ai-approval-detail-grid">
              <div class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.datasource') }}</span>
                <span class="ai-approval-detail-value">{{ String(activeApproval.payload?.datasourceId || '') }}</span>
              </div>
              <div v-if="activeApproval.payload?.datasourceType" class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.type') }}</span>
                <span class="ai-approval-detail-value">{{ String(activeApproval.payload?.datasourceType || '') }}</span>
              </div>
              <div v-if="activeApproval.payload?.database" class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.database') }}</span>
                <span class="ai-approval-detail-value">{{ String(activeApproval.payload?.database || '') }}</span>
              </div>
              <div class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.rows') }}</span>
                <span class="ai-approval-detail-value">
                  {{ String(activeApproval.payload?.payloadRows ?? '') }}
                  <template v-if="activeApproval.payload?.rowCount !== undefined">
                    / {{ String(activeApproval.payload?.rowCount ?? '') }}
                  </template>
                </span>
              </div>
              <div v-if="activeApproval.payload?.approxBytes !== undefined" class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.size') }}</span>
                <span class="ai-approval-detail-value">{{ tApp('ai.sidebar.bytes', { count: String(activeApproval.payload?.approxBytes ?? '') }) }}</span>
              </div>
              <div v-if="activeApproval.payload?.capturedAt" class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.captured') }}</span>
                <span class="ai-approval-detail-value">{{ String(activeApproval.payload?.capturedAt || '') }}</span>
              </div>
              <div v-if="activeApproval.payload?.truncated !== undefined" class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.truncated') }}</span>
                <span class="ai-approval-detail-value">{{ activeApproval.payload?.truncated ? tApp('ai.sidebar.yes') : tApp('ai.sidebar.no') }}</span>
              </div>
            </div>
            <div class="ai-approval-risk-notes">
              {{ tApp('ai.sidebar.analyzeRiskNote') }}
            </div>
          </template>

          <template v-else-if="activeApproval.kind === 'create_visualization'">
            <div class="ai-approval-detail-grid">
              <div class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.datasource') }}</span>
                <span class="ai-approval-detail-value">{{ String(activeApproval.payload?.datasourceId || '') }}</span>
              </div>
              <div v-if="activeApproval.payload?.datasourceType" class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.type') }}</span>
                <span class="ai-approval-detail-value">{{ String(activeApproval.payload?.datasourceType || '') }}</span>
              </div>
              <div v-if="activeApproval.payload?.database" class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.database') }}</span>
                <span class="ai-approval-detail-value">{{ String(activeApproval.payload?.database || '') }}</span>
              </div>
              <div class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.rows') }}</span>
                <span class="ai-approval-detail-value">
                  {{ String(activeApproval.payload?.payloadRows ?? '') }}
                  <template v-if="activeApproval.payload?.rowCount !== undefined">
                    / {{ String(activeApproval.payload?.rowCount ?? '') }}
                  </template>
                </span>
              </div>
              <div v-if="activeApproval.payload?.approxBytes !== undefined" class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.size') }}</span>
                <span class="ai-approval-detail-value">{{ tApp('ai.sidebar.bytes', { count: String(activeApproval.payload?.approxBytes ?? '') }) }}</span>
              </div>
              <div v-if="activeApproval.payload?.capturedAt" class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.captured') }}</span>
                <span class="ai-approval-detail-value">{{ String(activeApproval.payload?.capturedAt || '') }}</span>
              </div>
              <div v-if="activeApproval.payload?.truncated !== undefined" class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.truncated') }}</span>
                <span class="ai-approval-detail-value">{{ activeApproval.payload?.truncated ? tApp('ai.sidebar.yes') : tApp('ai.sidebar.no') }}</span>
              </div>
            </div>
            <div class="ai-approval-risk-notes">
              {{ tApp('ai.sidebar.visualizationRiskNote') }}
            </div>
          </template>

          <template v-else-if="activeApproval.kind === 'create_datasource'">
            <div class="ai-approval-detail-grid">
              <div class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.name') }}</span>
                <span class="ai-approval-detail-value">{{ String(activeApproval.payload?.name || '') }}</span>
              </div>
              <div class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.type') }}</span>
                <span class="ai-approval-detail-value">{{ String(activeApproval.payload?.type || '') }}</span>
              </div>
              <div v-if="activeApproval.payload?.host" class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.host') }}</span>
                <span class="ai-approval-detail-value">{{ String(activeApproval.payload?.host || '') }}</span>
              </div>
              <div v-if="activeApproval.payload?.port" class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.port') }}</span>
                <span class="ai-approval-detail-value">{{ String(activeApproval.payload?.port || '') }}</span>
              </div>
              <div v-if="activeApproval.payload?.database" class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.database') }}</span>
                <span class="ai-approval-detail-value">{{ String(activeApproval.payload?.database || '') }}</span>
              </div>
              <div v-if="activeApproval.payload?.username" class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.username') }}</span>
                <span class="ai-approval-detail-value">{{ String(activeApproval.payload?.username || '') }}</span>
              </div>
            </div>
          </template>

          <template v-else-if="activeApproval.kind === 'delete_datasource'">
            <div class="ai-approval-detail-grid">
              <div v-if="activeApproval.payload?.name" class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.name') }}</span>
                <span class="ai-approval-detail-value">{{ String(activeApproval.payload?.name || '') }}</span>
              </div>
              <div v-if="activeApproval.payload?.datasourceId" class="ai-approval-detail-row">
                <span class="ai-approval-detail-label">{{ tApp('ai.sidebar.label.id') }}</span>
                <span class="ai-approval-detail-value">{{ String(activeApproval.payload?.datasourceId || '') }}</span>
              </div>
            </div>
          </template>
        </div>
        <div class="ai-approval-actions">
          <button
            v-if="!isApproving"
            class="ai-approval-btn ai-approval-reject"
            type="button"
            :disabled="isSending"
            @click="respondToApproval('reject')"
          >
            {{ tApp('ai.sidebar.reject') }}
          </button>
          <button
            class="ai-approval-btn ai-approval-approve"
            :class="{ 'is-executing': isApproving }"
            type="button"
            :disabled="isSending"
            @click="respondToApproval('approve')"
          >
            <svg v-if="isApproving" class="ai-approval-spinner" viewBox="0 0 20 20" aria-hidden="true">
              <circle cx="10" cy="10" r="7.5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-dasharray="36 60" />
            </svg>
            {{ isApproving ? tApp('ai.sidebar.executingAction') : tApp('ai.sidebar.approve') }}
          </button>
        </div>
      </div>
      <div class="ai-chat-stream">
        <div
          v-for="msg in activeMessages"
          :key="msg.id"
          class="ai-message"
          :class="msg.role"
        >
          <AiMarkdown :content="msg.content" />
          <div
            v-if="msg.role === 'user' && String(msg.implicitStatement || '').trim()"
            class="ai-message-implicit"
          >
            <div class="ai-message-implicit-label">{{ tApp('ai.sidebar.statementContext') }}</div>
            <pre><code>{{ String(msg.implicitStatement || '') }}</code></pre>
          </div>
          <div v-if="msg.role === 'assistant' && msg.plan" class="ai-plan-card">
            <div class="ai-plan-header">
              <div class="ai-plan-title">{{ String(msg.plan.title || tApp('ai.sidebar.plan.title')) }}</div>
              <div class="ai-plan-agent">{{ resolveAgentModeLabel(msg.agent?.mode) }}</div>
            </div>
            <div v-if="msg.plan.summary" class="ai-plan-summary">{{ String(msg.plan.summary || '') }}</div>

            <div class="ai-plan-tabs" role="tablist" :aria-label="tApp('ai.sidebar.plan.tabGroup')">
              <button
                class="ai-plan-tab"
                :class="{ active: getPlanView(msg.id) === 'markdown' }"
                type="button"
                role="tab"
                :aria-selected="getPlanView(msg.id) === 'markdown'"
                @click="setPlanView(msg.id, 'markdown')"
              >
                {{ tApp('ai.sidebar.plan.tab.markdown') }}
              </button>
              <button
                class="ai-plan-tab"
                :class="{ active: getPlanView(msg.id) === 'workflow' }"
                type="button"
                role="tab"
                :aria-selected="getPlanView(msg.id) === 'workflow'"
                @click="setPlanView(msg.id, 'workflow')"
              >
                {{ tApp('ai.sidebar.plan.tab.workflow') }}
              </button>
            </div>

            <div v-if="getPlanView(msg.id) === 'markdown'" class="ai-plan-markdown">
              <AiMarkdown :content="buildPlanMarkdown(msg.plan)" />
            </div>
            <ol v-else class="ai-plan-workflow">
              <li
                v-for="(step, index) in (Array.isArray(msg.plan.steps) ? msg.plan.steps : [])"
                :key="String(step.id || `${msg.id}_${index}`)"
                class="ai-plan-step"
              >
                <div class="ai-plan-step-top">
                  <span class="ai-plan-step-index">{{ index + 1 }}</span>
                  <span class="ai-plan-step-title">
                    {{ String(step.title || tApp('ai.sidebar.plan.stepDefault', { index: index + 1 })) }}
                  </span>
                  <span class="ai-plan-step-status">{{ resolvePlanStepStatusLabel(step.status) }}</span>
                </div>
                <div v-if="step.description" class="ai-plan-step-description">
                  {{ String(step.description || '') }}
                </div>
              </li>
              <li v-if="!(Array.isArray(msg.plan.steps) && msg.plan.steps.length)" class="ai-plan-empty">
                {{ tApp('ai.sidebar.plan.empty') }}
              </li>
            </ol>
          </div>
        </div>
      </div>
    </div>

    <div class="ai-composer">
      <div v-if="contextChips.length" class="ai-context-row">
        <span v-for="chip in contextChips" :key="chip.id" class="ai-context-chip">
          {{ chip.label }}
          <button
            class="ai-context-remove"
            type="button"
            @click="removeContext(chip.id)"
            :aria-label="tApp('ai.sidebar.removeContext')"
          >
            ×
          </button>
        </span>
      </div>
      <div class="ai-composer-box">
        <textarea
          id="ai-sidebar-composer"
          name="ai-sidebar-composer"
          class="ai-composer-input ai-composer-input-area"
          v-model="draft"
          ref="composerInputRef"
          rows="1"
          autocapitalize="off"
          autocorrect="off"
          spellcheck="false"
          :disabled="isSending || Boolean(activeApproval)"
          :placeholder="tApp('ai.sidebar.placeholder')"
          @input="handleInput"
          @keydown="handleComposerKeydown"
        ></textarea>

        <div class="ai-composer-toolbar">
          <div ref="modelSelectRef" class="ai-model-select">
            <button
              class="ai-model-trigger"
              type="button"
              :disabled="!providerOptions.length || isSending"
              :aria-expanded="modelOpen"
              :aria-controls="modelMenuId"
              aria-haspopup="listbox"
              @click="toggleModelMenu"
              @keydown="handleModelKeydown"
            >
              <div class="ai-composer-icon ai-model-icon">
                <svg class="ai-composer-glyph" viewBox="0 0 24 24" aria-hidden="true">
                  <path
                    d="M12 3.5l2.6 5.5 6 .9-4.3 4.2 1 6-5.3-3-5.3 3 1-6L3.4 9.9l6-.9L12 3.5z"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="1.5"
                    stroke-linejoin="round"
                  />
                </svg>
              </div>
              <span class="ai-model-trigger-label">{{ selectedProviderLabel }}</span>
              <span class="ai-model-arrow">▾</span>
            </button>
            <div
              v-if="modelOpen"
              :id="modelMenuId"
              class="ai-model-menu"
              :class="{ 'ai-model-menu-up': modelMenuPlacement === 'up' }"
              role="listbox"
            >
              <button
                v-for="(opt, index) in providerOptions"
                :key="opt.id"
                class="ai-model-option"
                :class="{
                  active: index === modelActiveIndex,
                  selected: opt.id === selectedProviderId,
                }"
                type="button"
                role="option"
                :aria-selected="opt.id === selectedProviderId"
                @click="selectModel(opt.id)"
                @mouseenter="modelActiveIndex = index"
              >
                <span class="ai-model-option-label">{{ opt.label }}</span>
                <span v-if="opt.id === selectedProviderId" class="ai-model-option-check">✓</span>
              </button>
            </div>
          </div>

          <div class="ai-composer-actions">
            <button
              class="ai-voice-btn"
              type="button"
              :aria-label="tApp('ai.sidebar.voiceInput')"
              :title="tApp('ai.sidebar.voiceInputSoon')"
              disabled
            >
              <svg viewBox="0 0 24 24" aria-hidden="true">
                <path
                  d="M12 3a3 3 0 0 0-3 3v6a3 3 0 1 0 6 0V6a3 3 0 0 0-3-3Z"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="1.8"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                />
                <path
                  d="M19 11a7 7 0 0 1-14 0M12 18v3M9 21h6"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="1.8"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                />
              </svg>
            </button>
            <button
              class="ai-send-circle-btn ai-send-icon"
              type="button"
              :disabled="Boolean(activeApproval) || (!isBusy && (!draft.trim() || isSending))"
              @click="isBusy ? cancelInFlight() : send()"
              :aria-label="isBusy ? tApp('ai.sidebar.pause') : tApp('ai.sidebar.send')"
            >
              <span v-if="isBusy" aria-hidden="true">⏸</span>
              <svg
                v-else
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2.2"
                stroke-linecap="round"
                stroke-linejoin="round"
                aria-hidden="true"
              >
                <line x1="12" y1="19" x2="12" y2="5"></line>
                <polyline points="5 12 12 5 19 12"></polyline>
              </svg>
            </button>
          </div>
        </div>
      </div>
      <div v-if="showContext && filteredGroups.length" class="ai-context-dropdown">
        <div v-for="group in filteredGroups" :key="group.title" class="ai-context-group">
          <div class="ai-context-group-title">{{ group.title }}</div>
          <button
            v-for="item in group.items"
            :key="item.id"
            class="ai-context-item"
            :class="{ active: contextIndexMap.get(item.id) === activeContextIndex }"
            type="button"
            @click="selectContext(item)"
          >
            {{ item.label }}
          </button>
        </div>
      </div>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useAiSidebar } from './useAiSidebar'
import AiMarkdown from './AiMarkdown.vue'
import { tApp } from '@/modules/i18n/appI18n'

const {
  store,
  draft,
  composerInputRef,
  contextChips,
  showContext,
  selectedProviderId,
  isSending,
  isApproving,
  isBusy,
  activeApproval,
  approvalToneClass,
  activeContextIndex,
  modelOpen,
  modelActiveIndex,
  modelSelectRef,
  modelMenuId,
  modelMenuPlacement,
  activeMessages,
  providerOptions,
  selectedProviderLabel,
  filteredGroups,
  contextIndexMap,
  getPlanView,
  setPlanView,
  resolveAgentModeLabel,
  resolvePlanStepStatusLabel,
  buildPlanMarkdown,
  handleNewChat,
  handleInput,
  handleComposerKeydown,
  toggleModelMenu,
  handleModelKeydown,
  selectModel,
  selectContext,
  removeContext,
  send,
  cancelInFlight,
  respondToApproval,
} = useAiSidebar()

const executeGateReason = computed<string>(() => {
  const approval: any = activeApproval.value
  if (!approval || approval.kind !== 'execute_statement') return ''
  const trust = String(approval.payload?.trustLevel || '').trim()
  const risk = String(approval.payload?.risk?.level || '').trim().toLowerCase()
  if (!trust) return ''
  const trustLabel = tApp(`riskRules.trustLevels.${trust}.label`)
  return tApp('ai.sidebar.gateReason', {
    trust: trustLabel,
    risk: risk ? risk.toUpperCase() : tApp('ai.sidebar.riskUnknown'),
  })
})
</script>
