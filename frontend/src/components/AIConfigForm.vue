<template>
  <div class="ai-panel ai-form-panel" :class="{ show: visible, inline }" id="ai-config-form-panel">
    <div v-if="!inline" class="ai-panel-resizer"></div>
    <div class="ai-panel-shell">
      <div class="ai-panel-header">
        <div class="ai-panel-title">
          <h3 id="aiconfig-form-title">{{ title }}</h3>
          <p class="ai-panel-subtitle">{{ tApp('ai.form.subtitle') }}</p>
        </div>
        <button class="btn ghost" type="button" id="aiconfig-form-cancel" @click="$emit('close')">&times;</button>
      </div>
      <div class="ai-form-core">
        <div v-if="errors.length" class="form-errors show" id="aiconfig-form-errors">
          <div v-for="(err, idx) in errors" :key="idx">{{ err }}</div>
        </div>
        <div class="ai-form-grid">
          <div class="ai-field span-2">
            <label for="ai-name">{{ tApp('ai.form.configurationName') }}</label>
            <input id="ai-name" v-model="form.name" :placeholder="tApp('ai.form.namePlaceholder')" />
          </div>
          <div class="ai-field">
            <label for="ai-provider">{{ tApp('ai.form.provider') }}</label>
            <select id="ai-provider" v-model="form.provider" @change="applyProviderDefaults">
              <option v-for="(info, key) in providers" :key="key" :value="key">
                {{ info.name }}
              </option>
            </select>
          </div>
          <div class="ai-field">
            <label for="ai-model">{{ tApp('ai.form.model') }}</label>
            <select id="ai-model" v-model="form.model">
              <option v-for="model in models" :key="model" :value="model">{{ model }}</option>
              <option value="">{{ tApp('common.custom') }}</option>
            </select>
            <input
              v-if="form.model === ''"
              id="ai-model-custom"
              v-model="form.modelCustom"
              :placeholder="tApp('ai.form.customModelPlaceholder')"
              autocapitalize="off"
              autocomplete="off"
              autocorrect="off"
              spellcheck="false"
              style="margin-top:8px;"
            />
          </div>
          <div class="ai-field">
            <label for="ai-max-tokens">{{ tApp('ai.form.maxTokens') }}</label>
            <input
              id="ai-max-tokens"
              v-model="form.maxTokens"
              inputmode="numeric"
              :placeholder="tApp('ai.form.maxTokensPlaceholder')"
            />
            <p class="ai-field-hint">{{ tApp('common.optionalDefaultHint') }}</p>
          </div>
          <div class="ai-field span-2" v-if="form.provider === 'custom'">
            <label for="ai-baseurl">{{ tApp('ai.form.apiBaseUrl') }}</label>
            <input id="ai-baseurl" v-model="form.baseUrl" :placeholder="tApp('ai.form.baseUrlPlaceholder')" />
          </div>
          <div class="ai-field span-2">
            <label for="ai-apikey">{{ tApp('ai.form.apiKey') }}</label>
            <div class="ai-input-with-toggle">
              <input id="ai-apikey" v-model="form.apiKey" :type="apiKeyInputType" :placeholder="tApp('ai.form.apiKeyPlaceholder')" />
              <button
                class="ai-visibility-toggle"
                type="button"
                :data-visible="apiKeyVisible"
                @click="toggleApiKey"
                :aria-label="apiKeyVisible ? tApp('ai.form.hideApiKey') : tApp('ai.form.showApiKey')"
                :title="apiKeyVisible ? tApp('ai.form.hideApiKey') : tApp('ai.form.showApiKey')"
              >
                <span aria-hidden="true">{{ apiKeyVisible ? tApp('common.hide') : tApp('common.show') }}</span>
              </button>
            </div>
          </div>
        </div>
        <p class="ai-form-note">{{ tApp('ai.form.customProviderNote') }} <code>/chat/completions</code> API.</p>
      </div>
      <div class="ai-form-actions">
        <div class="ai-form-status" id="aiconfig-form-status">
          <span v-if="statusText" class="status" :class="statusClass">{{ statusText }}</span>
          <span v-if="statusDetail" class="status-detail">{{ statusDetail }}</span>
        </div>
        <button class="btn ghost" type="button" id="aiconfig-form-test" @click="test">{{ tApp('ai.form.testConnection') }}</button>
        <button class="btn" type="button" id="aiconfig-form-save" @click="save">{{ tApp('common.save') }}</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { api } from '@/services/api'
