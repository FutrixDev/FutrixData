<template>
  <section class="view active risk-rules-form-view">
    <div class="list-toolbar">
      <div>
        <h2>{{ pageTitle }}</h2>
      </div>
      <div class="console-actions">
        <button class="btn secondary" type="button" @click="router.push({ name: 'risk-rules' })">{{ tApp('common.cancel') }}</button>
        <button class="btn" type="button" @click="save">{{ tApp('riskRules.form.save') }}</button>
      </div>
    </div>

    <div class="risk-form-body">
      <div v-if="isBuiltinProbeEdit" class="risk-form-section">
        <div class="risk-form-section-title">
          <span class="section-number">1</span>
          {{ tApp('riskRules.form.builtinBehaviorTitle') }}
        </div>
        <div class="risk-preview">
          <div><strong>{{ builtinRuleTitleText }}</strong></div>
          <div style="margin-top: 8px;">{{ builtinRuleSummaryText }}</div>
          <div v-if="builtinRuleTriggerText" style="margin-top: 8px;">{{ tApp('riskRules.triggerLabel') }}{{ builtinRuleTriggerText }}</div>
        </div>
      </div>

      <!-- Section 1: Action -->
      <div v-if="!isBuiltinProbeEdit" class="risk-form-section">
        <div class="risk-form-section-title">
          <span class="section-number">1</span>
          {{ tApp('riskRules.form.sectionAction') }}
        </div>
        <div class="risk-action-grid">
          <div
            v-for="a in actionOptions"
            :key="a.value"
            class="risk-action-card"
            :class="{ selected: form.action === a.value }"
            @click="form.action = a.value"
          >
            <span class="risk-rule-action-dot" :class="'action-' + a.value" style="width: 12px; height: 12px;"></span>
            <span class="action-label">{{ tApp('riskRules.action.' + a.value) }}</span>
            <span class="action-desc">{{ tApp('riskRules.action.' + a.value + '.desc') }}</span>
          </div>
        </div>
      </div>

      <!-- Section 2: Scope -->
      <div v-if="!isBuiltinProbeEdit" class="risk-form-section">
        <div class="risk-form-section-title">
          <span class="section-number">2</span>
          {{ tApp('riskRules.form.sectionScope') }}
        </div>

        <div class="risk-field">
          <label>{{ tApp('riskRules.form.dsTypes') }}</label>
          <div class="risk-chip-grid">
            <button
              v-for="t in dsTypeOptions"
              :key="t.value"
              type="button"
              class="risk-chip"
              :class="{ selected: form.dsTypes.includes(t.value) }"
              @click="toggleDsType(t.value)"
            >{{ t.label }}<span v-if="dsTypeCount(t.value)" class="risk-chip-count">{{ dsTypeCount(t.value) }}</span></button>
          </div>
        </div>

        <!-- Datasource: custom dropdown -->
        <div class="risk-field">
          <label>{{ tApp('riskRules.form.datasource') }}</label>
          <div ref="dsDropdownRef" class="risk-dropdown">
            <button type="button" class="risk-dropdown-trigger" @click="dsDropdownOpen = !dsDropdownOpen">
              <span class="risk-dropdown-label">{{ selectedDsLabel }}</span>
              <span class="risk-dropdown-chevron"></span>
            </button>
            <div v-if="dsDropdownOpen" class="risk-dropdown-menu">
              <button type="button" class="risk-dropdown-option" :class="{ active: !form.datasourceId }" @click="selectDatasource('')">
                {{ tApp('riskRules.form.datasourceAny') }}
              </button>
              <button
                v-for="ds in filteredDatasources"
                :key="ds.id"
                type="button"
                class="risk-dropdown-option"
                :class="{ active: form.datasourceId === ds.id }"
                @click="selectDatasource(String(ds.id))"
              >
                {{ ds.name }} <span class="risk-dropdown-option-hint">{{ ds.type }}</span>
              </button>
            </div>
          </div>
        </div>

        <!-- Entity -->
        <div class="risk-field">
          <div class="risk-field-label-row">
            <label for="risk-rule-entity">{{ tApp('riskRules.form.entity') }}</label>
            <button
              v-if="filteredDatasources.length > 0"
              type="button"
              class="risk-entity-browse-link"
              @click="openEntityPicker"
            >{{ tApp('riskRules.form.entityFromDs') }}</button>
          </div>

          <!-- Selected entities as removable chips -->
          <div v-if="selectedEntities.length > 0" class="risk-selected-entities">
            <span v-for="ent in visibleChips" :key="ent" class="risk-entity-chip">
              {{ ent }}
              <button type="button" class="risk-entity-chip-remove" @click="toggleEntity(ent)">×</button>
            </span>
            <span v-if="hiddenChipCount > 0" class="risk-entity-chip overflow" :title="selectedEntities.slice(5).join(', ')">
              +{{ hiddenChipCount }}
            </span>
            <button v-if="selectedEntities.length > 1" type="button" class="risk-entity-chip-clear" @click="selectedEntities = []">
              {{ tApp('riskRules.form.clearAll') }}
            </button>
          </div>

          <!-- Manual text input -->
          <input
            id="risk-rule-entity"
            name="riskRuleEntity"
            v-model="form.entity"
            type="text"
            autocapitalize="off"
            autocorrect="off"
            spellcheck="false"
            :placeholder="tApp('riskRules.form.entityManualPlaceholder')"
            @keydown.enter.prevent="addManualEntity"
          />
        </div>
      </div>

      <!-- Entity Picker Dialog -->
      <div v-if="showEntityPicker" class="dialog-backdrop" @click.self="showEntityPicker = false">
        <div class="dialog-card risk-entity-dialog">
          <div class="risk-entity-dialog-header">
            <h3>{{ tApp('riskRules.form.entityPickTitle') }}</h3>
            <button type="button" class="btn ghost mini" @click="showEntityPicker = false">×</button>
          </div>

          <!-- Datasource selector inside dialog -->
          <div ref="entityDsDropdownRef" class="risk-dropdown" style="margin-bottom: 10px;">
            <button type="button" class="risk-dropdown-trigger" @click="entityDsDropdownOpen = !entityDsDropdownOpen">
              <span class="risk-dropdown-label">{{ entityPickDsLabel }}</span>
              <span class="risk-dropdown-chevron"></span>
            </button>
            <div v-if="entityDsDropdownOpen" class="risk-dropdown-menu">
              <button
                v-for="ds in filteredDatasources"
                :key="ds.id"
                type="button"
                class="risk-dropdown-option"
                :class="{ active: entityPickDs === String(ds.id) }"
                @click="selectEntityDs(String(ds.id))"
              >
                {{ ds.name }} <span class="risk-dropdown-option-hint">{{ ds.type }}</span>
              </button>
            </div>
          </div>

          <!-- Search + Select all -->
          <div v-if="entityList.length > 0" class="risk-entity-dialog-toolbar">
            <input
              id="risk-rule-entity-search"
              name="riskRuleEntitySearch"
              v-model="entitySearch"
              type="text"
              class="risk-entity-search"
              style="flex: 1;"
              :aria-label="tApp('riskRules.form.entitySearch')"
              autocapitalize="off"
              autocorrect="off"
              spellcheck="false"
              :placeholder="tApp('riskRules.form.entitySearch')"
            />
            <button type="button" class="btn secondary mini" @click="selectAllEntities">
              {{ allEntitiesSelected ? tApp('riskRules.form.deselectAll') : tApp('riskRules.form.selectAll') }}
            </button>
          </div>

          <!-- Entity list -->
          <div v-if="entityLoading" class="risk-entity-picker-empty">{{ tApp('riskRules.form.entityLoading') }}</div>
          <div v-else-if="entityList.length === 0 && entityPickDs" class="risk-entity-picker-empty">{{ tApp('riskRules.form.entityEmpty') }}</div>
          <div v-else-if="entityList.length > 0" class="risk-entity-picker" style="max-height: 320px;">
            <label
              v-for="ent in filteredEntityList"
              :key="ent"
              class="risk-entity-picker-item"
            >
              <input type="checkbox" :checked="selectedEntities.includes(ent)" @change="toggleEntity(ent)" />
              <span class="risk-entity-name">{{ ent }}</span>
            </label>
          </div>

          <!-- Dialog footer with count -->
          <div class="risk-entity-dialog-footer">
            <span class="risk-entity-dialog-count">{{ selectedEntities.length }} {{ tApp('riskRules.form.entitySelected') }}</span>
            <button type="button" class="btn small" @click="showEntityPicker = false">{{ tApp('riskRules.form.entityDone') }}</button>
          </div>
        </div>
      </div>

      <!-- Redis command picker dialog (full Redis catalog) -->
      <div v-if="showRedisCommandPicker" class="dialog-backdrop" @click.self="closeRedisCommandPicker">
        <div class="dialog-card risk-entity-dialog">
          <div class="risk-entity-dialog-header">
            <h3>{{ tApp('riskRules.form.redisPickTitle') }}</h3>
            <button type="button" class="btn ghost mini" @click="closeRedisCommandPicker">×</button>
          </div>
          <div class="risk-entity-dialog-toolbar">
            <input
              v-model="redisPickerSearch"
              type="text"
              class="risk-entity-search"
              style="flex: 1;"
              autocapitalize="off"
              autocorrect="off"
              spellcheck="false"
              :placeholder="tApp('riskRules.form.redisPickSearch')"
            />
            <span class="risk-entity-dialog-count">
              {{ redisPickerSelectedCount }} {{ tApp('riskRules.form.redisPickSelected') }}
            </span>
          </div>
          <div v-if="redisPickerLoading" class="risk-entity-picker-empty">{{ tApp('riskRules.form.redisPickLoading') }}</div>
          <div v-else-if="redisPickerError" class="risk-entity-picker-empty">{{ redisPickerError }}</div>
          <div v-else class="risk-redis-picker">
            <div v-for="group in filteredRedisGroups" :key="group.key" class="risk-redis-picker-group">
              <div class="risk-redis-picker-group-header">
                <span class="risk-redis-picker-group-name">{{ group.key }}</span>
                <button type="button" class="btn ghost mini" @click="toggleRedisGroupAll(group)">
                  {{ isRedisGroupFullySelected(group) ? tApp('riskRules.form.deselectAll') : tApp('riskRules.form.selectAll') }}
                </button>
              </div>
              <div class="risk-chip-grid">
                <button
                  v-for="cmd in group.commands"
                  :key="cmd.name"
                  type="button"
                  class="risk-chip"
                  :class="{ selected: redisPickerSelected.has(cmd.lower) }"
                  :title="cmd.summary"
                  @click="toggleRedisPickerCommand(cmd)"
                >{{ cmd.name }}</button>
              </div>
            </div>
          </div>
          <div class="risk-entity-dialog-footer">
            <button type="button" class="btn ghost mini" @click="clearRedisPickerSelection">
              {{ tApp('riskRules.form.redisPickClear') }}
            </button>
            <button type="button" class="btn small" @click="applyRedisPicker">
              {{ tApp('riskRules.form.redisPickApply') }}
            </button>
          </div>
        </div>
      </div>

      <!-- Section 3: Condition (dynamic based on dsTypes) -->
      <div v-if="!isBuiltinProbeEdit" class="risk-form-section">
        <div class="risk-form-section-title">
          <span class="section-number">3</span>
          {{ tApp('riskRules.form.sectionCondition') }}
        </div>

        <!-- SQL condition (PG / MySQL / D1 / DynamoDB) -->
        <div v-if="showSQLCondition">
          <div class="risk-field">
            <label>{{ tApp('riskRules.form.sqlCommands') }}</label>
            <div class="risk-chip-grid">
              <button
                v-for="cmd in sqlCommands"
                :key="cmd"
                type="button"
                class="risk-chip"
                :class="{ selected: form.commands.includes(cmd.toLowerCase()) }"
                @click="toggleCommand(cmd)"
              >{{ cmd }}</button>
            </div>
          </div>
          <div class="risk-field">
            <label>{{ tApp('riskRules.form.whereClause') }}</label>
            <div class="risk-chip-grid">
              <button type="button" class="risk-chip" :class="{ selected: form.hasWhere === null }" @click="form.hasWhere = null">
                {{ tApp('riskRules.form.whereAny') }}
              </button>
              <button type="button" class="risk-chip" :class="{ selected: form.hasWhere === true }" @click="form.hasWhere = true">
                {{ tApp('riskRules.form.whereRequired') }}
              </button>
              <button type="button" class="risk-chip" :class="{ selected: form.hasWhere === false }" @click="form.hasWhere = false">
                {{ tApp('riskRules.form.whereNone') }}
              </button>
            </div>
          </div>
        </div>

        <!-- Redis condition -->
        <div v-if="showRedisCondition">
          <div class="risk-field">
            <label>{{ tApp('riskRules.form.redisCategory') }}</label>
            <div class="risk-chip-grid">
              <button
                v-for="cat in redisCategories"
                :key="cat.key"
                type="button"
                class="risk-chip"
                :class="{ selected: isRedisCategorySelected(cat) }"
                @click="toggleRedisCategory(cat)"
              >{{ tApp('riskRules.redis.' + cat.key) }}</button>
              <button
                type="button"
                class="risk-chip"
                :class="{ selected: redisWildcardSelected }"
                @click="toggleRedisWildcard"
                :title="tApp('riskRules.form.redisWildcardHint')"
              >{{ tApp('riskRules.form.redisWildcard') }}</button>
            </div>
          </div>
          <div class="risk-field">
            <label for="risk-rule-redis-specific">{{ tApp('riskRules.form.redisSpecific') }}</label>
            <div class="risk-redis-specific-row">
              <input
                id="risk-rule-redis-specific"
                name="riskRuleRedisSpecific"
                v-model="form.redisSpecific"
                type="text"
                autocapitalize="off"
                autocorrect="off"
                spellcheck="false"
                :placeholder="tApp('riskRules.form.redisSpecificHint')"
              />
              <button
                type="button"
                class="btn secondary mini"
                @click="openRedisCommandPicker"
              >{{ tApp('riskRules.form.redisBrowseAll') }}</button>
            </div>
          </div>
          <div class="risk-field">
            <label for="risk-rule-key-pattern">{{ tApp('riskRules.form.keyPattern') }}</label>
            <input
              id="risk-rule-key-pattern"
              name="riskRuleKeyPattern"
              v-model="form.keyPattern"
              type="text"
              autocapitalize="off"
              autocorrect="off"
              spellcheck="false"
              :placeholder="tApp('riskRules.form.keyPatternHint')"
            />
          </div>
        </div>

        <!-- MongoDB condition -->
        <div v-if="showMongoCondition">
          <div class="risk-field">
            <label>{{ tApp('riskRules.form.mongoOps') }}</label>
            <div class="risk-chip-grid">
              <button
                v-for="op in mongoOps"
                :key="op"
                type="button"
                class="risk-chip"
                :class="{ selected: form.commands.includes(op) }"
                @click="toggleCommand(op)"
              >{{ op }}</button>
            </div>
          </div>
        </div>

        <!-- Elasticsearch condition -->
        <div v-if="showESCondition">
          <div class="risk-field">
            <label>{{ tApp('riskRules.form.esMethod') }}</label>
            <div class="risk-chip-grid">
              <button
                v-for="m in esMethods"
                :key="m"
                type="button"
                class="risk-chip"
                :class="{ selected: form.esMethods.includes(m) }"
                @click="toggleESMethod(m)"
              >{{ m }}</button>
            </div>
          </div>
          <div class="risk-field">
            <label for="risk-rule-es-path">{{ tApp('riskRules.form.esPath') }}</label>
            <input
              id="risk-rule-es-path"
              name="riskRuleEsPath"
              v-model="form.esPath"
              type="text"
              autocapitalize="off"
              autocorrect="off"
              spellcheck="false"
              :placeholder="tApp('riskRules.form.esPathHint')"
            />
          </div>
        </div>
      </div>

      <!-- Section 4: Thresholds (collapsible) -->
      <div v-if="showThresholds || isBuiltinProbeEdit" class="risk-form-section">
        <button
          type="button"
          class="risk-thresholds-toggle"
          @click="thresholdsOpen = !thresholdsOpen"
        >
          {{ thresholdsOpen ? '▼' : '▶' }}
          {{ isBuiltinProbeEdit ? tApp('riskRules.form.builtinThresholdsTitle') : tApp('riskRules.form.sectionThresholds') }}
        </button>
        <div v-if="thresholdsOpen" class="risk-thresholds-body">
          <div v-if="showThresholdField('maxExaminedRows') || showThresholdField('maxEstimatedJoinRows')" class="risk-field-row">
            <div class="risk-field">
              <template v-if="showThresholdField('maxExaminedRows')">
              <label for="risk-rule-max-examined-rows">{{ tApp('riskRules.form.maxExaminedRows') }}</label>
              <input id="risk-rule-max-examined-rows" name="riskRuleMaxExaminedRows" v-model.number="form.maxExaminedRows" type="number" placeholder="1000" />
              </template>
            </div>
            <div class="risk-field">
              <template v-if="showThresholdField('maxEstimatedJoinRows')">
              <label for="risk-rule-max-estimated-join-rows">{{ tApp('riskRules.form.maxEstimatedJoinRows') }}</label>
              <input id="risk-rule-max-estimated-join-rows" name="riskRuleMaxEstimatedJoinRows" v-model.number="form.maxEstimatedJoinRows" type="number" placeholder="10000" />
              </template>
            </div>
          </div>

          <div v-if="showThresholdField('allowSafeSeqScan')" class="risk-field" style="display: flex; align-items: center; gap: 10px; margin-bottom: 8px;">
            <div
              class="risk-toggle"
              :class="{ on: form.allowSafeSeqScan }"
              @click="form.allowSafeSeqScan = !form.allowSafeSeqScan"
            ></div>
            <div>
              <div style="font-size: 12.5px; font-weight: 600; color: var(--ink);">{{ tApp('riskRules.form.allowSafeSeqScan') }}</div>
              <div class="hint">{{ tApp('riskRules.form.allowSafeSeqScanHint') }}</div>
            </div>
          </div>

          <div v-if="form.allowSafeSeqScan && (showThresholdField('seqScanRowsThreshold') || showThresholdField('costThreshold'))" class="risk-field-row">
            <div class="risk-field">
              <template v-if="showThresholdField('seqScanRowsThreshold')">
              <label for="risk-rule-seq-scan-rows-threshold">{{ tApp('riskRules.form.seqScanRowsThreshold') }}</label>
              <input id="risk-rule-seq-scan-rows-threshold" name="riskRuleSeqScanRowsThreshold" v-model.number="form.seqScanRowsThreshold" type="number" placeholder="10000" />
              </template>
            </div>
            <div class="risk-field">
              <template v-if="showThresholdField('costThreshold')">
              <label for="risk-rule-cost-threshold">{{ tApp('riskRules.form.costThreshold') }}</label>
              <input id="risk-rule-cost-threshold" name="riskRuleCostThreshold" v-model.number="form.costThreshold" type="number" placeholder="1000" />
              </template>
            </div>
          </div>

          <div v-if="showThresholdField('maxJoinCount') || showThresholdField('maxFullScans')" class="risk-field-row">
            <div class="risk-field">
              <template v-if="showThresholdField('maxJoinCount')">
              <label for="risk-rule-max-join-count">{{ tApp('riskRules.form.maxJoinCount') }}</label>
              <input id="risk-rule-max-join-count" name="riskRuleMaxJoinCount" v-model.number="form.maxJoinCount" type="number" placeholder="4" />
              </template>
            </div>
            <div class="risk-field">
              <template v-if="showThresholdField('maxFullScans')">
              <label for="risk-rule-max-full-scans">{{ tApp('riskRules.form.maxFullScans') }}</label>
              <input id="risk-rule-max-full-scans" name="riskRuleMaxFullScans" v-model.number="form.maxFullScans" type="number" placeholder="1" />
              </template>
            </div>
          </div>

          <div v-if="showThresholdField('maxDynamoDBPages') || showThresholdField('maxDynamoDBEvaluatedItems')" class="risk-field-row">
            <div class="risk-field">
              <template v-if="showThresholdField('maxDynamoDBPages')">
              <label for="risk-rule-max-dynamodb-pages">{{ tApp('riskRules.form.maxDynamoDBPages') }}</label>
              <input id="risk-rule-max-dynamodb-pages" name="riskRuleMaxDynamoDBPages" v-model.number="form.maxDynamoDBPages" type="number" min="1" max="20" placeholder="20" autocapitalize="off" autocorrect="off" spellcheck="false" />
              </template>
            </div>
            <div class="risk-field">
              <template v-if="showThresholdField('maxDynamoDBEvaluatedItems')">
              <label for="risk-rule-max-dynamodb-evaluated-items">{{ tApp('riskRules.form.maxDynamoDBEvaluatedItems') }}</label>
              <input id="risk-rule-max-dynamodb-evaluated-items" name="riskRuleMaxDynamoDBEvaluatedItems" v-model.number="form.maxDynamoDBEvaluatedItems" type="number" min="1" max="5000" placeholder="5000" autocapitalize="off" autocorrect="off" spellcheck="false" />
              </template>
            </div>
          </div>
        </div>
      </div>

      <!-- Section 5: Description & Priority -->
      <div v-if="!isBuiltinProbeEdit" class="risk-form-section">
        <div class="risk-form-section-title">
          <span class="section-number">{{ showThresholds ? '5' : '4' }}</span>
          {{ tApp('riskRules.form.sectionInfo') }}
        </div>

        <div class="risk-field">
          <label for="risk-rule-description">{{ tApp('riskRules.form.ruleName') }}</label>
          <input
            id="risk-rule-description"
            name="riskRuleDescription"
            v-model="form.description"
            type="text"
            autocapitalize="off"
            autocorrect="off"
            spellcheck="false"
            :placeholder="tApp('riskRules.form.ruleNameHint')"
          />
        </div>

        <div class="risk-field">
          <label for="risk-rule-reason">{{ tApp('riskRules.form.reason') }}</label>
          <input
            id="risk-rule-reason"
            name="riskRuleReason"
            v-model="form.reason"
            type="text"
            autocapitalize="off"
            autocorrect="off"
            spellcheck="false"
            :placeholder="tApp('riskRules.form.reasonHint')"
          />
        </div>

        <div class="risk-field">
          <label>{{ tApp('riskRules.form.priority') }}</label>
          <div ref="priorityDropdownRef" class="risk-dropdown">
            <button type="button" class="risk-dropdown-trigger" @click="priorityDropdownOpen = !priorityDropdownOpen">
              <span class="risk-dropdown-label">{{ priorityLabel }}</span>
              <span class="risk-dropdown-chevron"></span>
            </button>
            <div v-if="priorityDropdownOpen" class="risk-dropdown-menu">
              <button
                v-for="p in priorityOptions"
                :key="p.value"
                type="button"
                class="risk-dropdown-option"
                :class="{ active: form.priorityLevel === p.value }"
                @click="form.priorityLevel = p.value; priorityDropdownOpen = false"
              >{{ p.label }}</button>
            </div>
          </div>
          <div class="hint">{{ tApp('riskRules.form.priorityHint') }}</div>
        </div>
      </div>

      <!-- Preview -->
      <div v-if="!isBuiltinProbeEdit" class="risk-form-section">
        <div class="risk-form-section-title">
          {{ tApp('riskRules.form.preview') }}
        </div>
        <div class="risk-preview" v-html="previewHTML"></div>
      </div>

      <!-- Form error -->
      <div v-if="formError" class="form-errors show" style="margin-bottom: 16px;">{{ formError }}</div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { tApp } from '@/modules/i18n/appI18n'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { canManageBuiltinRiskRules, canManageCustomRiskRules, builtinRiskRulesNotice, customRiskRulesNotice, resolvePlanLimitMessage } from '@/modules/plan/limits'
