<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'

import { tApp } from '@/modules/i18n/appI18n'
import { api } from '@/services/api'
import type { SkillAgent, MCPAgent } from '@/services/api/skill'

const emit = defineEmits<{ close: [] }>()

const CODEX_AGENT_ID = 'codex'

interface MergedAgent {
  id: string
  name: string
  /** At least one of skill or MCP is detected — used for row styling. */
  detected: boolean
  skillDetected: boolean
  mcpDetected: boolean
  skillInstalled: boolean
  mcpInstalled: boolean
  /** Whether this agent supports MCP (has a config detected). */
  mcpSupported: boolean
  installPath: string
  configPath: string
}

const agents = ref<MergedAgent[]>([])
const skillSelected = ref<Set<string>>(new Set())
const mcpSelected = ref<Set<string>>(new Set())
const grantSensitivity = ref(false)
const grantDatasourceManagement = ref(false)
const installing = ref(false)
const done = ref(false)
const results = ref<{ name: string; type: string; success: boolean; error?: string }[]>([])
// Tracks per-agent grant write failures separately from install outcomes so
// the user sees that the install succeeded but the grant did not apply —
// otherwise the next sensitivity tool call returns AGENT_FORBIDDEN with no
// hint of why.
const grantFailures = ref<{ name: string; permission: string; error: string }[]>([])

onMounted(async () => {
  const [skillAgents, mcpAgents] = await Promise.all([
    api.detectAIAgents(),
    api.detectMCPAgents(),
  ])

  const mcpById = new Map<string, MCPAgent>()
  for (const m of mcpAgents) mcpById.set(m.id, m)

  const merged: MergedAgent[] = skillAgents.map((s: SkillAgent) => {
    const m = mcpById.get(s.id)
    return {
      id: s.id,
      name: s.name,
      detected: s.detected || !!m?.detected,
      skillDetected: s.detected,
      mcpDetected: !!m?.detected,
      skillInstalled: s.installed,
      mcpInstalled: !!m?.installed,
      mcpSupported: !!m,
      installPath: s.installPath,
      configPath: m?.configPath ?? '',
    }
  })
  // Add MCP-only agents not in skill list (unlikely but safe).
  for (const m of mcpAgents) {
    if (!merged.find(a => a.id === m.id)) {
      merged.push({
        id: m.id, name: m.name, detected: m.detected,
        skillDetected: false, mcpDetected: m.detected,
        skillInstalled: false, mcpInstalled: m.installed,
        mcpSupported: true, installPath: '', configPath: m.configPath,
      })
    }
  }

  agents.value = merged

  // Pre-select uninstalled items for detected agents (per-type detection).
  for (const a of merged) {
    if (a.id !== CODEX_AGENT_ID && a.skillDetected && !a.skillInstalled && a.installPath) skillSelected.value.add(a.id)
    if (a.mcpDetected && a.mcpSupported && !a.mcpInstalled) mcpSelected.value.add(a.id)
  }
})

const toggleSkill = (id: string) => {
  const next = new Set(skillSelected.value)
  next.has(id) ? next.delete(id) : next.add(id)
  skillSelected.value = next
}

const toggleMCP = (id: string) => {
  const next = new Set(mcpSelected.value)
  next.has(id) ? next.delete(id) : next.add(id)
  mcpSelected.value = next
}

const hasSelection = computed(() => skillSelected.value.size > 0 || mcpSelected.value.size > 0)
const codexAgent = computed(() => agents.value.find(a => a.id === CODEX_AGENT_ID))
const codexCanSelectMCP = computed(() => {
  const agent = codexAgent.value
  return Boolean(agent?.mcpDetected && agent.mcpSupported && !agent.mcpInstalled && !installing.value)
})
const codexSetupHintKey = computed(() => {
  const agent = codexAgent.value
  if (!agent?.detected) return 'skill.install.codexSetupNotDetected'
  if (agent.mcpInstalled) return 'skill.install.codexSetupAuthorized'
  return 'skill.install.codexSetupReady'
})

const selectCodexPluginSetup = () => {
  if (!codexCanSelectMCP.value) return
  const nextMCP = new Set(mcpSelected.value)
  nextMCP.add(CODEX_AGENT_ID)
  mcpSelected.value = nextMCP

  const nextSkill = new Set(skillSelected.value)
  nextSkill.delete(CODEX_AGENT_ID)
  skillSelected.value = nextSkill
}

