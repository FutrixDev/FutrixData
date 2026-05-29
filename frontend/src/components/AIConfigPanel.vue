<template>
  <div class="ai-panel" :class="{ show: visible, inline }" id="ai-config-panel">
    <div v-if="!inline" class="ai-panel-resizer" @mousedown.prevent="startResize"></div>
    <div class="ai-panel-shell">
      <div class="ai-panel-header">
        <div class="ai-panel-title">
          <h3>{{ tApp('ai.panel.title') }}</h3>
          <p class="ai-panel-subtitle">{{ tApp('ai.panel.subtitle') }}</p>
        </div>
        <button
          v-if="!inline"
          class="btn ghost"
          type="button"
          id="ai-panel-close"
          @click="$emit('close')"
        >
          &times;
        </button>
      </div>
      <div class="ai-panel-core">
        <div class="ai-panel-core-head">
          <span class="ai-panel-section">{{ tApp('ai.panel.providers') }}</span>
          <button class="btn" type="button" @click="$emit('create')">{{ tApp('ai.panel.addProvider') }}</button>
        </div>
        <div class="ai-panel-list-body" id="aiconfig-list">
          <div v-if="!configs.length" class="empty-state">
            {{ tApp('ai.panel.empty') }}
          </div>
          <div v-else-if="split" class="ai-config-columns">
            <div class="ai-config-column">
              <div class="ai-config-column-title">
                <span>{{ tApp('status.connected') }}</span>
                <span class="ai-group-count">{{ connectedConfigs.length }}</span>
              </div>
              <div v-if="!connectedConfigs.length" class="empty-state">
                {{ tApp('ai.panel.noConnected') }}
              </div>
              <div v-for="cfg in connectedConfigs" :key="cfg.id" class="ai-card">
                <div class="ai-card-main">
                  <div class="ai-card-title">
                    <span class="ai-status-dot" :class="statusClass(cfg.status)"></span>
                    <span class="ai-provider-name">{{ cfg.name || cfg.provider }}</span>
                  </div>
                  <div class="ai-card-model">{{ cfg.model || tApp('ai.panel.modelNotSet') }}</div>
                  <div class="ai-card-status" :class="statusClass(cfg.status)">
                    <div class="ai-card-status-row">
                      <span class="status" :class="statusClass(cfg.status)">{{ statusLabel(cfg.status) }}</span>
                      <button
                        v-if="shouldToggleDetail(cfg)"
                        class="btn ghost mini ai-detail-toggle"
                        type="button"
                        @click="toggleDetail(cfg.id)"
                      >
                        {{ isExpanded(cfg.id) ? tApp('common.less') : tApp('common.more') }}
                      </button>
                    </div>
                    <div
                      v-if="statusDetail(cfg)"
                      class="status-detail"
                      :class="{ expanded: isExpanded(cfg.id) }"
                    >
                      {{ statusDetail(cfg) }}
                    </div>
                  </div>
                </div>
                <div class="ai-card-actions">
                  <button class="btn secondary small" type="button" @click="testConfig(cfg)">{{ tApp('common.test') }}</button>
                  <div class="ai-action-menu">
                    <button
                      class="btn ghost small ai-action-toggle"
                      type="button"
                      :aria-label="tApp('ai.panel.moreActions')"
                      @click.stop="toggleActionMenu(cfg.id)"
                    >
                      &#8942;
                    </button>
                    <div
                      v-if="isActionMenuOpen(cfg.id)"
                      class="ai-action-dropdown"
                      @click.stop
                    >
                      <button class="ai-action-item" type="button" @click="openEdit(cfg.id)">{{ tApp('common.edit') }}</button>
                      <button class="ai-action-item danger" type="button" @click="openDelete(cfg)">{{ tApp('common.delete') }}</button>
                    </div>
                  </div>
                </div>
              </div>
            </div>
            <div class="ai-config-column">
              <div class="ai-config-column-title">
                <span>{{ tApp('ai.panel.needsAttention') }}</span>
                <span class="ai-group-count">{{ failedConfigs.length }}</span>
              </div>
              <div v-if="!failedConfigs.length" class="empty-state">
                {{ tApp('ai.panel.noFailed') }}
              </div>
              <div v-for="cfg in failedConfigs" :key="cfg.id" class="ai-card">
                <div class="ai-card-main">
                  <div class="ai-card-title">
                    <span class="ai-status-dot" :class="statusClass(cfg.status)"></span>
                    <span class="ai-provider-name">{{ cfg.name || cfg.provider }}</span>
                  </div>
                  <div class="ai-card-model">{{ cfg.model || tApp('ai.panel.modelNotSet') }}</div>
                  <div class="ai-card-status" :class="statusClass(cfg.status)">
                    <div class="ai-card-status-row">
                      <span class="status" :class="statusClass(cfg.status)">{{ statusLabel(cfg.status) }}</span>
                      <button
                        v-if="shouldToggleDetail(cfg)"
                        class="btn ghost mini ai-detail-toggle"
                        type="button"
                        @click="toggleDetail(cfg.id)"
                      >
                        {{ isExpanded(cfg.id) ? tApp('common.less') : tApp('common.more') }}
                      </button>
                    </div>
                    <div
                      v-if="statusDetail(cfg)"
                      class="status-detail"
                      :class="{ expanded: isExpanded(cfg.id) }"
                    >
                      {{ statusDetail(cfg) }}
                    </div>
                  </div>
                </div>
                <div class="ai-card-actions">
                  <button class="btn secondary small" type="button" @click="testConfig(cfg)">{{ tApp('common.test') }}</button>
                  <div class="ai-action-menu">
                    <button
                      class="btn ghost small ai-action-toggle"
                      type="button"
                      :aria-label="tApp('ai.panel.moreActions')"
                      @click.stop="toggleActionMenu(cfg.id)"
                    >
                      &#8942;
                    </button>
                    <div
                      v-if="isActionMenuOpen(cfg.id)"
                      class="ai-action-dropdown"
                      @click.stop
                    >
                      <button class="ai-action-item" type="button" @click="openEdit(cfg.id)">{{ tApp('common.edit') }}</button>
                      <button class="ai-action-item danger" type="button" @click="openDelete(cfg)">{{ tApp('common.delete') }}</button>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
          <div v-else v-for="cfg in sortedConfigs" :key="cfg.id" class="ai-card">
            <div class="ai-card-main">
              <div class="ai-card-title">
                <span class="ai-status-dot" :class="statusClass(cfg.status)"></span>
                <span class="ai-provider-name">{{ cfg.name || cfg.provider }}</span>
              </div>
              <div class="ai-card-model">{{ cfg.model || tApp('ai.panel.modelNotSet') }}</div>
              <div class="ai-card-status" :class="statusClass(cfg.status)">
                <div class="ai-card-status-row">
                  <span class="status" :class="statusClass(cfg.status)">{{ statusLabel(cfg.status) }}</span>
                  <button
                    v-if="shouldToggleDetail(cfg)"
                    class="btn ghost mini ai-detail-toggle"
                    type="button"
                    @click="toggleDetail(cfg.id)"
                  >
                    {{ isExpanded(cfg.id) ? tApp('common.less') : tApp('common.more') }}
                  </button>
                </div>
                <div
                  v-if="statusDetail(cfg)"
                  class="status-detail"
                  :class="{ expanded: isExpanded(cfg.id) }"
                >
                  {{ statusDetail(cfg) }}
                </div>
              </div>
            </div>
            <div class="ai-card-actions">
              <button class="btn secondary small" type="button" @click="testConfig(cfg)">{{ tApp('common.test') }}</button>
              <div class="ai-action-menu">
                <button
                  class="btn ghost small ai-action-toggle"
                  type="button"
                  :aria-label="tApp('ai.panel.moreActions')"
                  @click.stop="toggleActionMenu(cfg.id)"
                >
                  &#8942;
                </button>
                <div
                  v-if="isActionMenuOpen(cfg.id)"
                  class="ai-action-dropdown"
                  @click.stop
                >
                  <button class="ai-action-item" type="button" @click="openEdit(cfg.id)">{{ tApp('common.edit') }}</button>
                  <button class="ai-action-item danger" type="button" @click="openDelete(cfg)">{{ tApp('common.delete') }}</button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div
      v-if="deleteConfirmOpen"
      class="dialog-backdrop"
      role="dialog"
      aria-modal="true"
      data-testid="aiconfig-delete-confirm-dialog"
    >
      <div class="dialog-card dialog-card--danger">
        <div class="dialog-head">
          <div class="dialog-head-main">
            <div class="dialog-icon danger"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg></div>
            <div>
              <h4>{{ tApp('ai.panel.deleteTitle') }}</h4>
              <div class="meta">
                <span>{{ tApp('common.cannotUndo') }}</span>
              </div>
            </div>
          </div>
        </div>
        <div v-if="deleteTarget" class="dialog-highlight">
          {{ deleteTarget.name || deleteTarget.provider }}
        </div>
        <div class="dialog-actions">
          <button class="btn ghost" type="button" :disabled="deleteConfirmBusy" @click="closeDeleteConfirm">
            {{ tApp('common.cancel') }}
          </button>
          <button
            class="btn danger"
            type="button"
            :disabled="deleteConfirmBusy"
            data-testid="aiconfig-delete-confirm"
            @click="confirmDelete"
          >
            {{ tApp('common.delete') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useAIConfigPanel } from './useAIConfigPanel'
import { tApp } from '@/modules/i18n/appI18n'

const props = defineProps<{ visible: boolean; inline?: boolean; split?: boolean }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'create'): void; (e: 'edit', id: string): void }>()

const {
  inline,
  split,
  configs,
  connectedConfigs,
  failedConfigs,
  sortedConfigs,
  statusLabel,
  statusClass,
  statusDetail,
  shouldToggleDetail,
  isExpanded,
  toggleDetail,
  isActionMenuOpen,
  toggleActionMenu,
  openEdit,
  openDelete,
  deleteConfirmOpen,
  deleteConfirmBusy,
  deleteTarget,
  closeDeleteConfirm,
  confirmDelete,
  testConfig,
  startResize,
} = useAIConfigPanel(props, emit)
</script>