import { builtinRuleSummary, builtinRuleTitle, builtinRuleTrigger, editableProbeThresholdFields, isProbeBuiltinRule } from '@/modules/riskRules/builtinCatalog'
import {
  RiskEngineAddRule,
  RiskEngineListRules,
  RiskEngineUpdateRule,
  RiskEngineUpdateBuiltinProbeRuleThresholds,
  RiskEngineListUserRules,
  ListEntities,
} from '@wailsjs/go/main/App'
import { riskengine } from '@wailsjs/go/models'

const router = useRouter()
const route = useRoute()
const appStore = useAppStore()
const authStore = useAuthStore()
const canManageRiskRules = () => canManageCustomRiskRules(authStore.effectivePlan, { isAuthenticated: authStore.isAuthenticated })
const canManageBuiltinRules = () => canManageBuiltinRiskRules(authStore.effectivePlan, { isAuthenticated: authStore.isAuthenticated })

const isEdit = computed(() => route.name === 'risk-rules-edit')
const editId = computed(() => route.params.id as string)
const editKind = computed(() => {
  const raw = route.query?.kind
  return typeof raw === 'string' ? raw : ''
})
const currentRule = ref<riskengine.Rule | null>(null)
const isBuiltinProbeEdit = computed(() => isProbeBuiltinRule(currentRule.value))
const editableThresholds = computed(() => editableProbeThresholdFields(String(currentRule.value?.id || '')))
const showThresholdField = (field: string) => (
  !isBuiltinProbeEdit.value || editableThresholds.value.includes(field)
)
const pageTitle = computed(() => {
  if (isBuiltinProbeEdit.value && currentRule.value?.code) {
    return tApp('riskRules.form.editBuiltinTitle', { code: currentRule.value.code })
  }
  return isEdit.value ? tApp('route.riskRulesEdit') : tApp('route.riskRulesCreate')
})
const builtinRuleTitleText = computed(() => currentRule.value ? builtinRuleTitle(currentRule.value) : '')
const builtinRuleSummaryText = computed(() => currentRule.value ? builtinRuleSummary(currentRule.value) : '')
const builtinRuleTriggerText = computed(() => currentRule.value ? builtinRuleTrigger(currentRule.value) : '')