import { useAppStore } from '@/stores/app'
import type { ProviderInfo } from '@/types'
import { tApp } from '@/modules/i18n/appI18n'

const props = defineProps<{ visible: boolean; mode: 'create' | 'edit'; configId?: string | null; inline?: boolean }>()
const emit = defineEmits<{ (e: 'close'): void }>()

const store = useAppStore()
const inline = computed(() => Boolean(props.inline))
const providers = ref<Record<string, ProviderInfo>>({})
const apiKeyVisible = ref(false)
const maskedApiKey = ref('')
const revealedApiKey = ref('')
const errors = ref<string[]>([])
const statusText = ref('')
const statusDetail = ref('')
const statusClass = ref('')
const lastTestConnected = ref(false)
const lastTestFingerprint = ref('')

const form = reactive({
  name: '',
  provider: 'openai',
  baseUrl: '',
  apiKey: '',
  model: '',
  modelCustom: '',
  maxTokens: '',
  options: {} as Record<string, any>,
})

const title = computed(() => (props.mode === 'edit' ? tApp('ai.form.titleEdit') : tApp('ai.form.titleCreate')))

const models = computed(() => {
  const providerInfo = providers.value[form.provider]
  if (!providerInfo) return []
  return providerInfo.models || []
})

const apiKeyIsMaskedPlaceholder = computed(() => {
  if (props.mode !== 'edit' || !props.configId) return false
  if (!maskedApiKey.value) return false
  return form.apiKey === maskedApiKey.value
})

const apiKeyInputType = computed(() => {
  if (apiKeyVisible.value) return 'text'
  if (apiKeyIsMaskedPlaceholder.value) return 'text'
  return 'password'
})

const resolveModelSelection = (provider: string, model: string) => {
  const trimmed = String(model || '').trim()
  if (!trimmed) return { model: '', modelCustom: '' }
  const knownModels = providers.value[provider]?.models || []
  if (knownModels.includes(trimmed)) return { model: trimmed, modelCustom: '' }
  return { model: '', modelCustom: trimmed }
}

const applyProviderDefaults = () => {
  const info = providers.value[form.provider]
  if (!info) return
  form.baseUrl = info.baseUrl
  form.model = info.defaultModel || ''
  form.modelCustom = ''
}

const fillForm = async () => {
  errors.value = []
  statusText.value = ''
  statusDetail.value = ''
  statusClass.value = ''
  lastTestConnected.value = false
  lastTestFingerprint.value = ''
  apiKeyVisible.value = false
  maskedApiKey.value = ''
  revealedApiKey.value = ''

  if (props.mode === 'edit' && props.configId) {
    const cfg = store.aiConfigs.find((item) => item.id === props.configId)
    if (cfg) {
      form.name = cfg.name || ''
      form.provider = cfg.provider || 'openai'
      form.baseUrl = cfg.baseUrl || ''
      form.apiKey = cfg.apiKey || ''
      maskedApiKey.value = form.apiKey
      const resolvedModel = resolveModelSelection(form.provider, cfg.model || '')
      form.model = resolvedModel.model
      form.modelCustom = resolvedModel.modelCustom
      form.options = { ...(cfg.options || {}) }
      const tokenValue =
        (cfg.options || {}).maxTokens
        ?? (cfg.options || {}).maxCompletionTokens
        ?? (cfg.options || {}).max_tokens
        ?? (cfg.options || {}).max_completion_tokens
      form.maxTokens = tokenValue == null ? '' : String(tokenValue)
    }
  } else {
    form.name = ''
    form.provider = 'openai'
    form.baseUrl = providers.value.openai?.baseUrl || ''
    form.apiKey = ''
    form.model = providers.value.openai?.defaultModel || ''
    form.modelCustom = ''
    form.options = {}
    form.maxTokens = ''
  }
}

