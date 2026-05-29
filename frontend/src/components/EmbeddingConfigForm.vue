<template>
  <div class="ai-panel ai-form-panel inline show" id="embedding-config-form-panel">
    <div class="ai-panel-shell">
      <div class="ai-panel-header">
        <div class="ai-panel-title">
          <h3>{{ title }}</h3>
          <p class="ai-panel-subtitle">{{ tApp('ai.panel.embeddingSubtitle') }}</p>
        </div>
        <button class="btn ghost" type="button" @click="$emit('close')">&times;</button>
      </div>
      <div class="ai-form-core">
        <div v-if="errors.length" class="form-errors show">
          <div v-for="(err, idx) in errors" :key="idx">{{ err }}</div>
        </div>
        <div class="ai-form-grid">
          <div class="ai-field span-2">
            <label for="emb-name">{{ tApp('ai.form.configurationName') }}</label>
            <input
              id="emb-name"
              v-model="form.name"
              :placeholder="tApp('ai.form.namePlaceholder')"
              autocapitalize="off"
              autocomplete="off"
              autocorrect="off"
              spellcheck="false"
            />
          </div>
          <div class="ai-field">
            <label for="emb-provider">{{ tApp('ai.form.provider') }}</label>
            <select id="emb-provider" v-model="form.provider" @change="applyProviderDefaults">
              <option v-for="(info, key) in providers" :key="key" :value="key">
                {{ info.name }}
              </option>
            </select>
          </div>
          <div class="ai-field">
            <label for="emb-model">{{ tApp('ai.form.model') }}</label>
            <select id="emb-model" v-model="form.model">
              <option v-for="model in models" :key="model" :value="model">{{ model }}</option>
              <option value="">{{ tApp('common.custom') }}</option>
            </select>
            <input
              v-if="form.model === ''"
              id="emb-model-custom"
              v-model="form.modelCustom"
              :placeholder="tApp('ai.form.customModelPlaceholder')"
              autocapitalize="off"
              autocomplete="off"
              autocorrect="off"
              spellcheck="false"
              style="margin-top:8px;"
            />
          </div>
          <div class="ai-field" v-if="form.provider === 'custom'"></div>
          <div class="ai-field span-2" v-if="form.provider === 'custom'">
            <label for="emb-baseurl">{{ tApp('ai.form.embeddingEndpointUrl') }}</label>
            <input
              id="emb-baseurl"
              v-model="form.baseUrl"
              :placeholder="tApp('ai.form.embeddingEndpointUrlPlaceholder')"
              autocapitalize="off"
              autocomplete="off"
              autocorrect="off"
              spellcheck="false"
            />
          </div>
          <div class="ai-field span-2">
            <label for="emb-apikey">{{ tApp('ai.form.apiKey') }}</label>
            <div class="ai-input-with-toggle">
              <input
                id="emb-apikey"
                v-model="form.apiKey"
                :type="apiKeyVisible ? 'text' : 'password'"
                :placeholder="tApp('ai.form.apiKeyPlaceholder')"
                autocapitalize="off"
                autocomplete="off"
                autocorrect="off"
                spellcheck="false"
              />
              <button
                class="ai-visibility-toggle"
                type="button"
                :data-visible="apiKeyVisible"
                @click="apiKeyVisible = !apiKeyVisible"
              >
                <span aria-hidden="true">{{ apiKeyVisible ? tApp('common.hide') : tApp('common.show') }}</span>
              </button>
            </div>
          </div>
        </div>
        <p class="ai-form-note">{{ tApp('ai.panel.embeddingNote') }}</p>
      </div>
      <div class="ai-form-actions">
        <div class="ai-form-status">
          <span v-if="statusText" class="status" :class="statusClass">{{ statusText }}</span>
          <span v-if="statusDetail" class="status-detail">{{ statusDetail }}</span>
        </div>
        <button class="btn ghost" type="button" @click="test">{{ tApp('ai.form.testConnection') }}</button>
        <button class="btn" type="button" @click="save">{{ tApp('common.save') }}</button>
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

const props = defineProps<{ mode: 'create' | 'edit'; configId?: string | null }>()
const emit = defineEmits<{ (e: 'close'): void }>()

const store = useAppStore()
const providers = ref<Record<string, ProviderInfo>>({})
const apiKeyVisible = ref(false)
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
})

const title = computed(() => (props.mode === 'edit' ? tApp('ai.form.titleEdit') : tApp('ai.form.titleCreate')))

const models = computed(() => {
  const providerInfo = providers.value[form.provider]
  if (!providerInfo) return []
  return providerInfo.models || []
})

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
  apiKeyVisible.value = false

  if (props.mode === 'edit' && props.configId) {
    const cfg = store.embeddingConfigs.find((item) => item.id === props.configId)
    if (cfg) {
      form.name = cfg.name || ''
      form.provider = cfg.provider || 'openai'
      form.baseUrl = cfg.baseUrl || ''
      form.apiKey = cfg.apiKey || ''
      const knownModels = providers.value[form.provider]?.models || []
      if (knownModels.includes(cfg.model || '')) {
        form.model = cfg.model || ''
        form.modelCustom = ''
      } else {
        form.model = ''
        form.modelCustom = cfg.model || ''
      }
    }
  } else {
    form.name = ''
    form.provider = 'openai'
    form.baseUrl = providers.value.openai?.baseUrl || ''
    form.apiKey = ''
    form.model = providers.value.openai?.defaultModel || ''
    form.modelCustom = ''
  }
}

const buildPayload = () => {
  return {
    name: form.name.trim(),
    provider: form.provider,
    baseUrl: form.baseUrl.trim(),
    apiKey: form.apiKey.trim(),
    model: form.model === '' ? form.modelCustom.trim() : form.model,
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
    const result = await api.testEmbeddingConfigPayload(payload)
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
    if (!payload.name) { errors.value = [tApp('validation.nameRequired')]; return }
    if (!payload.model) { errors.value = [tApp('validation.modelRequired')]; return }
    if (payload.provider === 'custom' && !payload.baseUrl) { errors.value = [tApp('validation.baseUrlRequiredForCustomProvider')]; return }

    let savedId = props.configId || ''
    if (props.mode === 'edit' && props.configId) {
      await api.updateEmbeddingConfig(props.configId, payload)
    } else {
      const created = await api.createEmbeddingConfig(payload)
      savedId = String((created as any)?.id || '')
    }

    if (savedId && lastTestConnected.value && lastTestFingerprint.value === fingerprint) {
      await api.testEmbeddingConfig(savedId)
    }
    await store.loadEmbeddingConfigs()
    emit('close')
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    errors.value = [message]
  }
}

onMounted(async () => {
  providers.value = await api.listEmbeddingProviders()
  await store.loadEmbeddingConfigs()
  await fillForm()
})

watch(
  () => [props.mode, props.configId],
  async () => { await fillForm() },
)
</script>
