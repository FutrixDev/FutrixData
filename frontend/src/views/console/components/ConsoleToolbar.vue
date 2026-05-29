<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { getDatasourceTypeIconUrl } from '@/modules/datasource/icons'
import { formatDatasourceTypeLabel, normalizeDatasourceType } from '@/modules/datasource/types'
import { deriveMongoDisplay } from '@/modules/mongo/datasource'
import { useConsoleViewContext } from '../context'
import { tApp } from '@/modules/i18n/appI18n'

const router = useRouter()
const ctx = useConsoleViewContext()

const store = ctx.store
const consoleSubtitle = ctx.consoleSubtitle
const backToList = ctx.backToList
const connectedDatasources = ctx.connectedDatasources
const switchDatasourceById = ctx.switchDatasourceById

const datasourceMenuOpen = ref(false)
const datasourceDropdownRef = ref<HTMLElement | null>(null)
const datasourceMenuId = 'console-datasource-menu'

const datasourceTypeLabel = (type: string) => formatDatasourceTypeLabel(normalizeDatasourceType(String(type || '').toLowerCase()))
const datasourceTypeIcon = (type: string) => getDatasourceTypeIconUrl(type)
const normalizedDatasourceType = (type: string) => normalizeDatasourceType(String(type || '').toLowerCase())

const normalizeDatasourceOptions = (options: any) => {
  if (!options || typeof options !== 'object') {
    if (typeof options === 'string') {
      try {
        const parsed = JSON.parse(options)
        if (parsed && typeof parsed === 'object') return parsed
      } catch {}
    }
    return {}
  }
  return options
}

const datasourceEndpoint = (ds: any) => {
  const normalizedType = normalizedDatasourceType(ds?.type || '')
  if (normalizedType === 'd1') {
    const options = normalizeDatasourceOptions(ds?.options)
    const configuredDatabaseName = String(options?.databaseName || ds?.database || ds?.name || '').trim()
    if (configuredDatabaseName) return configuredDatabaseName
    return '-'
  }
  if (normalizedType === 'mongodb') {
    const mongo = deriveMongoDisplay({
      ...ds,
      options: normalizeDatasourceOptions(ds?.options),
    })
    const mongoHost = String(mongo.hostLabel || '').trim()
    if (mongoHost) return mongoHost
  }
  const host = String(ds?.host || '').trim()
  const port = Number(ds?.port || 0)
  if (!host && !port) return '-'
  if (!port) return host || '-'
  return `${host}:${port}`
}

const datasourceOptionLabel = (ds: any) => `${datasourceTypeLabel(ds?.type)} - ${String(ds?.name || '')} | ${datasourceEndpoint(ds)}`

const currentConnectedDatasource = computed(() => {
  const currentId = String(store.current?.id || '')
  if (!currentId) return null
  return connectedDatasources.value.find((item: any) => String(item.id || '') === currentId) || null
})

const canSwitchDatasource = computed(() => connectedDatasources.value.length > 0)

const datasourceTriggerLabel = computed(() => {
  const current = currentConnectedDatasource.value
  if (!current) return tApp('console.switchDatasource')
  return datasourceOptionLabel(current)
})

const isCurrentDatasource = (id: string) => String(store.current?.id || '') === String(id || '')

const closeDatasourceMenu = () => {
  datasourceMenuOpen.value = false
}

const toggleDatasourceMenu = () => {
  if (!canSwitchDatasource.value) return
  datasourceMenuOpen.value = !datasourceMenuOpen.value
}

const selectDatasource = async (id: string) => {
  closeDatasourceMenu()
  await switchDatasourceById(String(id || ''))
}

const handleDocumentMouseDown = (event: MouseEvent) => {
  if (!datasourceMenuOpen.value) return
  const target = event.target as Node | null
  if (!target) return
  if (!datasourceDropdownRef.value) return
  if (datasourceDropdownRef.value.contains(target)) return
  closeDatasourceMenu()
}

onMounted(() => {
  document.addEventListener('mousedown', handleDocumentMouseDown)
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', handleDocumentMouseDown)
})
</script>

<template>
  <div class="list-toolbar">
    <div class="console-toolbar-title">
      <h2 id="console-title">{{ tApp('console.title') }}</h2>
      <p class="meta" id="console-subtitle" :title="consoleSubtitle">{{ consoleSubtitle }}</p>
    </div>
    <div class="console-actions">
      <button
        class="btn secondary"
        type="button"
        style="flex: 0 0 auto; white-space: nowrap; border-color: rgba(217, 119, 6, 0.3); color: #b45309;"
        :disabled="!store.current?.id"
        @click="store.current?.id && router.push({ name: 'sensitivity-detail', params: { id: store.current.id } })"
        :title="tApp('sensitivity.title')"
      >🛡️ {{ tApp('sensitivity.title') }}</button>
      <button class="btn secondary" type="button" @click="backToList">{{ tApp('common.back') }}</button>
      <div ref="datasourceDropdownRef" class="console-datasource-dropdown">
        <button
          class="btn secondary console-datasource-trigger"
          type="button"
          :disabled="!canSwitchDatasource"
          :aria-expanded="datasourceMenuOpen"
          :aria-controls="datasourceMenuId"
          :aria-label="tApp('console.switchDatasource')"
          aria-haspopup="listbox"
          data-testid="console-datasource-dropdown-trigger"
          @click="toggleDatasourceMenu"
        >
          <img
            v-if="currentConnectedDatasource && datasourceTypeIcon(currentConnectedDatasource.type)"
            class="datasource-type-icon console-datasource-trigger-icon"
            :src="datasourceTypeIcon(currentConnectedDatasource.type)!"
            :alt="tApp('datasource.list.typeLogoAlt', { type: datasourceTypeLabel(currentConnectedDatasource.type) })"
            loading="lazy"
          />
          <span class="console-datasource-trigger-label">{{ datasourceTriggerLabel }}</span>
          <span class="console-datasource-trigger-arrow">▾</span>
        </button>
        <div
          v-if="datasourceMenuOpen"
          :id="datasourceMenuId"
          class="console-datasource-menu"
          role="listbox"
        >
          <button
            v-for="ds in connectedDatasources"
            :key="ds.id"
            class="console-datasource-option"
            :class="{ active: isCurrentDatasource(ds.id) }"
            type="button"
            role="option"
            :aria-selected="isCurrentDatasource(ds.id)"
            :data-datasource-id="ds.id"
            data-testid="console-datasource-dropdown-option"
            @click="selectDatasource(ds.id)"
          >
            <img
              v-if="datasourceTypeIcon(ds.type)"
              class="datasource-type-icon console-datasource-option-icon"
              :src="datasourceTypeIcon(ds.type)!"
              :alt="tApp('datasource.list.typeLogoAlt', { type: datasourceTypeLabel(ds.type) })"
              loading="lazy"
            />
            <span class="console-datasource-option-label">{{ datasourceOptionLabel(ds) }}</span>
          </button>
          <div v-if="!connectedDatasources.length" class="console-datasource-empty">
            {{ tApp('console.lifecycle.noConnectedDatasources') }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