const onInstall = async () => {
  installing.value = true
  try {
    const allResults: typeof results.value = []
    // Dedupe across skill+MCP outcomes for the same agent — both flows mint
    // the same identity (EnsureForInstall is idempotent), so granting twice
    // would just be wasted IPC. Keep the order of first appearance.
    const grantTargets: { accessKey: string; name: string }[] = []
    const seenKeys = new Set<string>()

    if (skillSelected.value.size > 0) {
      const res = await api.installSkill([...skillSelected.value])
      for (const o of res.installed || []) {
        allResults.push({ name: o.name, type: tApp('skill.install.skill'), success: o.success, error: o.error })
        if (o.success && o.accessKey && !seenKeys.has(o.accessKey)) {
          seenKeys.add(o.accessKey)
          grantTargets.push({ accessKey: o.accessKey, name: o.name })
        }
      }
    }

    if (mcpSelected.value.size > 0) {
      const res = await api.installMCP([...mcpSelected.value])
      for (const o of res.installed || []) {
        allResults.push({ name: o.name, type: tApp('skill.install.mcp'), success: o.success, error: o.error })
        if (o.success && o.accessKey && !seenKeys.has(o.accessKey)) {
          seenKeys.add(o.accessKey)
          grantTargets.push({ accessKey: o.accessKey, name: o.name })
        }
      }
    }

    // Apply optional grants once per identity. Failures are surfaced
    // in the results panel rather than suppressed — silent failure here
    // means the user thinks "granted ✓" but tools still return AGENT_FORBIDDEN.
    const grantFails: typeof grantFailures.value = []
    if ((grantSensitivity.value || grantDatasourceManagement.value) && grantTargets.length > 0) {
      for (const target of grantTargets) {
        if (grantSensitivity.value) {
          try {
            await api.setAgentSensitivityGrant(target.accessKey, true)
          } catch (err) {
            const message = err instanceof Error ? err.message : String(err || '')
            grantFails.push({ name: target.name, permission: tApp('skill.install.sensitivityGrantShort'), error: message })
          }
        }
        if (grantDatasourceManagement.value) {
          try {
            await api.setAgentDatasourceManagementGrant(target.accessKey, true)
          } catch (err) {
            const message = err instanceof Error ? err.message : String(err || '')
            grantFails.push({ name: target.name, permission: tApp('skill.install.datasourceGrantShort'), error: message })
          }
        }
      }
    }

    results.value = allResults
    grantFailures.value = grantFails
    done.value = true
  } finally {
    installing.value = false
  }
}

const onSkip = async () => {
  await api.markSkillInstallPrompted()
  emit('close')
}

const onDone = async () => {
  await api.markSkillInstallPrompted()
  emit('close')
}
</script>

