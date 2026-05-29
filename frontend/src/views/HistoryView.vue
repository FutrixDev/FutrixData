<template>
  <section class="view active" id="view-history">
    <div class="list-toolbar">
      <div>
        <h2>{{ tApp('history.title') }}</h2>
        <p class="meta">{{ tApp('history.subtitle') }}</p>
      </div>
    </div>

    <div class="history-subtabs" role="tablist" :aria-label="tApp('history.title')">
      <button
        class="history-subtab"
        :class="{ 'history-subtab--active': activeTab === 'console' }"
        type="button"
        role="tab"
        data-testid="history-tab-console"
        :aria-selected="activeTab === 'console'"
        @click="activeTab = 'console'"
      >
        {{ tApp('history.tab.console') }}
      </button>
      <button
        class="history-subtab"
        :class="{ 'history-subtab--active': activeTab === 'agent-audit' }"
        type="button"
        role="tab"
        data-testid="history-tab-agent-audit"
        :aria-selected="activeTab === 'agent-audit'"
        @click="activeTab = 'agent-audit'"
      >
        {{ tApp('history.tab.agentAudit') }}
      </button>
    </div>

    <div class="list-controls history-controls">
      <div class="list-controls-left">
        <input
          id="history-search"
          v-model="keyword"
          :placeholder="tApp('history.searchPlaceholder')"
          data-testid="history-search-input"
          autocapitalize="off"
          autocorrect="off"
          spellcheck="false"
        />
        <label
          v-if="activeTab === 'agent-audit'"
          class="history-agent-filter"
        >
          <span class="history-agent-filter__label">{{ tApp('history.agent.filterLabel') }}:</span>
          <select
            v-model="agentFilter"
            class="history-agent-filter__select"
            data-testid="history-agent-filter"
          >
            <option value="">{{ tApp('history.agent.filterAll') }}</option>
            <option
              v-for="option in agentFilterOptions"
              :key="option.accessKey"
              :value="option.accessKey"
            >
              {{ option.agentName }}{{ option.revoked ? ` · ${tApp('history.agent.revokedBadge')}` : '' }}
            </option>
          </select>
        </label>
        <div v-if="filterPills.length" class="history-filter-pills">
          <span
            v-for="pill in filterPills"
            :key="pill.label"
            class="pill history-pill"
            :class="pill.className"
          >
            {{ pill.label }}: {{ pill.value }}
          </span>
        </div>
      </div>
      <div class="list-controls-right">
        <button
          v-if="activeTab === 'console' && entries.length"
          class="btn ghost small"
          type="button"
          data-testid="history-clear-filtered"
          @click="openClearConfirm"
        >
          {{ tApp('history.clearFiltered') }}
        </button>
        <button v-if="filterPills.length || keyword.trim() || agentFilter" class="btn ghost small" type="button" @click="clearFilters">{{ tApp('history.clearFilters') }}</button>
      </div>
    </div>

    <div v-if="activeTab === 'console'" class="history-grid">
      <div v-if="loading" class="card history-card">
        <div class="meta">{{ tApp('history.loading') }}</div>
      </div>
      <div v-else-if="loadError" class="card history-card">
        <div class="meta">{{ tApp('history.loadFailed', { message: loadError }) }}</div>
      </div>
      <div v-else-if="!entries.length" class="card history-card">
        <div class="meta">{{ tApp('history.empty') }}</div>
      </div>
      <div v-for="entry in entries" :key="entry.id" class="card history-card">
        <div class="history-card-head">
          <button class="history-statement" type="button" @click="openHistoryEntry(entry)">
            {{ entry.statement }}
          </button>
          <div class="history-card-actions">
            <span class="history-timestamp">{{ formatTimestamp(entry.executedAt) }}</span>
            <button
              class="btn ghost mini danger"
              type="button"
              data-testid="history-delete"
              @click.stop="deleteEntry(entry)"
            >
              {{ tApp('common.delete') }}
            </button>
          </div>
        </div>
        <div class="history-meta">
          <span>{{ entry.datasourceName }} · {{ datasourceTypeLabel(entry.datasourceType) }}</span>
          <span v-if="historyDatabaseLabel(entry)">{{ historyDatabaseLabel(entry) }}</span>
          <span v-if="entry.targets?.length"
            >{{ targetLabel(entry.targets || []) }}: {{ (entry.targets || []).join(', ') }}</span
          >
        </div>
        <div class="history-tags">
          <span
            v-for="tag in historyTags(entry)"
            :key="tag.key"
            class="pill history-pill"
            :class="[`history-tag--${tag.type}`, tag.className]"
          >
            {{ tag.label }}
          </span>
        </div>
      </div>
    </div>

    <div v-else class="history-grid history-grid--agent">
      <div v-if="loading" class="card history-card">
        <div class="meta">{{ tApp('history.loading') }}</div>
      </div>
      <div v-else-if="loadError" class="card history-card">
        <div class="meta">{{ tApp('history.loadFailed', { message: loadError }) }}</div>
      </div>
      <div v-else-if="!agentEntries.length" class="card history-card">
        <div class="meta">{{ tApp('history.agent.empty') }}</div>
      </div>
      <template v-else>
        <div v-for="entry in agentEntries" :key="entry.id" class="card history-card history-agent-entry">
            <div class="history-card-head">
              <div class="history-agent-entry__title">
                <code class="history-agent-entry__tool">{{ entry.toolName }}</code>
                <span
                  v-if="agentSummaryFor(entry)"
                  class="history-agent-entry__summary"
                >{{ agentSummaryFor(entry) }}</span>
              </div>
              <div class="history-card-actions">
                <span class="pill history-pill history-agent-status" :class="`history-agent-status--${entry.status}`">
                  {{ agentStatusLabel(entry.status) }}
                </span>
                <span class="history-timestamp">{{ formatTimestamp(entry.executedAt) }}</span>
              </div>
            </div>
            <div class="history-agent-entry__identity" data-testid="history-agent-entry-identity">
              <span class="history-agent-entry__identity-label">{{ tApp('history.agent.filterLabel') }}:</span>
              <span class="history-agent-entry__identity-name">{{ entry.agentName || tApp('history.agent.unknown') }}</span>
              <span
                v-if="isAgentRevoked(entry.accessKey)"
                class="pill history-pill history-agent-revoked"
                data-testid="history-agent-revoked-badge"
              >
                {{ tApp('history.agent.revokedBadge') }}
              </span>
              <span class="pill history-pill history-agent-protocol">{{ agentProtocolLabel(entry.protocol) }}</span>
            </div>
            <div class="history-meta">
              <span>{{ tApp('history.filter.datasource') }}: {{ entry.datasourceName || '-' }}</span>
              <span v-if="agentTargetFor(entry)">{{ tApp('history.agent.target') }}: {{ agentTargetFor(entry) }}</span>
            </div>
            <div v-if="entry.statement" class="history-agent-entry__statement-wrap">
              <div class="history-agent-entry__statement-label">{{ tApp('history.agent.statement') }}</div>
              <pre class="history-statement history-statement--static history-agent-entry__statement">{{ entry.statement }}</pre>
            </div>
            <div class="history-tags">
              <span v-if="entry.datasourceType" class="pill history-pill history-tag--type" :class="datasourceTypeClass(entry.datasourceType)">
                {{ datasourceTypeLabel(entry.datasourceType) }}
              </span>
            </div>
            <div
              v-if="entry.riskAttribution"
              class="history-agent-entry__risk"
              :class="[`history-agent-entry__risk--${entry.riskAttribution.action}`, `history-agent-entry__risk--source-${entry.riskAttribution.source}`]"
              data-testid="history-agent-entry-risk"
            >
              <div class="history-agent-entry__risk-head">
                <span class="history-agent-entry__risk-title">{{ tApp('history.agent.risk.title') }}</span>
                <span
                  v-if="entry.riskAttribution.action"
                  class="pill history-pill history-agent-entry__risk-action"
                  :class="`history-agent-entry__risk-action--${entry.riskAttribution.action}`"
                  data-testid="history-agent-entry-risk-action"
                >{{ agentRiskActionLabel(entry.riskAttribution.action) }}</span>
                <span
                  v-if="entry.riskAttribution.level"
                  class="pill history-pill history-agent-entry__risk-level"
                  :class="`history-agent-entry__risk-level--${entry.riskAttribution.level}`"
                >{{ tApp('history.agent.risk.levelLabel') }}: {{ agentRiskLevelLabel(entry.riskAttribution.level) }}</span>
              </div>
              <div
                v-if="entry.riskAttribution.source === 'risk_engine'"
                class="history-agent-entry__risk-rule"
                data-testid="history-agent-entry-risk-rule"
              >
                <span class="history-agent-entry__risk-label">{{ tApp('history.agent.risk.ruleLabel') }}:</span>
                <button
                  v-if="entry.riskAttribution.ruleId"
                  class="link-button history-agent-entry__risk-rule-link"
                  type="button"
                  :title="tApp('history.agent.risk.viewRule')"
                  data-testid="history-agent-entry-risk-rule-link"
                  @click="openRiskRule(entry.riskAttribution)"
                >{{ entry.riskAttribution.ruleDescription || entry.riskAttribution.ruleCode || entry.riskAttribution.ruleId }}</button>
                <span v-else>{{ entry.riskAttribution.ruleDescription || entry.riskAttribution.ruleCode || '-' }}</span>
              </div>
              <div v-else class="history-agent-entry__risk-rule">
                <span class="history-agent-entry__risk-label">{{ tApp('history.agent.risk.ruleLabel') }}:</span>
                <span>{{ tApp('history.agent.risk.sourcePolicy') }}</span>
              </div>
              <div
                v-if="entry.riskAttribution.reasons && entry.riskAttribution.reasons.length"
                class="history-agent-entry__risk-reasons"
              >
                <span class="history-agent-entry__risk-label">{{ tApp('history.agent.risk.reasonsLabel') }}:</span>
                <ul class="history-agent-entry__risk-reasons-list">
                  <li v-for="(reason, idx) in entry.riskAttribution.reasons" :key="idx">{{ reason }}</li>
                </ul>
              </div>
            </div>
            <div
              v-if="entry.message && entry.status !== 'success'"
              class="history-agent-entry__rejection"
              :class="`history-agent-entry__rejection--${entry.status}`"
              data-testid="history-agent-entry-rejection"
            >
              <span class="history-agent-entry__rejection-label">{{ tApp('history.agent.rejectionReason') }}</span>
              <span class="history-agent-entry__rejection-message">{{ entry.message }}</span>
            </div>
            <div v-else-if="entry.message" class="history-meta">
              <span>{{ entry.message }}</span>
            </div>
        </div>
      </template>
    </div>

    <div
      v-if="clearConfirmOpen"
      class="dialog-backdrop"
      role="dialog"
      aria-modal="true"
      data-testid="history-clear-confirm-dialog"
    >
      <div class="dialog-card">
        <div class="dialog-head">
          <div class="dialog-head-main">
            <div class="dialog-icon"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg></div>
            <div>
              <h4>{{ tApp('history.clearFilteredTitle') }}</h4>
              <div class="meta">
                <span>{{ tApp('history.clearFilteredDesc') }}</span>
              </div>
            </div>
          </div>
        </div>
        <div class="dialog-actions">
          <button class="btn ghost" type="button" :disabled="clearConfirmBusy" @click="closeClearConfirm">
            {{ tApp('common.cancel') }}
          </button>
          <button
            class="btn"
            type="button"
            :disabled="clearConfirmBusy"
            data-testid="history-clear-confirm"
            @click="confirmClearFiltered"
          >
            {{ tApp('common.clear') }}
          </button>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useHistoryView } from './history/useHistoryView'
import { tApp } from '@/modules/i18n/appI18n'

const {
  keyword,
  entries,
  agentEntries,
  agentFilter,
  agentFilterOptions,
  activeTab,
  loading,
  loadError,
  filterPills,
  clearConfirmOpen,
  clearConfirmBusy,
  datasourceTypeLabel,
  datasourceTypeClass,
  targetLabel,
  historyDatabaseLabel,
  historyTags,
  formatTimestamp,
  agentProtocolLabel,
  agentStatusLabel,
  agentRiskActionLabel,
  agentRiskLevelLabel,
  openRiskRule,
  agentSummaryFor,
  agentTargetFor,
  isAgentRevoked,
  openHistoryEntry,
  deleteEntry,
  openClearConfirm,
  closeClearConfirm,
  confirmClearFiltered,
  clearFilters,
} = useHistoryView()
</script>