const formError = ref('')
const thresholdsOpen = ref(false)

// Custom dropdown state
const dsDropdownRef = ref<HTMLElement | null>(null)
const dsDropdownOpen = ref(false)
const entityDsDropdownRef = ref<HTMLElement | null>(null)
const entityDsDropdownOpen = ref(false)
const priorityDropdownRef = ref<HTMLElement | null>(null)
const priorityDropdownOpen = ref(false)
const showEntityPicker = ref(false)

const dsTypeOptions = [
  { value: 'postgresql', label: 'PostgreSQL' },
  { value: 'mysql', label: 'MySQL' },
  { value: 'd1', label: 'D1' },
  { value: 'mongodb', label: 'MongoDB' },
  { value: 'redis', label: 'Redis' },
  { value: 'redis_cluster', label: 'Redis Cluster' },
  { value: 'elasticsearch', label: 'Elasticsearch' },
  { value: 'dynamodb', label: 'DynamoDB' },
]

const actionOptions = [
  { value: 'block' },
  { value: 'warn' },
  { value: 'allow' },
]

const sqlCommands = ['SELECT', 'INSERT', 'UPDATE', 'DELETE', 'DROP', 'TRUNCATE', 'ALTER', 'CREATE', 'SHOW', 'DESCRIBE', 'EXPLAIN', 'GRANT', 'REVOKE', 'REPLACE']
const mongoOps = ['find', 'aggregate', 'count', 'deleteOne', 'deleteMany', 'updateOne', 'updateMany', 'findOneAndUpdate', 'insertOne', 'insertMany', 'drop', 'dropDatabase', 'createIndex', 'dropIndex', 'createCollection']
const esMethods = ['GET', 'POST', 'PUT', 'DELETE', 'HEAD', 'PATCH']

