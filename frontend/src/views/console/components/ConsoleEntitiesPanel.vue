<script setup lang="ts">
import { computed, ref } from 'vue'
import { api } from '@/services/api'
import { tApp } from '@/modules/i18n/appI18n'
import { useConsoleViewContext } from '../context'
import { softBreakIdentifierHtml, softBreakIdentifierListHtml } from '../utils/identifierWrap'

const ctx = useConsoleViewContext()

const store = ctx.store

const entityTitle = ctx.entityTitle
const entityHeaderLabel = ctx.entityHeaderLabel
const entityHeaderPrimaryLabel = ctx.entityHeaderPrimaryLabel
const entityHeaderSecondaryLabel = ctx.entityHeaderSecondaryLabel
const entityHeaderTypeLabel = ctx.entityHeaderTypeLabel
const entityHeaderIconUrl = ctx.entityHeaderIconUrl
const loadEntities = ctx.loadEntities
const refreshEntities = () => loadEntities(true)

const showMongoDatabaseSwitch = ctx.showMongoDatabaseSwitch
const enterMongoDatabaseMode = ctx.enterMongoDatabaseMode

const showEntityFilter = ctx.showEntityFilter
const entityFilterLabel = ctx.entityFilterLabel
const entityPattern = ctx.entityPattern
const entityFilterPlaceholder = ctx.entityFilterPlaceholder
const entityFilterHint = ctx.entityFilterHint

const mongoDatabaseMode = ctx.mongoDatabaseMode
const mongoDatabaseError = ctx.mongoDatabaseError
const filteredDatabases = ctx.filteredDatabases
const selectMongoDatabase = ctx.selectMongoDatabase
const promptMongoDatabase = ctx.promptMongoDatabase

const isRedis = ctx.isRedis
const emptyEntityLabel = ctx.emptyEntityLabel
const parityWorkspaceKind = ctx.parityWorkspaceKind

const entityPagingEnabled = ctx.entityPagingEnabled
const entityPagingLoading = ctx.entityPagingLoading
const entityPagingDone = ctx.entityPagingDone
const loadMoreEntities = ctx.loadMoreEntities

const isElasticWorkspace = computed(() => parityWorkspaceKind?.value === 'elastic')
const isDynamo = computed(() => store.current?.type === 'dynamodb')
const isChromaWorkspace = computed(() => parityWorkspaceKind?.value === 'chroma')
const fieldsTabLabel = computed(() => (isElasticWorkspace.value ? tApp('console.entities.mappings') : tApp('console.entities.fields')))
const indexesTabLabel = computed(() => (isElasticWorkspace.value ? tApp('console.entities.stats') : tApp('console.entities.indexes')))
const disableIndexesTab = (detail: any) => {
  if (!showEntityFields.value) return false
  if (isElasticWorkspace.value) return false
  if (isDynamo.value) {
    const details = detail?.details
    let hasPartitionKey = false
    if (Array.isArray(details)) {
      for (const item of details) {
        if (String(item?.label || '') !== 'Partition Key') continue
        hasPartitionKey = String(item?.value ?? '').trim() !== ''
        break
      }
    }
    return !(hasPartitionKey || detail?.indexes?.length)
  }
  return !(detail?.indexes?.length)
}

const isDynamoKeyRow = (idx: any) => {
  if (!isDynamo.value) return false
  const name = String(idx?.name || '')
  return name === 'Partition Key' || name === 'Sort Key'
}

const dynamoKeyIndexRows = (detail: any) => {
  if (!isDynamo.value) return null
  const details = detail?.details
  if (!Array.isArray(details)) return null

  let pk = ''
  let sk = ''
  for (const item of details) {
    const label = String(item?.label || '')
    if (label === 'Partition Key') pk = String(item?.value ?? '').trim()
    if (label === 'Sort Key') sk = String(item?.value ?? '').trim()
  }
  if (!pk) return null

  return [
    { name: 'Partition Key', column: pk, unique: true },
    { name: 'Sort Key', column: sk || '-', unique: false },
  ]
}

