<script setup lang="ts">
import { computed } from 'vue'
import { formatAppList, tApp } from '@/modules/i18n/appI18n'
import { useConsoleViewContext } from '../context'

const ctx = useConsoleViewContext()

const riskDanger = ctx.riskDanger
const closeRiskDanger = ctx.closeRiskDanger
const confirmRiskDanger = ctx.confirmRiskDanger

const isBlock = computed(() => riskDanger.value?.riskInfo?.action === 'block')
const pillClass = computed(() => isBlock.value ? 'pill-danger' : 'pill-warning')

const pillLabel = computed(() => {
  const info = riskDanger.value?.riskInfo
  if (!info) return ''
  if (info.level === 'high') return tApp('danger.levelHigh')
  if (info.level === 'medium') return tApp('danger.levelMedium')
  return tApp('danger.review')
})
</script>

<template>
  <div
    v-if="riskDanger"
    class="dialog-backdrop"
    role="dialog"
    aria-modal="true"
    data-testid="risk-danger-dialog"
  >
    <div class="dialog-card" :class="isBlock ? 'dialog-card--danger' : 'dialog-card--warning'">
      <div class="dialog-head">
        <div class="dialog-head-main">
          <div class="dialog-icon" :class="isBlock ? 'danger' : 'warning'"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg></div>
          <div>
            <h4>{{ isBlock ? tApp('danger.blockTitle') : tApp('danger.warnTitle') }}</h4>
            <div class="meta">
              <span>{{ tApp('danger.riskSubtitle') }}</span>
            </div>
          </div>
        </div>
        <span class="pill" :class="pillClass">{{ pillLabel }}</span>
      </div>
      <div class="dialog-command">{{ riskDanger.statement }}</div>
      <div class="dialog-stages" v-if="riskDanger.riskInfo.reasons?.length">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
        <span>{{ riskDanger.riskInfo.reasons.join('; ') }}</span>
      </div>
      <div class="dialog-stages" v-if="riskDanger.riskInfo.explain?.stages?.length">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 16 16 12 12 8"/><line x1="8" y1="12" x2="16" y2="12"/></svg>
        <span>{{ tApp('danger.stages', { stages: riskDanger.riskInfo.explain.stages.join(' \u2192 ') }) }}</span>
      </div>
      <div class="dialog-stages" v-else-if="riskDanger.riskInfo.explain?.indexes?.length">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 16 16 12 12 8"/><line x1="8" y1="12" x2="16" y2="12"/></svg>
        <span>{{ tApp('danger.indexes', { indexes: formatAppList(riskDanger.riskInfo.explain.indexes) }) }}</span>
      </div>
      <div class="dialog-actions">
        <button class="btn ghost" type="button" @click="closeRiskDanger">{{ tApp('danger.cancel') }}</button>
        <button class="btn danger" type="button" data-testid="risk-danger-confirm" @click="confirmRiskDanger">
          {{ tApp('danger.runAnyway') }}
        </button>
      </div>
    </div>
  </div>
</template>