const redisCategories = [
  { key: 'write', commands: ['set', 'del', 'unlink', 'mset', 'setnx', 'setex', 'append', 'incr', 'decr', 'hset', 'hmset', 'hdel', 'lpush', 'rpush', 'lpop', 'rpop', 'sadd', 'srem', 'zadd', 'zrem', 'xadd', 'xdel', 'expire', 'rename', 'copy'] },
  { key: 'read', commands: ['get', 'mget', 'getrange', 'strlen', 'type', 'ttl', 'pttl', 'exists', 'hget', 'hmget', 'hexists', 'hlen', 'lindex', 'llen', 'scard', 'sismember', 'zcard', 'zscore', 'pfcount', 'info', 'ping'] },
  { key: 'scan', commands: ['keys', 'scan', 'sscan', 'hscan', 'zscan', 'hgetall', 'smembers', 'zrange', 'zrevrange', 'lrange', 'xrange', 'sort'] },
  { key: 'admin', commands: ['config', 'client', 'cluster', 'debug', 'slaveof', 'replicaof', 'failover', 'shutdown', 'flushall', 'flushdb'] },
  { key: 'script', commands: ['eval', 'evalsha', 'evalro', 'fcall', 'fcall_ro'] },
]
const redisCommandSet = new Set(redisCategories.flatMap(cat => cat.commands))