const dynamoIndexList = (detail: any) => {
  const indexes = Array.isArray(detail?.indexes) ? detail.indexes : []
  if (!isDynamo.value) return indexes
  const keyRows = dynamoKeyIndexRows(detail)
  return keyRows ? [...keyRows, ...indexes] : indexes
}

const dynamoSecondaryIndexes = (detail: any) =>
  Array.isArray(detail?.indexes) ? detail.indexes : []

// Returns HTML containing <wbr> hints; bind with v-html. Avoids ZWSPs in the
// text node so users can copy identifier names from the panel cleanly.
const wrapIdentifier = (value: unknown) => softBreakIdentifierHtml(String(value ?? ''))

const wrapIdentifierList = (values: ReadonlyArray<string>) =>
  softBreakIdentifierListHtml(values)

const redisRootLoading = ctx.redisRootLoading
const filteredRedisTreeItems = ctx.filteredRedisTreeItems
const isRedisExpanded = ctx.isRedisExpanded
const toggleRedisFolder = ctx.toggleRedisFolder
const selectRedisItem = ctx.selectRedisItem

const filteredEntities = ctx.filteredEntities
const describeEntity = ctx.describeEntity
const isEntityExpanded = ctx.isEntityExpanded
const toggleEntityExpanded = ctx.toggleEntityExpanded
const entityDetailsLoading = ctx.entityDetailsLoading
const entityDetailsError = ctx.entityDetailsError
const entityDetails = ctx.entityDetails
const expandedEntityView = ctx.expandedEntityView
const showEntityFields = ctx.showEntityFields
const indexKindClass = ctx.indexKindClass
const indexKindLabel = ctx.indexKindLabel
const indexFieldList = ctx.indexFieldList

const elasticFieldFilters = ref<Record<string, string>>({})
const elasticMetaFor = (name: string) => {
  const meta = store.elasticsearchIndexMeta
  if (!meta || typeof meta !== 'object') return null
  return meta[name] || null
}

const elasticHealthValue = (name: string) => String(elasticMetaFor(name)?.health || '').trim().toLowerCase()

const elasticHealthLabel = (name: string) => {
  const value = elasticHealthValue(name)
  return value || '-'
}

const elasticHealthClass = (name: string) => {
  const value = elasticHealthValue(name)
  if (value === 'green') return 'green'
  if (value === 'yellow') return 'yellow'
  if (value === 'red') return 'red'
  return 'unknown'
}

const elasticStoreSizeLabel = (name: string) => {
  const value = String(elasticMetaFor(name)?.storeSize || '').trim()
  return value || '-'
}

const elasticFieldsFor = (name: string) => {
  const detail = entityDetails[name]
  const columns = Array.isArray(detail?.columns) ? detail.columns : []
  return columns
    .map((column: any) => ({
      name: String(column?.name || '').trim(),
      type: String(column?.dataType || '').trim(),
    }))
    .filter((item: { name: string; type: string }) => Boolean(item.name))
}

const elasticFilteredFields = (name: string) => {
  const keyword = String(elasticFieldFilters.value[name] || '').trim().toLowerCase()
  const fields = elasticFieldsFor(name)
  if (!keyword) return fields
  return fields.filter((field) => field.name.toLowerCase().includes(keyword))
}

const defaultElasticFieldSelection = (name: string) => {
  const fields = elasticFieldsFor(name).map((item) => item.name)
  return fields
}

const normalizedElasticFieldSelection = (values: unknown) =>
  Array.isArray(values)
    ? Array.from(
      new Set(
        values
          .map((field) => String(field || '').trim())
          .filter(Boolean),
      ),
    )
    : []

const savedElasticFieldSelection = (name: string) => {
  const datasourceId = String(store.current?.id || '').trim()
  if (!datasourceId) return []
  return normalizedElasticFieldSelection(store.elasticsearchFieldSelectionsByDatasource?.[datasourceId]?.[name])
}