<template>
  <div class="dialog-backdrop" role="dialog" aria-modal="true" data-testid="skill-install-dialog">
    <div class="dialog-card skill-dialog">
      <!-- Header -->
      <div class="dialog-head">
        <div class="dialog-head-main">
          <div class="dialog-icon">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/></svg>
          </div>
          <div>
            <h4>{{ tApp('skill.install.title') }}</h4>
            <div class="meta">
              <span>{{ tApp('skill.install.subtitle') }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Agent list (before install) -->
      <template v-if="!done">
        <div v-if="codexAgent" class="skill-codex-setup" data-testid="skill-codex-setup">
          <div class="skill-codex-setup__copy">
            <span class="skill-codex-setup__label">{{ tApp('skill.install.codexSetupLabel') }}</span>
            <span class="skill-codex-setup__hint">{{ tApp(codexSetupHintKey) }}</span>
          </div>
          <button
            v-if="codexCanSelectMCP"
            class="skill-codex-setup__btn"
            type="button"
            data-testid="skill-codex-select-mcp"
            @click="selectCodexPluginSetup"
          >
            {{ tApp('skill.install.codexSetupSelect') }}
          </button>
        </div>

        <div class="skill-approval-policy" data-testid="skill-install-approval-policy">
          {{ tApp('skill.agentApprovalPolicyNotice') }}
        </div>

        <div class="skill-agents" data-testid="skill-agent-list">
          <!-- Column headers -->
          <div class="skill-agent-header">
            <span class="skill-agent-header-name"></span>
            <span class="skill-agent-header-col">{{ tApp('skill.install.skill') }}</span>
            <span class="skill-agent-header-col">{{ tApp('skill.install.mcp') }}</span>
          </div>

          <div
            v-for="agent in agents"
            :key="agent.id"
            class="skill-agent-row"
            :class="{ disabled: !agent.detected }"
            :data-testid="'skill-agent-' + agent.id"
          >
            <span class="skill-agent-name">
              {{ agent.name }}
              <span v-if="!agent.detected" class="skill-agent-badge not-detected">{{ tApp('skill.install.notDetected') }}</span>
            </span>

            <!-- Skill checkbox -->
            <span class="skill-agent-check">
              <template v-if="!agent.installPath">
                <span class="skill-agent-badge not-detected">—</span>
              </template>
              <template v-else-if="agent.skillInstalled">
                <span class="skill-agent-badge installed">{{ tApp('skill.install.alreadyInstalled') }}</span>
              </template>
              <template v-else>
                <input
                  type="checkbox"
                  :checked="skillSelected.has(agent.id)"
                  :disabled="!agent.skillDetected || installing"
                  @change="toggleSkill(agent.id)"
                />
              </template>
            </span>

            <!-- MCP checkbox -->
            <span class="skill-agent-check">
              <template v-if="!agent.mcpSupported">
                <span class="skill-agent-badge not-detected">—</span>
              </template>
              <template v-else-if="agent.mcpInstalled">
                <span class="skill-agent-badge installed">{{ tApp('skill.install.alreadyInstalled') }}</span>
              </template>
              <template v-else>
                <input
                  type="checkbox"
                  :checked="mcpSelected.has(agent.id)"
                  :disabled="!agent.mcpDetected || installing"
                  @change="toggleMCP(agent.id)"
                />
              </template>
            </span>
          </div>
        </div>

        <label class="skill-install-grant" for="skill-install-grant">
          <input
            id="skill-install-grant"
            v-model="grantSensitivity"
            type="checkbox"
            :disabled="!hasSelection || installing"
            data-testid="skill-install-grant-input"
          />
          <span class="skill-install-grant__text">
            <span class="skill-install-grant__label">{{ tApp('skill.install.sensitivityGrantLabel') }}</span>
            <span class="skill-install-grant__hint">{{ tApp('skill.install.sensitivityGrantHint') }}</span>
          </span>
        </label>

        <label class="skill-install-grant" for="skill-install-datasource-grant">
          <input
            id="skill-install-datasource-grant"
            v-model="grantDatasourceManagement"
            type="checkbox"
            :disabled="!hasSelection || installing"
            data-testid="skill-install-datasource-grant-input"
          />
          <span class="skill-install-grant__text">
            <span class="skill-install-grant__label">{{ tApp('skill.install.datasourceGrantLabel') }}</span>
            <span class="skill-install-grant__hint">{{ tApp('skill.install.datasourceGrantHint') }}</span>
          </span>
        </label>

        <div class="dialog-actions">
          <button class="btn ghost" type="button" :disabled="installing" @click="onSkip">
            {{ tApp('skill.install.skip') }}
          </button>
          <button
            class="btn primary"
            type="button"
            :disabled="!hasSelection || installing"
            data-testid="skill-install-confirm"
            @click="onInstall"
          >
            {{ installing ? tApp('skill.install.installing') : tApp('skill.install.install') }}
          </button>
        </div>
      </template>

      <!-- Results (after install) -->
      <template v-else>
        <div class="skill-results" data-testid="skill-install-results">
          <div v-for="(r, i) in results" :key="i" class="skill-result-row">
            <span v-if="r.success" class="skill-result-icon success">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>
            </span>
            <span v-else class="skill-result-icon error">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
            </span>
            <span class="skill-result-name">{{ r.name }}</span>
            <span class="skill-result-type">{{ r.type }}</span>
            <span v-if="r.error" class="skill-result-error">{{ r.error }}</span>
          </div>
        </div>

        <div
          v-if="grantFailures.length > 0"
          class="skill-grant-failures"
          data-testid="skill-install-grant-failures"
        >
          <p class="skill-grant-failures__title">
            {{ tApp('skill.install.grantPartialTitle') }}
          </p>
          <div
            v-for="(f, i) in grantFailures"
            :key="i"
            class="skill-grant-failures__row"
          >
            <span class="skill-grant-failures__name">{{ f.name }}</span>
            <span class="skill-grant-failures__name">{{ f.permission }}</span>
            <span class="skill-grant-failures__error">{{ f.error }}</span>
          </div>
          <p class="skill-grant-failures__hint">
            {{ tApp('skill.install.grantPartialHint') }}
          </p>
        </div>

        <div class="dialog-actions">
          <button class="btn primary" type="button" data-testid="skill-install-done" @click="onDone">
            {{ tApp('skill.install.done') }}
          </button>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.skill-dialog {
  width: min(520px, 92vw);
}

.skill-agents {
  display: grid;
  gap: 2px;
}

.skill-agent-header {
  display: grid;
  grid-template-columns: 1fr 80px 80px;
  gap: 10px;
  padding: 4px 12px;
  font-size: 11px;
  font-weight: 600;
  color: var(--soft-ink);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.skill-agent-header-col {
  text-align: center;
}

.skill-agent-row {
  display: grid;
  grid-template-columns: 1fr 80px 80px;
  align-items: center;
  gap: 10px;
  padding: 9px 12px;
  border-radius: 10px;
  font-size: 13px;
  transition: background 0.15s;
}

.skill-agent-row:hover:not(.disabled) {
  background: color-mix(in oklab, var(--primary) 6%, transparent);
}

.skill-agent-row.disabled {
  opacity: 0.55;
  cursor: default;
}

.skill-agent-name {
  font-weight: 600;
  color: var(--ink);
  display: flex;
  align-items: center;
  gap: 8px;
}

.skill-agent-check {
  display: flex;
  justify-content: center;
  align-items: center;
}

.skill-agent-check input[type="checkbox"] {
  width: 16px;
  height: 16px;
  margin: 0;
  accent-color: var(--primary);
  cursor: pointer;
}

.skill-agent-badge {
  font-size: 11px;
  font-weight: 600;
  padding: 2px 7px;
  border-radius: 6px;
  letter-spacing: 0.02em;
  white-space: nowrap;
}

.skill-agent-badge.installed {
  background: color-mix(in oklab, var(--success, #22c55e) 12%, var(--panel));
  color: var(--success, #16a34a);
}

.skill-agent-badge.not-detected {
  background: color-mix(in oklab, var(--soft-ink) 10%, var(--panel));
  color: var(--soft-ink);
}

.skill-codex-setup {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 10px;
  border: 1px solid color-mix(in oklab, var(--primary) 18%, var(--edge));
  background: color-mix(in oklab, var(--primary) 5%, var(--panel));
}

.skill-codex-setup__copy {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.skill-codex-setup__label {
  font-size: 12px;
  font-weight: 700;
  color: var(--ink);
}

.skill-codex-setup__hint {
  font-size: 12px;
  color: var(--soft-ink);
  line-height: 1.45;
}

.skill-codex-setup__btn {
  flex: 0 0 auto;
  padding: 5px 10px;
  border-radius: 8px;
  border: 1px solid color-mix(in oklab, var(--primary) 26%, var(--edge));
  background: var(--panel);
  color: var(--primary);
  font: inherit;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  white-space: nowrap;
}

.skill-codex-setup__btn:hover {
  background: color-mix(in oklab, var(--primary) 10%, var(--panel));
}

.skill-approval-policy {
  margin: 10px 0;
  padding: 9px 11px;
  border-radius: 8px;
  border: 1px solid color-mix(in oklab, var(--warn, #f59e0b) 28%, var(--edge));
  background: color-mix(in oklab, var(--warn, #f59e0b) 9%, var(--panel));
  color: var(--ink);
  font-size: 12px;
  line-height: 1.5;
}

.skill-results {
  display: grid;
  gap: 8px;
}

.skill-result-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}

.skill-result-icon {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  flex-shrink: 0;
}

.skill-result-icon.success {
  background: color-mix(in oklab, var(--success, #22c55e) 15%, var(--panel));
  color: var(--success, #16a34a);
}

.skill-result-icon.error {
  background: color-mix(in oklab, var(--danger) 15%, var(--panel));
  color: var(--danger, #b91c1c);
}

.skill-result-name {
  font-weight: 600;
  color: var(--ink);
}

.skill-result-type {
  font-size: 11px;
  font-weight: 600;
  padding: 1px 6px;
  border-radius: 4px;
  background: color-mix(in oklab, var(--primary) 10%, var(--panel));
  color: var(--primary);
}

.skill-result-error {
  font-size: 12px;
  color: var(--danger, #b91c1c);
}

.skill-install-grant {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 10px;
  background: color-mix(in oklab, var(--primary) 5%, var(--panel));
  border: 1px solid color-mix(in oklab, var(--primary) 14%, var(--edge));
  cursor: pointer;
}

.skill-install-grant input[type="checkbox"] {
  width: 16px;
  height: 16px;
  margin-top: 2px;
  accent-color: var(--primary);
  cursor: pointer;
}

.skill-install-grant__text {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.skill-install-grant__label {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink);
}

.skill-install-grant__hint {
  font-size: 12px;
  color: var(--soft-ink);
  line-height: 1.5;
}

.skill-grant-failures {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  border-radius: 10px;
  background: color-mix(in oklab, var(--danger, #b91c1c) 8%, var(--panel));
  border: 1px solid color-mix(in oklab, var(--danger, #b91c1c) 22%, var(--edge));
}

.skill-grant-failures__title {
  margin: 0;
  font-size: 13px;
  font-weight: 700;
  color: var(--danger, #b91c1c);
}

.skill-grant-failures__row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
}

.skill-grant-failures__name {
  font-weight: 600;
  color: var(--ink);
}

.skill-grant-failures__error {
  color: var(--danger, #b91c1c);
}

.skill-grant-failures__hint {
  margin: 2px 0 0;
  font-size: 11px;
  color: var(--soft-ink);
  line-height: 1.5;
}
</style>