const normalizeCommandValue = (command: string) => command.trim().toLowerCase()

const uniqueCommands = (commands: string[]) => {
  const seen = new Set<string>()
  const result: string[] = []
  for (const command of commands) {
    const normalized = normalizeCommandValue(command)
    if (!normalized || seen.has(normalized)) continue
    seen.add(normalized)
    result.push(normalized)
  }
  return result
}

const redisSpecificDisplayValue = (commands: string[]) => commands.map(c => c.toUpperCase()).join(', ')

const splitRedisCommandsForForm = (commands: string[], includeUnknownSpecific = true) => {
  const normalized = uniqueCommands(commands)
  const redisCommands = normalized.filter(command => redisCommandSet.has(command))
  const nonRedisCommands = normalized.filter(command => !redisCommandSet.has(command))
  const categoryCommands = new Set<string>()
  for (const cat of redisCategories) {
    if (cat.commands.every(command => redisCommands.includes(command))) {
      for (const command of cat.commands) categoryCommands.add(command)
    }
  }
  return {
    categoryCommands: redisCommands.filter(command => categoryCommands.has(command)),
    specificCommands: [
      ...redisCommands.filter(command => !categoryCommands.has(command)),
      ...(includeUnknownSpecific ? nonRedisCommands : []),
    ],
    nonRedisCommands: includeUnknownSpecific ? [] : nonRedisCommands,
  }
}

const form = reactive({
  action: 'warn' as string,
  dsTypes: [] as string[],
  datasourceId: '',
  entity: '',
  commands: [] as string[],
  hasWhere: null as boolean | null,
  esMethods: [] as string[],
  esPath: '',
  keyPattern: '',
  redisSpecific: '',
  description: '',
  reason: '',
  priorityLevel: 'medium',
  maxExaminedRows: null as number | null,
  seqScanRowsThreshold: null as number | null,
  costThreshold: null as number | null,
  allowSafeSeqScan: true,
  maxJoinCount: null as number | null,
  maxFullScans: null as number | null,
  maxEstimatedJoinRows: null as number | null,
  maxDynamoDBPages: null as number | null,
  maxDynamoDBEvaluatedItems: null as number | null,
})

// Priority options
const priorityOptions = computed(() => [
  { value: 'low', label: `${tApp('riskRules.form.priorityLow')} (10)` },
  { value: 'medium', label: `${tApp('riskRules.form.priorityMedium')} (50)` },
  { value: 'high', label: `${tApp('riskRules.form.priorityHigh')} (90)` },
])

const priorityLabel = computed(() => {
  const found = priorityOptions.value.find(p => p.value === form.priorityLevel)
  return found ? found.label : form.priorityLevel
})

// Datasource dropdown helpers
const selectedDsLabel = computed(() => {
  if (!form.datasourceId) return tApp('riskRules.form.datasourceAny')
  const ds = appStore.datasources.find(d => String(d.id) === form.datasourceId)
  return ds ? `${ds.name} (${ds.type})` : form.datasourceId
})

const dsTypeCount = (type: string) => {
  return appStore.datasources.filter(d => String(d.type) === type).length
}

const selectDatasource = (id: string) => {
  form.datasourceId = id
  dsDropdownOpen.value = false
  // Auto-select dsType when a specific datasource is chosen
  if (id) {
    const ds = appStore.datasources.find(d => String(d.id) === id)
    if (ds && !form.dsTypes.includes(String(ds.type))) {
      form.dsTypes.push(String(ds.type))
    }
  }
}

// Entity picker state (dialog-based)
const entityPickDs = ref('')
const entityList = ref<string[]>([])
const entityLoading = ref(false)
const selectedEntities = ref<string[]>([])
const entitySearch = ref('')

const visibleChips = computed(() => selectedEntities.value.slice(0, 5))
const hiddenChipCount = computed(() => Math.max(0, selectedEntities.value.length - 5))

const filteredEntityList = computed(() => {
  if (!entitySearch.value) return entityList.value
  const q = entitySearch.value.toLowerCase()
  return entityList.value.filter(e => e.toLowerCase().includes(q))
})

const allEntitiesSelected = computed(() =>
  entityList.value.length > 0 && entityList.value.every(e => selectedEntities.value.includes(e))
)

const entityPickDsLabel = computed(() => {
  if (!entityPickDs.value) return `-- ${tApp('riskRules.form.datasource')} --`
  const ds = appStore.datasources.find(d => String(d.id) === entityPickDs.value)
  return ds ? ds.name : entityPickDs.value
})

const addManualEntity = () => {
  const val = form.entity.trim()
  if (val && !selectedEntities.value.includes(val)) {
    selectedEntities.value.push(val)
  }
  form.entity = ''
}

const toggleEntity = (ent: string) => {
  const idx = selectedEntities.value.indexOf(ent)
  if (idx >= 0) selectedEntities.value.splice(idx, 1)
  else selectedEntities.value.push(ent)
}

const selectAllEntities = () => {
  if (allEntitiesSelected.value) {
    selectedEntities.value = []
  } else {
    selectedEntities.value = [...entityList.value]
  }
}