const elasticSelectionList = (name: string) => {
  const fields = elasticFieldsFor(name).map((item) => item.name)
  const current = normalizedElasticFieldSelection(store.elasticsearchFieldSelections[name])
  const previous = current.length ? current : savedElasticFieldSelection(name)
  if (!fields.length) {
    if (previous.length) return previous
    return []
  }
  if (previous.length) {
    const normalized = Array.from(
      new Set(
        previous
          .filter((field) => field && fields.includes(field)),
      ),
    )
    if (normalized.length) {
      const changed =
        normalized.length !== current.length ||
        normalized.some((field, index) => field !== current[index])
      if (changed) {
        store.elasticsearchFieldSelections[name] = normalized
      }
      return normalized
    }
  }
  const defaults = defaultElasticFieldSelection(name)
  if (!defaults.length) return []
  const unchanged = current.length === defaults.length && current.every((field, index) => field === defaults[index])
  if (!unchanged) {
    store.elasticsearchFieldSelections[name] = defaults
  }
  return defaults
}

const emptyElasticFieldSet = new Set<string>()
const elasticSelectedFieldSetMap = computed(() => {
  const map: Record<string, Set<string>> = {}
  filteredEntities.value.forEach((name: string) => {
    map[name] = new Set(elasticSelectionList(name))
  })
  return map
})

const onElasticFieldChecked = (name: string, fieldName: string, checked: boolean) => {
  const next = new Set(elasticSelectionList(name))
  if (checked) {
    next.add(fieldName)
  } else {
    if (next.has(fieldName) && next.size <= 1) return
    next.delete(fieldName)
  }
  const order = elasticFieldsFor(name).map((field) => field.name)
  store.elasticsearchFieldSelections[name] = order.filter((field) => next.has(field))
}

const elasticFieldTypeTag = (dataType: string) => {
  const normalized = String(dataType || '').trim().toLowerCase()
  if (!normalized) return ''
  if (normalized.includes('keyword')) return 'kw'
  if (normalized.includes('text')) return 'txt'
  if (normalized.includes('date') || normalized.includes('time')) return 'dt'
  if (normalized.includes('long') || normalized.includes('double') || normalized.includes('float') || normalized.includes('int')) return 'num'
  return ''
}

const normalizeChromaDetailLabel = (raw: string) => {
  const normalized = String(raw || '').trim().toLowerCase()
  if (normalized === 'id') return tApp('console.entities.chroma.id')
  if (normalized === 'dimension') return tApp('console.entities.chroma.dimension')
  if (normalized === 'records') return tApp('console.entities.chroma.records')
  if (normalized === 'metadata') return tApp('console.entities.chroma.metadata')
  return String(raw || '').trim()
}

const chromaCollectionDetails = (detail: any) => {
  const details = Array.isArray(detail?.details) ? detail.details : []
  return details
    .map((item) => {
      const rawLabel = String(item?.label || '').trim()
      if (!rawLabel) return null
      const value = item?.value
      const displayValue = typeof value === 'string'
        ? value
        : value == null
          ? '-'
          : JSON.stringify(value)
      return {
        label: normalizeChromaDetailLabel(rawLabel),
        rawLabel: rawLabel.toLowerCase(),
        value: String(displayValue || '-'),
      }
    })
    .filter(Boolean) as Array<{ label: string, rawLabel: string, value: string }>
}

const chromaCollectionBadges = (name: string) => {
  const detail = entityDetails[name]
  return chromaCollectionDetails(detail)
    .filter((item) => item.rawLabel === 'dimension' || item.rawLabel === 'records')
}

type CreateTableDialogState = {
  open: boolean
  table: string
  loading: boolean
  sql: string
  error: string
}

const createTableDialog = ref<CreateTableDialogState>({
  open: false,
  table: '',
  loading: false,
  sql: '',
  error: '',
})

const canShowCreateTable = computed(() => store.current?.type === 'mysql')

const quoteMysqlIdent = (value: string) => `\`${String(value).replaceAll('`', '``')}\``

