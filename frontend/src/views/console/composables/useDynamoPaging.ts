import { computed, onBeforeUnmount, ref, watch, type ComputedRef, type Ref } from 'vue'
import { api } from '@/services/api'
import type { ExplainResult, QueryResult } from '@/types'
import { useAppStore } from '@/stores/app'
import { tApp } from '@/modules/i18n/appI18n'
import { formatDynamoClampedLimitLabels } from '../utils/dynamoLimitLabels'

type Params = {
  statement: Ref<string>
  result: Ref<QueryResult | null>
  resultMeta: Ref<string>
  statusMessage: Ref<string>
  statusType: Ref<string>
  explainResult: Ref<ExplainResult | null>
  isDynamo: ComputedRef<boolean>
  markActive: () => void
}

export const useDynamoPaging = ({
  statement,
  result,
  resultMeta,
  statusMessage,
  statusType,
  explainResult,
  isDynamo,
  markActive,
}: Params) => {
  const store = useAppStore()

  const dynamoQueryPageSize = ref(100)
  const dynamoMaxReturnedRows = ref(100)
  const dynamoMaxPages = ref(5)
  const lastValidPageSize = ref(dynamoQueryPageSize.value)
  const lastValidMaxPages = ref(dynamoMaxPages.value)
  watch(dynamoQueryPageSize, (next) => {
    const n = Number(next)
    if (Number.isFinite(n) && n > 0) lastValidPageSize.value = n
  })
  watch(dynamoMaxPages, (next) => {
    const n = Number(next)
    if (Number.isFinite(n) && n > 0) lastValidMaxPages.value = n
  })
  // Derived requested budget = pageSize × maxPages. The backend reports the
  // effective risk-policy-capped budget in result.detail.
  const dynamoMaxEvaluatedItems = computed(() => {
    const ps = Number(dynamoQueryPageSize.value)
    const mp = Number(dynamoMaxPages.value)
    const effPs = Number.isFinite(ps) && ps > 0 ? ps : lastValidPageSize.value
    const effMp = Number.isFinite(mp) && mp > 0 ? mp : lastValidMaxPages.value
    const product = effPs * effMp
    return product > 0 ? product : 0
  })
  const dynamoPagingActive = ref(false)
  const dynamoPagingLoading = ref(false)
  const dynamoPagingHasNext = ref(false)
  const dynamoPagingPageIndex = ref(0)
  const dynamoPagingSource = ref('')
  const dynamoPagingNextToken = ref('')
  const dynamoPagingPrevToken = ref('')

  const resetDynamoPaging = () => {
    dynamoPagingActive.value = false
    dynamoPagingLoading.value = false
    dynamoPagingHasNext.value = false
    dynamoPagingPageIndex.value = 0
    dynamoPagingSource.value = ''
    dynamoPagingNextToken.value = ''
    dynamoPagingPrevToken.value = ''
  }

  const dynamoPageTip = ref('')
  let dynamoPageTipTimer: number | null = null

  const showDynamoPageTip = (message: string) => {
    if (dynamoPageTipTimer) {
      window.clearTimeout(dynamoPageTipTimer)
      dynamoPageTipTimer = null
    }
    dynamoPageTip.value = message
    dynamoPageTipTimer = window.setTimeout(() => {
      dynamoPageTip.value = ''
      dynamoPageTipTimer = null
    }, 1500)
  }

  const formatDynamoDetailMeta = (data: QueryResult) => {
    const detail = data.detail || {}
    if (!detail || typeof detail !== 'object') return ''
    const parts: string[] = []
    const effectiveLimits = detail.effectiveLimits || {}
    const effectivePageSize = Number(effectiveLimits.pageSize || detail.effectivePageSize || detail.pageSize || 0)
    if (effectivePageSize > 0) parts.push(tApp('console.dynamo.status.pageSize', { pageSize: effectivePageSize }))
    const maxPages = Number(effectiveLimits.maxPages || detail.maxPages || 0)
    if (maxPages > 0) parts.push(tApp('console.dynamo.status.maxPages', { maxPages }))
    const pagesFetched = Number(detail.pagesFetched || 0)
    if (pagesFetched > 0) parts.push(tApp('console.dynamo.status.pagesFetched', { pages: pagesFetched }))
    const clampedLimits = detail.clampedLimits && typeof detail.clampedLimits === 'object' ? detail.clampedLimits : {}
    const clampedLabels = formatDynamoClampedLimitLabels(clampedLimits)
    if (clampedLabels) {
      parts.push(tApp('console.dynamo.status.clampedLimits', { limits: clampedLabels }))
    }
    const stopReason = String(detail.stopReason || '').trim()
    if (stopReason) {
      const stopReasonKey = `console.dynamo.status.stopReason.${stopReason}`
      const stopReasonText = tApp(stopReasonKey)
      parts.push(
        stopReasonText === stopReasonKey
          ? tApp('console.dynamo.status.stopReason.fallback', { stopReason })
          : stopReasonText,
      )
    }
    return parts.length ? ` | ${parts.join(' | ')}` : ''
  }

  const loadNextDynamoPage = async () => {
    if (!store.current) return
    if (!isDynamo.value || explainResult.value) return
    if (!dynamoPagingActive.value || !dynamoPagingHasNext.value) return
    if (dynamoPagingLoading.value) return
    if (!result.value) return
    if (!dynamoPagingSource.value) return
    if (!dynamoPagingNextToken.value) {
      dynamoPagingHasNext.value = false
      return
    }

    dynamoPagingLoading.value = true
    try {
      const nextIndex = dynamoPagingPageIndex.value + 1
      const data = await api.executeStatement(
        store.current.id,
        dynamoPagingSource.value,
        '',
        dynamoPagingNextToken.value,
        dynamoQueryPageSize.value,
        '',
        true,
        {
          maxReturnedRows: dynamoMaxReturnedRows.value,
          maxPages: dynamoMaxPages.value,
          maxEvaluatedItems: dynamoMaxEvaluatedItems.value,
        },
      )
      const incoming = data.rows || []
      result.value = {
        ...data,
        columns: data.columns || result.value.columns,
        rows: [...(result.value.rows || []), ...incoming],
        rowCount: (result.value.rows?.length ?? 0) + incoming.length,
      }
      dynamoPagingPageIndex.value = nextIndex
      dynamoPagingNextToken.value = data.nextToken || ''
      dynamoPagingPrevToken.value = data.prevToken || ''
      dynamoPagingHasNext.value = !!data.nextToken
      dynamoPagingActive.value = dynamoPagingHasNext.value || !!dynamoPagingPrevToken.value
      resultMeta.value = `Rows: ${result.value.rowCount} | Page ${dynamoPagingPageIndex.value + 1} | ${data.elapsedMs}ms${formatDynamoDetailMeta(data)}`
      markActive()
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      statusMessage.value = `Failed | ${message}`
      statusType.value = 'failed'
      resultMeta.value = ''
      dynamoPagingHasNext.value = false
    } finally {
      dynamoPagingLoading.value = false
    }
  }

  onBeforeUnmount(() => {
    if (dynamoPageTipTimer) {
      window.clearTimeout(dynamoPageTipTimer)
      dynamoPageTipTimer = null
    }
  })

  return {
    dynamoQueryPageSize,
    dynamoMaxReturnedRows,
    dynamoMaxPages,
    dynamoMaxEvaluatedItems,
    dynamoPagingActive,
    dynamoPagingLoading,
    dynamoPagingHasNext,
    dynamoPagingPageIndex,
    dynamoPagingSource,
    dynamoPagingNextToken,
    dynamoPagingPrevToken,
    dynamoPageTip,
    showDynamoPageTip,
    resetDynamoPaging,
    loadNextDynamoPage,
  }
}