const toggleApiKey = async () => {
  if (apiKeyVisible.value) {
    apiKeyVisible.value = false
    if (maskedApiKey.value && revealedApiKey.value && form.apiKey === revealedApiKey.value) {
      form.apiKey = maskedApiKey.value
    }
    return
  }

  apiKeyVisible.value = true
  if (props.mode !== 'edit' || !props.configId || !apiKeyIsMaskedPlaceholder.value) {
    return
  }
  if (revealedApiKey.value) {
    form.apiKey = revealedApiKey.value
    return
  }
  try {
    revealedApiKey.value = await api.getAIConfigAPIKey(props.configId)
    form.apiKey = revealedApiKey.value
  } catch {
    apiKeyVisible.value = false
  }
}

const resolveApiKeyForPayload = () => {
  const raw = form.apiKey.trim()
  if (props.mode !== 'edit' || !props.configId) return raw
  if (!raw) return ''
  if (maskedApiKey.value && raw === maskedApiKey.value) return ''
  if (revealedApiKey.value && raw === revealedApiKey.value) return ''
  return raw
}

const buildPayload = () => {
  const options = { ...(form.options || {}) }
  const parsedMaxTokens = Number.parseInt(String(form.maxTokens || '').trim(), 10)
  if (Number.isFinite(parsedMaxTokens) && parsedMaxTokens > 0) {
    options.maxTokens = parsedMaxTokens
  } else {
    delete options.maxTokens
  }
  return {
    name: form.name.trim(),
    provider: form.provider,
    baseUrl: form.provider === 'custom' ? form.baseUrl.trim() : form.baseUrl.trim(),
    apiKey: resolveApiKeyForPayload(),
    model: form.model === '' ? form.modelCustom.trim() : form.model,
    options,
  }
}

const test = async () => {
  errors.value = []
  statusText.value = tApp('status.testing')
  statusClass.value = 'testing'
  statusDetail.value = ''
  try {
    const payload = buildPayload()
    lastTestFingerprint.value = JSON.stringify(payload)
    const result = props.mode === 'edit' && props.configId
      ? await api.testAIConfigPreview(props.configId, payload)
      : await api.testAIConfigPayload(payload)
    statusText.value = result.connected ? tApp('status.connected') : tApp('status.failed')
    statusClass.value = result.connected ? 'connected' : 'failed'
    statusDetail.value = [result.modelInfo, result.latencyMs ? `${result.latencyMs}ms` : ''].filter(Boolean).join(' · ')
    lastTestConnected.value = Boolean(result.connected)
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    statusText.value = tApp('status.failed')
    statusClass.value = 'failed'
    statusDetail.value = message
    lastTestConnected.value = false
    lastTestFingerprint.value = ''
  }
}

const save = async () => {
  errors.value = []
  statusText.value = ''
  statusDetail.value = ''
  try {
    const payload = buildPayload()
    const fingerprint = JSON.stringify(payload)
    if (!payload.name) {
      errors.value = [tApp('validation.nameRequired')]
      return
    }
    if (!payload.provider) {
      errors.value = [tApp('validation.providerRequired')]
      return
    }
    if (props.mode === 'create' && !payload.apiKey) {
      errors.value = [tApp('validation.apiKeyRequired')]
      return
    }
    if (props.mode === 'edit' && !form.apiKey.trim()) {
      errors.value = [tApp('validation.apiKeyRequired')]
      return
    }
    if (payload.provider === 'custom' && !payload.baseUrl) {
      errors.value = [tApp('validation.baseUrlRequiredForCustomProvider')]
      return
    }
    let savedId = props.configId || ''
    if (props.mode === 'edit' && props.configId) {
      await api.updateAIConfig(props.configId, payload)
    } else {
      const created = await api.createAIConfig(payload)
      savedId = String((created as any)?.id || '')
    }

    if (savedId && lastTestConnected.value && lastTestFingerprint.value === fingerprint) {
      await api.testAIConfig(savedId)
    }
    await store.loadAIConfigs()
    emit('close')
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    errors.value = [message]
  }
}

onMounted(async () => {
  providers.value = await api.listAIProviders()
  await fillForm()
})

watch(
  () => [props.visible, props.mode, props.configId],
  async () => {
    if (props.visible) {
      await fillForm()
    }
  }
)
</script>