const extractCreateStatement = (result: any) => {
  const columns = Array.isArray(result?.columns) ? result.columns.map((c: any) => String(c)) : []
  const rows = Array.isArray(result?.rows) ? result.rows : []
  const row = rows[0] || null
  if (!row) return ''

  const createColumn =
    columns.find((col) => col.toLowerCase().includes('create table')) ??
    columns.find((col) => col.toLowerCase().includes('create'))
  if (createColumn && row[createColumn] != null) {
    return String(row[createColumn])
  }

  const rowKey =
    Object.keys(row).find((key) => key.toLowerCase().includes('create table')) ??
    Object.keys(row).find((key) => key.toLowerCase().includes('create'))
  if (rowKey && row[rowKey] != null) {
    return String(row[rowKey])
  }

  for (const key of Object.keys(row)) {
    const value = row[key]
    if (typeof value === 'string' && value.trim()) return value
  }
  return ''
}

const closeCreateTableDialog = () => {
  createTableDialog.value.open = false
}

const copyCreateTableSQL = async () => {
  const sql = createTableDialog.value.sql.trim()
  if (!sql) return
  try {
    await navigator.clipboard.writeText(sql)
    store.setNotice(tApp('console.entities.createTableCopied'), 'success')
  } catch (err) {
    store.setNotice(err instanceof Error ? err.message : tApp('common.copyFailed'), 'error')
  }
}

const openCreateTableDialog = async (table: string) => {
  if (!store.current || !canShowCreateTable.value) return

  createTableDialog.value = { open: true, table, loading: true, sql: '', error: '' }
  try {
    const result = await api.executeStatement(
      store.current.id,
      `SHOW CREATE TABLE ${quoteMysqlIdent(table)}`,
      store.mongoDatabase,
      '',
      10,
    )
    const sql = extractCreateStatement(result)
    if (!sql) {
      createTableDialog.value = { open: true, table, loading: false, sql: '', error: tApp('console.entities.noCreateTableOutput') }
      return
    }
    createTableDialog.value = { open: true, table, loading: false, sql, error: '' }
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    createTableDialog.value = { open: true, table, loading: false, sql: '', error: message }
  }
}

const onEntityContextMenu = (table: string, event: MouseEvent) => {
  if (!canShowCreateTable.value) return
  event.preventDefault()
  event.stopPropagation()
  void openCreateTableDialog(table)
}

const onEntityListScroll = (event: Event) => {
  if (!entityPagingEnabled?.value) return
  if (entityPagingLoading?.value || entityPagingDone?.value) return
  const el = event.target as HTMLElement | null
  if (!el) return
  const threshold = 80
  if (el.scrollTop + el.clientHeight >= el.scrollHeight - threshold) {
    void loadMoreEntities()
  }
}
</script>

