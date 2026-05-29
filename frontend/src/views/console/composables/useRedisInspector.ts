import { computed, ref, type ComputedRef, type Ref } from 'vue'
import { api } from '@/services/api'
import { tApp } from '@/modules/i18n/appI18n'
import {
  buildRedisFullView,
  buildRedisPreview,
  redisFullStatementForType,
  type RedisFullView,
} from '@/modules/redis/common'

type Params = {
  store: any
  isRedis: ComputedRef<boolean>
  entityDetail: Ref<any>
  markActive: () => void
}

export function useRedisInspector({ store, isRedis, entityDetail, markActive }: Params) {
  const redisFullValue = ref<string | null>(null)
  const redisFullView = ref<RedisFullView | null>(null)
  const redisFullError = ref('')
  const redisFullLoading = ref(false)

  const resetRedisFullPreview = () => {
    redisFullValue.value = null
    redisFullView.value = null
    redisFullError.value = ''
    redisFullLoading.value = false
  }

  const redisPreview = computed(() => {
    if (!entityDetail.value?.preview) return null
    return buildRedisPreview(entityDetail.value.preview)
  })

  const redisDetailItems = computed(() => {
    if (!isRedis.value) return []
    const details = entityDetail.value?.details || []
    if (!details.length) return []
    const order = ['Type', 'TTL', 'Size']
    const map = new Map(details.map((item: any) => [item.label, item]))
    const ordered = order.map((label) => map.get(label)).filter(Boolean)
    const extras = details.filter((item: any) => !order.includes(item.label))
    return [...ordered, ...extras]
  })

  const loadRedisFullPreview = async () => {
    if (!store.current || !store.selectedEntity || !redisPreview.value) return
    redisFullLoading.value = true
    redisFullError.value = ''
    try {
      const kind = redisPreview.value.kind
      const fullStatement = redisFullStatementForType(store.selectedEntity, kind)
      const response = await api.executeStatement(store.current.id, fullStatement, store.mongoDatabase, '', 0)
      const row = response.rows?.[0]
      const raw = row && typeof row === 'object' && 'result' in row ? (row as any).result : row
      const view = buildRedisFullView(raw, kind)
      redisFullView.value = view
      // Backwards-compatible: keep the string form for the legacy <pre> in case
      // a consumer still reads redisFullValue. For non-string kinds we set null
      // so templates can branch on the structured view.
      if (kind === 'string') {
        redisFullValue.value = view.isEmpty ? '' : (view.rows[0]?.[0] ?? '')
      } else {
        redisFullValue.value = null
      }
      markActive()
    } catch (err) {
      redisFullError.value = err instanceof Error ? err.message : String(err)
    } finally {
      redisFullLoading.value = false
    }
  }

  const copyRedisKey = async () => {
    if (!store.selectedEntity) return
    try {
      await navigator.clipboard.writeText(store.selectedEntity)
      store.setNotice(tApp('redis.inspector.keyCopied'), 'success')
    } catch (err) {
      store.setNotice(err instanceof Error ? err.message : tApp('common.copyFailed'), 'error')
    }
  }

  return {
    redisPreview,
    redisDetailItems,
    redisFullLoading,
    redisFullValue,
    redisFullView,
    redisFullError,
    resetRedisFullPreview,
    loadRedisFullPreview,
    copyRedisKey,
  }
}