const openEntityPicker = () => {
  showEntityPicker.value = true
  // Auto-select the form's datasource if set
  if (form.datasourceId && !entityPickDs.value) {
    entityPickDs.value = form.datasourceId
    loadEntities()
  } else if (filteredDatasources.value.length === 1 && !entityPickDs.value) {
    entityPickDs.value = String(filteredDatasources.value[0].id)
    loadEntities()
  }
}

const selectEntityDs = (id: string) => {
  entityPickDs.value = id
  entityDsDropdownOpen.value = false
  loadEntities()
}

const loadEntities = async () => {
  if (!entityPickDs.value) {
    entityList.value = []
    return
  }
  entityLoading.value = true
  try {
    const result = await ListEntities(entityPickDs.value, '', '', '', false)
    entityList.value = Array.isArray(result) ? result : []
  } catch {
    entityList.value = []
  } finally {
    entityLoading.value = false
  }
}

// Click outside handler for dropdowns
const onDocClick = (e: PointerEvent) => {
  const target = e.target as Node
  if (dsDropdownRef.value && !dsDropdownRef.value.contains(target)) dsDropdownOpen.value = false
  if (entityDsDropdownRef.value && !entityDsDropdownRef.value.contains(target)) entityDsDropdownOpen.value = false
  if (priorityDropdownRef.value && !priorityDropdownRef.value.contains(target)) priorityDropdownOpen.value = false
}

// Condition visibility based on selected dsTypes
const hasDsType = (...types: string[]) => form.dsTypes.some(t => types.includes(t))
const showSQLCondition = computed(() => hasDsType('postgresql', 'mysql', 'd1', 'dynamodb'))
const showRedisCondition = computed(() => hasDsType('redis', 'redis_cluster'))
const showMongoCondition = computed(() => hasDsType('mongodb'))
const showESCondition = computed(() => hasDsType('elasticsearch'))
const showThresholds = computed(() => hasDsType('postgresql', 'mysql', 'd1', 'mongodb', 'dynamodb'))

const allSelectedCommands = computed(() => {
  const commands = [...form.commands]
  if (form.redisSpecific) {
    commands.push(...form.redisSpecific.split(','))
  }
  return uniqueCommands(commands)
})

const filteredDatasources = computed(() => {
  if (form.dsTypes.length === 0) return appStore.datasources
  return appStore.datasources.filter(ds => form.dsTypes.includes(String(ds.type)))
})

const toggleDsType = (t: string) => {
  const idx = form.dsTypes.indexOf(t)
  if (idx >= 0) form.dsTypes.splice(idx, 1)
  else form.dsTypes.push(t)
}

const toggleCommand = (cmd: string) => {
  const lower = cmd.toLowerCase()
  const idx = form.commands.findIndex(c => c.toLowerCase() === lower)
  if (idx >= 0) form.commands.splice(idx, 1)
  else form.commands.push(lower)
}

const toggleESMethod = (m: string) => {
  const idx = form.esMethods.indexOf(m)
  if (idx >= 0) form.esMethods.splice(idx, 1)
  else form.esMethods.push(m)
}

const isRedisCategorySelected = (cat: { commands: string[] }) => {
  return cat.commands.every(c => form.commands.includes(c))
}

const toggleRedisCategory = (cat: { commands: string[] }) => {
  if (isRedisCategorySelected(cat)) {
    form.commands = form.commands.filter(c => !cat.commands.includes(c))
  } else {
    const toAdd = cat.commands.filter(c => !form.commands.includes(c))
    form.commands.push(...toAdd)
  }
}

const redisSpecificTokens = (raw: string) =>
  raw.split(',').map(s => s.trim()).filter(s => s.length > 0)

const redisWildcardSelected = computed(() => {
  if (form.commands.includes('*')) return true
  return redisSpecificTokens(form.redisSpecific).some(t => t === '*')
})

const toggleRedisWildcard = () => {
  if (redisWildcardSelected.value) {
    form.commands = form.commands.filter(c => c !== '*')
    form.redisSpecific = redisSpecificTokens(form.redisSpecific)
      .filter(t => t !== '*')
      .join(', ')
    return
  }
  if (!form.commands.includes('*')) form.commands.push('*')
}

type RedisCatalogEntry = { name: string; lower: string; summary: string; group: string }
type RedisCatalogGroup = { key: string; commands: RedisCatalogEntry[] }

const showRedisCommandPicker = ref(false)
const redisPickerLoading = ref(false)
const redisPickerError = ref('')
const redisPickerSearch = ref('')
const redisCatalog = ref<RedisCatalogEntry[] | null>(null)
const redisPickerSelected = ref<Set<string>>(new Set())

const loadRedisCommandCatalog = async () => {
  if (redisCatalog.value) return
  redisPickerLoading.value = true
  redisPickerError.value = ''
  try {
    const mod: any = await import('@/modules/redis/commands.json')
    const raw = (mod?.default?.commands ?? mod?.commands ?? {}) as Record<string, any>
    const entries: RedisCatalogEntry[] = Object.keys(raw).map(name => ({
      name,
      lower: name.toLowerCase(),
      summary: typeof raw[name]?.summary === 'string' ? raw[name].summary : '',
      group: typeof raw[name]?.group === 'string' ? raw[name].group : 'other',
    }))
    entries.sort((a, b) => a.name.localeCompare(b.name))
    redisCatalog.value = entries
  } catch (err) {
    redisPickerError.value = err instanceof Error ? err.message : String(err)
  } finally {
    redisPickerLoading.value = false
  }
}

const openRedisCommandPicker = async () => {
  showRedisCommandPicker.value = true
  redisPickerSearch.value = ''
  redisPickerSelected.value = new Set(redisSpecificTokens(form.redisSpecific).map(t => t.toLowerCase()))
  await loadRedisCommandCatalog()
}

const closeRedisCommandPicker = () => {
  showRedisCommandPicker.value = false
}

const toggleRedisPickerCommand = (cmd: RedisCatalogEntry) => {
  const next = new Set(redisPickerSelected.value)
  if (next.has(cmd.lower)) next.delete(cmd.lower)
  else next.add(cmd.lower)
  redisPickerSelected.value = next
}

const clearRedisPickerSelection = () => {
  redisPickerSelected.value = new Set()
}

const filteredRedisGroups = computed<RedisCatalogGroup[]>(() => {
  const entries = redisCatalog.value || []
  const term = redisPickerSearch.value.trim().toLowerCase()
  const filtered = term ? entries.filter(e => e.lower.includes(term)) : entries
  const buckets = new Map<string, RedisCatalogEntry[]>()
  for (const e of filtered) {
    if (!buckets.has(e.group)) buckets.set(e.group, [])
    buckets.get(e.group)!.push(e)
  }
  const groups: RedisCatalogGroup[] = []
  for (const [key, commands] of buckets) groups.push({ key, commands })
  groups.sort((a, b) => a.key.localeCompare(b.key))
  return groups
})

const redisPickerSelectedCount = computed(() => redisPickerSelected.value.size)