<template>
  <div class="panel console-panel console-panel--entities">
    <div class="panel-head">
      <div class="entity-panel-header-main" data-testid="entity-panel-header">
        <img
          v-if="entityHeaderIconUrl"
          class="entity-panel-header-icon"
          data-testid="entity-panel-header-icon"
          :src="entityHeaderIconUrl"
          :alt="tApp('datasource.list.typeLogoAlt', { type: entityHeaderTypeLabel || entityHeaderPrimaryLabel || entityHeaderLabel || entityTitle })"
          loading="lazy"
        />
        <div class="entity-panel-header-copy">
          <h4 id="entity-title" class="entity-panel-header-label" data-testid="entity-panel-header-label">
            {{ entityHeaderPrimaryLabel || entityHeaderLabel || entityTitle }}
          </h4>
          <p
            v-if="entityHeaderSecondaryLabel"
            class="entity-panel-header-meta"
            data-testid="entity-panel-header-meta"
          >
            {{ entityHeaderSecondaryLabel }}
          </p>
        </div>
      </div>
      <div class="panel-head-actions">
        <button
          class="entity-panel-refresh-button"
          type="button"
          :aria-label="tApp('console.entities.refresh')"
          :title="tApp('console.entities.refresh')"
          data-testid="entity-panel-refresh"
          @click="refreshEntities"
        >
          <span class="material-symbols-outlined" aria-hidden="true">refresh</span>
        </button>
        <button
          v-if="showMongoDatabaseSwitch"
          class="btn ghost mini"
          type="button"
          @click="enterMongoDatabaseMode"
        >
          {{ tApp('console.entities.databases') }}
        </button>
      </div>
    </div>

    <div v-if="showEntityFilter">
      <label for="entity-pattern" id="entity-pattern-label">{{ entityFilterLabel }}</label>
      <input
        id="entity-pattern"
        v-model="entityPattern"
        :placeholder="entityFilterPlaceholder"
        autocapitalize="off"
        autocomplete="off"
        autocorrect="off"
        spellcheck="false"
      />
      <p class="meta" id="entity-pattern-hint">{{ entityFilterHint }}</p>
    </div>

    <div class="entity-list" id="entity-list" @scroll="onEntityListScroll">
      <template v-if="mongoDatabaseMode">
        <div v-if="mongoDatabaseError" class="meta">{{ mongoDatabaseError }}</div>
        <div v-for="db in filteredDatabases" :key="db" class="entity-item" @click="selectMongoDatabase(db)">
          <div class="entity-title" :title="db">{{ db }}</div>
        </div>
        <button class="btn ghost small" type="button" @click="promptMongoDatabase">
          {{ tApp('console.entities.createDatabase') }}
        </button>
      </template>
      <template v-else>
        <div v-if="!store.current" class="meta">{{ tApp('console.subtitle.selectDatasource') }}</div>
        <template v-else-if="isRedis">
          <div v-if="redisRootLoading && filteredRedisTreeItems.length === 0" class="meta">{{ tApp('console.entities.loadingKeys') }}</div>
          <div v-else-if="filteredRedisTreeItems.length === 0" class="empty-state compact"><span>{{ emptyEntityLabel }}</span></div>
          <div
            v-for="item in filteredRedisTreeItems"
            :key="item.id"
            class="entity-item"
            :class="{ active: item.isKey && item.prefix === store.selectedEntity }"
            @click="selectRedisItem(item)"
          >
            <div class="redis-tree-row" :style="{ paddingLeft: `${item.depth * 14}px` }">
              <button
                v-if="item.isFolder"
                class="redis-tree-toggle"
                type="button"
                :aria-label="isRedisExpanded(item.prefix) ? tApp('console.entities.collapse') : tApp('console.entities.expand')"
                @click.stop="toggleRedisFolder(item)"
              >
                {{ isRedisExpanded(item.prefix) ? '▾' : '▸' }}
              </button>
              <span v-else class="redis-tree-toggle placeholder"></span>
              <span
                class="entity-title"
                :class="{ 'redis-tree-folder': item.isFolder, 'redis-tree-key': item.isKey }"
                :title="item.prefix"
              >
                {{ item.label }}
              </span>
              <span v-if="item.isFolder" class="redis-tree-count">{{ item.childrenCount }}</span>
            </div>
          </div>
        </template>
        <template v-else>
          <div v-if="entityPagingEnabled && entityPagingLoading && store.entities.length === 0" class="meta">
            Loading...
          </div>
          <div v-else-if="store.entities.length === 0" class="empty-state compact"><span>{{ emptyEntityLabel }}</span></div>
          <div v-for="item in filteredEntities" :key="item" class="entity-entry">
            <div
              class="entity-item"
              :class="{ active: item === store.selectedEntity, expanded: isEntityExpanded(item) }"
              @click="describeEntity(item, { autoExecute: true })"
              @contextmenu="onEntityContextMenu(item, $event)"
            >
              <button
                class="entity-toggle"
                type="button"
                :aria-label="isEntityExpanded(item) ? tApp('console.entities.collapseDetails') : tApp('console.entities.expandDetails')"
                @click.stop="toggleEntityExpanded(item)"
              >
                <span class="entity-toggle-icon" :class="{ open: isEntityExpanded(item) }" aria-hidden="true"></span>
              </button>
              <div class="entity-title" :title="item">{{ item }}</div>
              <span v-if="store.entityKinds[item] === 'view'" class="entity-kind-pill entity-kind--view" :title="tApp('console.entities.view')">{{ tApp('console.entities.view') }}</span>
              <div v-if="isElasticWorkspace && elasticMetaFor(item)" class="es-index-meta">
                <span class="es-health-pill" :class="elasticHealthClass(item)">{{ elasticHealthLabel(item) }}</span>
                <span class="es-store-size">{{ elasticStoreSizeLabel(item) }}</span>
              </div>
              <div v-else-if="isChromaWorkspace && chromaCollectionBadges(item).length" class="chroma-collection-inline">
                <span
                  v-for="badge in chromaCollectionBadges(item)"
                  :key="`${item}-${badge.rawLabel}`"
                  class="chroma-collection-badge"
                  :title="`${badge.label}: ${badge.value}`"
                >
                  <span class="chroma-badge-icon" aria-hidden="true">{{ badge.rawLabel === 'dimension' ? 'd' : '#' }}</span>
                  {{ badge.value }}
                </span>
              </div>
            </div>
            <div v-if="isEntityExpanded(item)" class="entity-expand">
              <div v-if="entityDetailsLoading[item]" class="meta">{{ tApp('console.entities.loadingDetails') }}</div>
              <div v-else-if="entityDetailsError[item]" class="meta">{{ tApp('console.entities.failed', { message: entityDetailsError[item] }) }}</div>
              <div v-else-if="entityDetails[item]" class="entity-expand-body">
                <div v-if="isElasticWorkspace" class="es-index-fields-panel">
                  <div class="es-index-fields-head">
                    <span class="es-index-fields-label">{{ tApp('console.entities.fields') }}</span>
                    <span class="es-index-fields-count">{{ elasticFieldsFor(item).length }}</span>
                  </div>
                  <input
                    :data-testid="`elastic-index-fields-filter-${item}`"
                    v-model="elasticFieldFilters[item]"
                    class="es-index-fields-filter"
                    :placeholder="tApp('console.elastic.results.filterFieldsPlaceholder')"
                    type="text"
                    autocapitalize="off"
                    autocorrect="off"
                    spellcheck="false"
                  />
                  <div class="es-index-fields-list">
                    <label
                      v-for="field in elasticFilteredFields(item)"
                      :key="field.name"
                      class="es-index-field-item"
                    >
                      <input
                        :checked="(elasticSelectedFieldSetMap[item] || emptyElasticFieldSet).has(field.name)"
                        type="checkbox"
                        @change="onElasticFieldChecked(item, field.name, ($event.target as HTMLInputElement).checked)"
                      />
                      <span class="es-index-field-name">{{ field.name }}</span>
                      <span v-if="elasticFieldTypeTag(field.type)" class="es-index-field-type">{{ elasticFieldTypeTag(field.type) }}</span>
                    </label>
                    <div v-if="elasticFilteredFields(item).length === 0" class="meta">{{ tApp('console.entities.noFields') }}</div>
                  </div>
                </div>
                <template v-else-if="isChromaWorkspace">
                  <div class="entity-expand-panel entity-expand-panel--chroma">
                    <div v-if="chromaCollectionDetails(entityDetails[item]).length" class="chroma-detail-list">
                      <div
                        v-for="detail in chromaCollectionDetails(entityDetails[item])"
                        :key="`${item}-${detail.rawLabel}`"
                        class="chroma-detail-row"
                      >
                        <span class="chroma-detail-label">{{ detail.label }}</span>
                        <span
                          class="chroma-detail-value"
                          :class="{ 'is-mono': detail.rawLabel === 'id' }"
                          :title="detail.value"
                        >{{ detail.value }}</span>
                      </div>
                    </div>
                    <div v-else class="meta">{{ tApp('console.entities.noDetails') }}</div>
                  </div>
                </template>
                <template v-else>
                  <div class="entity-expand-tabs" role="tablist" :aria-label="tApp('console.entities.details')">
                    <button
                      v-if="showEntityFields"
                      class="entity-expand-tab"
                      type="button"
                      role="tab"
                      :aria-selected="expandedEntityView === 'fields'"
                      :class="{ active: expandedEntityView === 'fields' }"
                      @click="expandedEntityView = 'fields'"
                    >
                      {{ fieldsTabLabel }}
                    </button>
                    <button
                      class="entity-expand-tab"
                      type="button"
                      role="tab"
                      :aria-selected="!showEntityFields || expandedEntityView === (isElasticWorkspace ? 'stats' : 'indexes')"
                      :class="{ active: !showEntityFields || expandedEntityView === (isElasticWorkspace ? 'stats' : 'indexes') }"
                      :disabled="disableIndexesTab(entityDetails[item])"
                      @click="expandedEntityView = isElasticWorkspace ? 'stats' : 'indexes'"
                    >
                      {{ indexesTabLabel }}
                    </button>
                  </div>
                  <div v-if="showEntityFields && expandedEntityView === 'fields'" class="entity-expand-panel">
                    <div v-if="entityDetails[item]?.columns?.length">
                      <table class="columns-table compact">
                        <thead>
                          <tr>
                            <th>{{ isElasticWorkspace ? tApp('console.entities.field') : tApp('console.entities.column') }}</th>
                            <th>{{ tApp('common.type') }}</th>
                            <th>{{ tApp('console.entities.nullable') }}</th>
                            <th>{{ tApp('console.entities.default') }}</th>
                          </tr>
                        </thead>
                        <tbody>
                          <tr v-for="col in entityDetails[item]?.columns" :key="col.name">
                            <td class="col-name" :title="col.name" v-html="wrapIdentifier(col.name)"></td>
                            <td>{{ col.dataType }}</td>
                            <td>{{ col.nullable }}</td>
                            <td>{{ col.defaultValue ?? '-' }}</td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                    <div v-else class="meta">{{ isElasticWorkspace ? tApp('console.entities.noMappings') : tApp('console.entities.noFields') }}</div>
                  </div>
                  <div v-else class="entity-expand-panel">
                    <template v-if="isElasticWorkspace">
                      <div v-if="entityDetails[item]?.details?.length" class="detail-list">
                        <div v-for="d in entityDetails[item]?.details" :key="d.label" class="detail-row">
                          <span class="detail-label">{{ d.label }}</span>
                          <span class="detail-value">{{ d.value ?? '-' }}</span>
                        </div>
                      </div>
                    <div v-else class="meta">{{ tApp('console.entities.noStats') }}</div>
                  </template>
                  <template v-else-if="isDynamo">
                    <div v-if="dynamoIndexList(entityDetails[item])?.length" class="index-list">
                      <div v-if="dynamoKeyIndexRows(entityDetails[item])?.length" class="index-list-section">
                        <h5 class="index-list-section-head">
                          <span>{{ tApp('console.entities.tableKeys') }}</span>
                          <span class="index-list-section-count">{{ dynamoKeyIndexRows(entityDetails[item])?.length }}</span>
                        </h5>
                        <div
                          v-for="idx in dynamoKeyIndexRows(entityDetails[item])"
                          :key="`key-${idx.name}`"
                          class="index-row ddb-key-row"
                        >
                          <span class="index-kind-pill" :class="indexKindClass(idx)">{{ indexKindLabel(idx) }}</span>
                          <div class="index-row-content">
                            <div class="index-row-name ddb-key-name" :title="idx.name" v-html="wrapIdentifier(idx.name)"></div>
                            <div class="index-row-fields" :title="indexFieldList(idx).join(', ')">
                              <span class="index-row-fields-label">{{ tApp('console.entities.indexColumns') }}</span>
                              <span class="index-row-fields-value" v-html="wrapIdentifierList(indexFieldList(idx))"></span>
                            </div>
                          </div>
                        </div>
                      </div>
                      <div v-if="dynamoSecondaryIndexes(entityDetails[item])?.length" class="index-list-section">
                        <h5 class="index-list-section-head">
                          <span>{{ tApp('console.entities.secondaryIndexes') }}</span>
                          <span class="index-list-section-count">{{ dynamoSecondaryIndexes(entityDetails[item])?.length }}</span>
                        </h5>
                        <div
                          v-for="idx in dynamoSecondaryIndexes(entityDetails[item])"
                          :key="`idx-${idx.name}`"
                          class="index-row"
                        >
                          <span class="index-kind-pill" :class="indexKindClass(idx)">{{ indexKindLabel(idx) }}</span>
                          <div class="index-row-content">
                            <div class="index-row-name" :title="idx.name" v-html="wrapIdentifier(idx.name)"></div>
                            <div class="index-row-fields" :title="indexFieldList(idx).join(', ')">
                              <span class="index-row-fields-label">{{ tApp('console.entities.indexColumns') }}</span>
                              <span class="index-row-fields-value" v-html="wrapIdentifierList(indexFieldList(idx))"></span>
                            </div>
                          </div>
                        </div>
                      </div>
                    </div>
                    <div v-else class="meta">{{ tApp('console.entities.noIndexes') }}</div>
                  </template>
                  <template v-else>
                    <div v-if="entityDetails[item]?.indexes?.length" class="index-list">
                      <div
                        v-for="idx in entityDetails[item]?.indexes"
                        :key="idx.name"
                        class="index-row"
                      >
                        <span class="index-kind-pill" :class="indexKindClass(idx)">{{ indexKindLabel(idx) }}</span>
                        <div class="index-row-content">
                          <div class="index-row-name" :title="idx.name" v-html="wrapIdentifier(idx.name)"></div>
                          <div class="index-row-fields" :title="indexFieldList(idx).join(', ')">
                            <span class="index-row-fields-label">{{ tApp('console.entities.indexColumns') }}</span>
                            <span class="index-row-fields-value" v-html="wrapIdentifierList(indexFieldList(idx))"></span>
                          </div>
                        </div>
                      </div>
                    </div>
                    <div v-else class="meta">{{ tApp('console.entities.noIndexes') }}</div>
                  </template>
                  </div>
                </template>
              </div>
              <div v-else class="meta">{{ tApp('console.entities.noDetails') }}</div>
            </div>
          </div>
          <div v-if="entityPagingEnabled && entityPagingLoading && store.entities.length > 0" class="meta">{{ tApp('console.entities.loadingMore') }}</div>
        </template>
      </template>
    </div>
  </div>

  <div
    v-if="createTableDialog.open"
    class="dialog-backdrop"
    role="dialog"
    aria-modal="true"
    data-testid="create-table-dialog"
    @click.self="closeCreateTableDialog"
  >
    <div class="dialog-card">
      <div class="dialog-head">
        <div>
          <h4>{{ tApp('console.entities.createTableTitle') }}</h4>
          <div class="meta">{{ createTableDialog.table }}</div>
        </div>
        <span class="pill">MySQL</span>
      </div>

      <div v-if="createTableDialog.loading" class="meta">{{ tApp('console.entities.loading') }}</div>
      <div v-else-if="createTableDialog.error" class="meta">{{ tApp('console.entities.failed', { message: createTableDialog.error }) }}</div>
      <pre v-else class="dialog-code" data-testid="create-table-sql">{{ createTableDialog.sql }}</pre>

      <div class="dialog-actions">
        <button
          class="btn ghost"
          type="button"
          :disabled="createTableDialog.loading || !createTableDialog.sql"
          @click="copyCreateTableSQL"
        >
          {{ tApp('common.copy') }}
        </button>
        <button class="btn" type="button" @click="closeCreateTableDialog">{{ tApp('console.entities.close') }}</button>
      </div>
    </div>
  </div>
</template>
