import { computed, reactive, ref, type Ref } from 'vue'
import { api } from '@/services/api'
import type { DescribeResult, IndexInfo } from '@/types'
import { useAppStore } from '@/stores/app'

type Params = {
  markActive: () => void
  d1ExecutionMode?: Ref<'dev' | 'remote'>
}

export const useEntityDetails = ({ markActive, d1ExecutionMode = ref<'dev' | 'remote'>('remote') }: Params) => {
  const store = useAppStore()

  const entityPattern = ref('')
  const entityDetail = ref<DescribeResult | null>(null)
  const entityDetails = reactive<Record<string, DescribeResult | null>>({})
  const entityDetailsLoading = reactive<Record<string, boolean>>({})
  const entityDetailsError = reactive<Record<string, string>>({})
  const expandedEntity = ref<string | null>(null)
  const expandedEntityView = ref<'fields' | 'indexes' | 'stats'>('fields')

  const filteredEntities = computed(() => {
    const pattern = entityPattern.value.trim().toLowerCase()
    if (!pattern) return store.entities
    return store.entities.filter((item) => item.toLowerCase().includes(pattern))
  })

  const entityDetailRequests = new Map<string, Promise<DescribeResult>>()

  const mysqlIndexRank = (idx: IndexInfo) => {
    const name = String(idx?.name || '').trim().toLowerCase()
    if (name === 'primary') return 0
    if (idx.unique) return 1
    return 2
  }

  const sortMysqlIndexesForDisplay = (indexes: IndexInfo[]) => {
    return indexes
      .map((idx, originalIndex) => ({ idx, originalIndex }))
      .sort((a, b) => mysqlIndexRank(a.idx) - mysqlIndexRank(b.idx) || a.originalIndex - b.originalIndex)
      .map(({ idx }) => idx)
  }

  const clearEntityDetailsCache = () => {
    Object.keys(entityDetails).forEach((key) => delete entityDetails[key])
    Object.keys(entityDetailsLoading).forEach((key) => delete entityDetailsLoading[key])
    Object.keys(entityDetailsError).forEach((key) => delete entityDetailsError[key])
    entityDetailRequests.clear()
  }

  const seedEntityDetails = (detailsByName: Record<string, DescribeResult | null | undefined>) => {
    Object.entries(detailsByName || {}).forEach(([rawName, rawDetail]) => {
      const name = String(rawName || '').trim()
      if (!name || !rawDetail) return
      const normalizedDetail =
        store.current?.type === 'mysql' && Array.isArray(rawDetail.indexes)
          ? { ...rawDetail, indexes: sortMysqlIndexesForDisplay(rawDetail.indexes) }
          : rawDetail
      entityDetails[name] = normalizedDetail
      entityDetailsLoading[name] = false
      entityDetailsError[name] = ''
    })
  }

  const isEntityExpanded = (name: string) => expandedEntity.value === name

  const fetchEntityDetails = async (name: string, skipCache = false) => {
    if (!store.current) {
      throw new Error('Datasource not selected.')
    }
    const cached = entityDetails[name]
    if (!skipCache && cached && !entityDetailsError[name]) return cached

    if (skipCache) {
      delete entityDetails[name]
      delete entityDetailsError[name]
      delete entityDetailsLoading[name]
    }

    const existing = entityDetailRequests.get(name)
    if (existing && !skipCache) return await existing

    entityDetailsLoading[name] = true
    entityDetailsError[name] = ''
    const promise = (store.current?.type === 'd1'
      ? api.describeEntity(
          store.current.id,
          name,
          store.mongoDatabase,
          d1ExecutionMode.value,
        )
      : api.describeEntity(
          store.current.id,
          name,
          store.mongoDatabase,
        ))
      .then((detail) => {
        const normalizedDetail =
          store.current?.type === 'mysql' && Array.isArray(detail?.indexes)
            ? { ...detail, indexes: sortMysqlIndexesForDisplay(detail.indexes) }
            : detail

        entityDetails[name] = normalizedDetail
        entityDetailsError[name] = ''
        markActive()
        return normalizedDetail
      })
      .catch((err) => {
        const message = err instanceof Error ? err.message : String(err)
        entityDetails[name] = null
        entityDetailsError[name] = message
        throw err
      })
      .finally(() => {
        entityDetailsLoading[name] = false
        entityDetailRequests.delete(name)
      })

    entityDetailRequests.set(name, promise)
    return await promise
  }

  const toggleEntityExpanded = async (name: string) => {
    if (expandedEntity.value === name) {
      expandedEntity.value = null
      return
    }
    expandedEntity.value = name
    expandedEntityView.value = 'fields'
    try {
      await fetchEntityDetails(name)
    } catch {
      // Errors are displayed inline in the expand panel.
    }
  }

  const indexFieldList = (idx: IndexInfo) => {
    const rawDefinition = idx.definition || ''
    let raw = idx.column || ''
    if (!raw && rawDefinition) {
      const open = rawDefinition.lastIndexOf('(')
      const close = rawDefinition.lastIndexOf(')')
      if (open !== -1 && close !== -1 && close > open) {
        const inside = rawDefinition.slice(open + 1, close).trim()
        if (inside) {
          raw = inside
        }
      }
      if (!raw) raw = rawDefinition
    }
    if (!raw) return ['-']
    const matches = raw.match(/[A-Za-z0-9_.$]+(?=\\s*:)/g)
    if (matches && matches.length) return matches
    const parts = raw
      .split(',')
      .map((entry) => entry.trim())
      .filter(Boolean)
    return parts.length ? parts : [raw.trim()]
  }

  const indexKindLabel = (idx: IndexInfo) => {
    if (idx.name === 'PRIMARY') return 'PK'
    if (idx.unique) return 'UQ'
    return 'IDX'
  }

  const indexKindClass = (idx: IndexInfo) => {
    if (idx.name === 'PRIMARY') return 'pk'
    if (idx.unique) return 'unique'
    return 'index'
  }

  return {
    entityPattern,
    entityDetail,
    entityDetails,
    entityDetailsLoading,
    entityDetailsError,
    expandedEntity,
    expandedEntityView,
    filteredEntities,
    clearEntityDetailsCache,
    seedEntityDetails,
    isEntityExpanded,
    fetchEntityDetails,
    toggleEntityExpanded,
    indexFieldList,
    indexKindLabel,
    indexKindClass,
  }
}