const isRedisGroupFullySelected = (group: RedisCatalogGroup) =>
  group.commands.length > 0 && group.commands.every(c => redisPickerSelected.value.has(c.lower))

const toggleRedisGroupAll = (group: RedisCatalogGroup) => {
  const next = new Set(redisPickerSelected.value)
  if (isRedisGroupFullySelected(group)) {
    for (const c of group.commands) next.delete(c.lower)
  } else {
    for (const c of group.commands) next.add(c.lower)
  }
  redisPickerSelected.value = next
}

const applyRedisPicker = () => {
  const tokens = Array.from(redisPickerSelected.value).sort()
  form.redisSpecific = tokens.map(t => t.toUpperCase()).join(', ')
  showRedisCommandPicker.value = false
}

// Preview
const previewHTML = computed(() => {
  const action = `<strong>${tApp('riskRules.action.' + form.action)}</strong>`
  const commands = allSelectedCommands.value.length > 0
    ? `<strong>${allSelectedCommands.value.join(', ').toUpperCase()}</strong>`
    : form.esMethods.length > 0
      ? `<strong>${form.esMethods.join(', ')}</strong>`
      : '<strong>all</strong>'

  let entity = '<strong>all entities</strong>'
  if (selectedEntities.value.length > 0) {
    entity = `<strong>${selectedEntities.value.join(', ')}</strong>`
  } else if (form.entity) {
    entity = `<strong>${form.entity}</strong>`
  }

  const dsTypes = form.dsTypes.length > 0
    ? `<strong>${form.dsTypes.map(t => dsTypeOptions.find(o => o.value === t)?.label || t).join(', ')}</strong>`
    : '<strong>all datasources</strong>'

  return tApp('riskRules.form.previewText')
    .replace('{action}', action)
    .replace('{commands}', commands)
    .replace('{entity}', entity)
    .replace('{dsTypes}', dsTypes)
})

// Build Rule from form
const buildRule = (): riskengine.Rule => {
  const priorityMap: Record<string, number> = { low: 10, medium: 50, high: 90 }
  const priority = priorityMap[form.priorityLevel] || 50

  const scope = new riskengine.RuleScope({
    dsTypes: form.dsTypes.length > 0 ? form.dsTypes : undefined,
    datasourceId: form.datasourceId || undefined,
    entity: undefined as string | undefined,
    entityPattern: undefined as string | undefined,
    keyPattern: form.keyPattern || undefined,
  })

  // Entity: single entity or pattern for multiple
  if (selectedEntities.value.length > 0) {
    if (selectedEntities.value.length === 1) {
      scope.entity = selectedEntities.value[0]
    } else {
      // Multiple entities: use regex pattern
      const escaped = selectedEntities.value.map(e => e.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))
      scope.entityPattern = `(?i)^(${escaped.join('|')})$`
    }
  } else if (form.entity) {
    scope.entity = form.entity
  }

  // Build condition
  const allCommands = allSelectedCommands.value

  const when = new riskengine.RuleCondition({
    command: allCommands.length > 0 ? allCommands : undefined,
    hasWhere: form.hasWhere,
    httpMethod: form.esMethods.length > 0 ? form.esMethods : undefined,
    pathPattern: form.esPath ? `(?i)${form.esPath.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}` : undefined,
  })

  // Build thresholds
  let thresholds: riskengine.RuleThresholds | undefined
  if (showThresholds.value) {
    const t = new riskengine.RuleThresholds({})
    let hasAny = false
    if (form.maxExaminedRows != null) { t.maxExaminedRows = form.maxExaminedRows; hasAny = true }
    if (form.seqScanRowsThreshold != null) { t.seqScanRowsThreshold = form.seqScanRowsThreshold; hasAny = true }
    if (form.costThreshold != null) { t.costThreshold = form.costThreshold; hasAny = true }
    if (!form.allowSafeSeqScan) { t.allowSafeSeqScan = false; hasAny = true }
    if (form.maxJoinCount != null) { t.maxJoinCount = form.maxJoinCount; hasAny = true }
    if (form.maxFullScans != null) { t.maxFullScans = form.maxFullScans; hasAny = true }
    if (form.maxEstimatedJoinRows != null) { t.maxEstimatedJoinRows = form.maxEstimatedJoinRows; hasAny = true }
    if (form.maxDynamoDBPages != null) { t.maxDynamoDBPages = form.maxDynamoDBPages; hasAny = true }
    if (form.maxDynamoDBEvaluatedItems != null) { t.maxDynamoDBEvaluatedItems = form.maxDynamoDBEvaluatedItems; hasAny = true }
    if (hasAny) thresholds = t
  }

  const id = isEdit.value ? editId.value : 'user-' + Date.now() + '-' + Math.random().toString(36).slice(2, 6)

  return new riskengine.Rule({
    id,
    description: form.description,
    scope,
    enabled: true,
    priority,
    action: form.action,
    reason: form.reason,
    when,
    thresholds,
    builtin: false,
  })
}

const buildBuiltinProbeThresholds = (): riskengine.RuleThresholds => {
  const thresholds = new riskengine.RuleThresholds({})
  const hasNumericThreshold = (value: unknown): value is number => typeof value === 'number' && Number.isFinite(value)
  if (showThresholdField('maxExaminedRows') && hasNumericThreshold(form.maxExaminedRows)) thresholds.maxExaminedRows = form.maxExaminedRows
  if (showThresholdField('seqScanRowsThreshold') && hasNumericThreshold(form.seqScanRowsThreshold)) thresholds.seqScanRowsThreshold = form.seqScanRowsThreshold
  if (showThresholdField('costThreshold') && hasNumericThreshold(form.costThreshold)) thresholds.costThreshold = form.costThreshold
  if (showThresholdField('allowSafeSeqScan') && !form.allowSafeSeqScan) thresholds.allowSafeSeqScan = false
  if (showThresholdField('maxJoinCount') && hasNumericThreshold(form.maxJoinCount)) thresholds.maxJoinCount = form.maxJoinCount
  if (showThresholdField('maxFullScans') && hasNumericThreshold(form.maxFullScans)) thresholds.maxFullScans = form.maxFullScans
  if (showThresholdField('maxEstimatedJoinRows') && hasNumericThreshold(form.maxEstimatedJoinRows)) thresholds.maxEstimatedJoinRows = form.maxEstimatedJoinRows
  if (showThresholdField('maxDynamoDBPages') && hasNumericThreshold(form.maxDynamoDBPages)) thresholds.maxDynamoDBPages = form.maxDynamoDBPages
  if (showThresholdField('maxDynamoDBEvaluatedItems') && hasNumericThreshold(form.maxDynamoDBEvaluatedItems)) thresholds.maxDynamoDBEvaluatedItems = form.maxDynamoDBEvaluatedItems
  return thresholds
}

const save = async () => {
  formError.value = ''
  const canManage = isBuiltinProbeEdit.value ? canManageBuiltinRules() : canManageRiskRules()
  if (!canManage) {
    const message = isBuiltinProbeEdit.value
      ? builtinRiskRulesNotice(authStore.effectivePlan, { isAuthenticated: authStore.isAuthenticated })
      : customRiskRulesNotice(authStore.effectivePlan, { isAuthenticated: authStore.isAuthenticated })
    formError.value = message
    appStore.setNotice(message, 'error')
    return
  }
  if (!isBuiltinProbeEdit.value && !form.description) {
    formError.value = tApp('validation.nameRequired')
    return
  }
  try {
    if (isBuiltinProbeEdit.value) {
      await RiskEngineUpdateBuiltinProbeRuleThresholds(editId.value, buildBuiltinProbeThresholds())
    } else {
      const rule = buildRule()
      if (isEdit.value) {
        await RiskEngineUpdateRule(editId.value, rule)
      } else {
        await RiskEngineAddRule(rule)
      }
    }
    router.push({ name: 'risk-rules' })
  } catch (e: any) {
    const message = resolvePlanLimitMessage(e, authStore.effectivePlan)
    if (message) {
      formError.value = message
      appStore.setNotice(message, 'error')
      return
    }
    formError.value = String(e?.message || e || tApp('riskRules.form.saveError'))
  }
}

// Load existing rule for edit
const loadRule = async () => {
  if (!isEdit.value) return
  try {
    const rules = await RiskEngineListRules() || []
    const candidates = rules.filter(r => r.id === editId.value)
    const preferBuiltinProbe = editableProbeThresholdFields(editId.value).length > 0
    const rule = (
      (editKind.value === 'builtin' ? candidates.find(r => isProbeBuiltinRule(r)) : null) ||
      (editKind.value === 'custom' ? candidates.find(r => !r.builtin) : null) ||
      (preferBuiltinProbe ? candidates.find(r => isProbeBuiltinRule(r)) : null) ||
      candidates[0]
    )
    if (!rule) return
    currentRule.value = rule
    if (rule.builtin && !isProbeBuiltinRule(rule)) {
      formError.value = tApp('riskRules.form.builtinReadonly')
      appStore.setNotice(formError.value, 'error')
      router.push({ name: 'risk-rules' })
      return
    }

    form.action = (rule.action === 'require_approval' ? 'warn' : rule.action) || 'warn'
    const loadedDsTypes = rule.scope?.dsTypes || []
    form.dsTypes = loadedDsTypes
    form.datasourceId = rule.scope?.datasourceId || ''
    form.entity = rule.scope?.entity || ''
    form.keyPattern = rule.scope?.keyPattern || ''
    form.description = rule.description || ''
    form.reason = rule.reason || ''

    // Priority
    if (rule.priority >= 80) form.priorityLevel = 'high'
    else if (rule.priority >= 30) form.priorityLevel = 'medium'
    else form.priorityLevel = 'low'

    // Condition
    const loadedCommands = (rule.when?.command || []).map(c => c.toLowerCase())
    if (loadedDsTypes.some(t => t === 'redis' || t === 'redis_cluster')) {
      const hasNonRedisDsType = loadedDsTypes.some(t => t !== 'redis' && t !== 'redis_cluster')
      const split = splitRedisCommandsForForm(loadedCommands, !hasNonRedisDsType)
      form.commands = uniqueCommands([...split.nonRedisCommands, ...split.categoryCommands])
      form.redisSpecific = redisSpecificDisplayValue(split.specificCommands)
    } else {
      form.commands = loadedCommands
      form.redisSpecific = ''
    }
    form.hasWhere = rule.when?.hasWhere ?? null
    form.esMethods = rule.when?.httpMethod || []
    // Extract esPath from pattern (remove (?i) prefix and escaping)
    if (rule.when?.pathPattern) {
      form.esPath = rule.when.pathPattern.replace(/^\(\?i\)/, '').replace(/\\\./g, '.')
    }

    // Thresholds
    if (rule.thresholds) {
      form.maxExaminedRows = rule.thresholds.maxExaminedRows ?? null
      form.seqScanRowsThreshold = rule.thresholds.seqScanRowsThreshold ?? null
      form.costThreshold = rule.thresholds.costThreshold ?? null
      form.allowSafeSeqScan = rule.thresholds.allowSafeSeqScan !== false
      form.maxJoinCount = rule.thresholds.maxJoinCount ?? null
      form.maxFullScans = rule.thresholds.maxFullScans ?? null
      form.maxEstimatedJoinRows = rule.thresholds.maxEstimatedJoinRows ?? null
      form.maxDynamoDBPages = rule.thresholds.maxDynamoDBPages ?? null
      form.maxDynamoDBEvaluatedItems = rule.thresholds.maxDynamoDBEvaluatedItems ?? null
      if (
        rule.thresholds.maxExaminedRows != null ||
        rule.thresholds.seqScanRowsThreshold != null ||
        rule.thresholds.costThreshold != null ||
        rule.thresholds.maxJoinCount != null ||
        rule.thresholds.maxFullScans != null ||
        rule.thresholds.maxEstimatedJoinRows != null ||
        rule.thresholds.maxDynamoDBPages != null ||
        rule.thresholds.maxDynamoDBEvaluatedItems != null ||
        rule.thresholds.allowSafeSeqScan === false
      ) {
        thresholdsOpen.value = true
      }
    }

    // If entity pattern set, extract entity names or fall back to text
    if (rule.scope?.entityPattern) {
      const match = rule.scope.entityPattern.match(/^\(\?i\)\^\((.+)\)\$$/)
      if (match) {
        selectedEntities.value = match[1].split('|').map(n => n.replace(/\\(.)/g, '$1'))
      } else {
        form.entity = rule.scope.entityPattern
      }
    }
    if (isBuiltinProbeEdit.value) {
      thresholdsOpen.value = true
    }
  } catch { /* ignore */ }
}

const resetForm = () => {
  currentRule.value = null
  form.action = 'warn'
  form.dsTypes = []
  form.datasourceId = ''
  form.entity = ''
  form.commands = []
  form.hasWhere = null
  form.esMethods = []
  form.esPath = ''
  form.keyPattern = ''
  form.redisSpecific = ''
  form.description = ''
  form.reason = ''
  form.priorityLevel = 'medium'
  form.maxExaminedRows = null
  form.seqScanRowsThreshold = null
  form.costThreshold = null
  form.allowSafeSeqScan = true
  form.maxJoinCount = null
  form.maxFullScans = null
  form.maxEstimatedJoinRows = null
  form.maxDynamoDBPages = null
  form.maxDynamoDBEvaluatedItems = null
  selectedEntities.value = []
  entityPickDs.value = ''
  entityList.value = []
  entitySearch.value = ''
  thresholdsOpen.value = false
  formError.value = ''
}

// Reset form when switching between edit↔create (same component, different route)
watch(() => [route.params.id, route.query?.kind], async () => {
  resetForm()
  await loadRule()
})

onMounted(async () => {
  document.addEventListener('pointerdown', onDocClick)
  await loadRule()
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onDocClick)
})
</script>
