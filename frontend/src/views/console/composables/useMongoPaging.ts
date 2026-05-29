import { onBeforeUnmount, ref, type ComputedRef, type Ref } from 'vue'
import { api } from '@/services/api'
import type { ExplainResult, QueryResult } from '@/types'
import { useAppStore } from '@/stores/app'

type Params = {
  statement: Ref<string>
  result: Ref<QueryResult | null>
  resultMeta: Ref<string>
  statusMessage: Ref<string>
  statusType: Ref<string>
  explainResult: Ref<ExplainResult | null>
  isMongo: ComputedRef<boolean>
  markActive: () => void
}

export const useMongoPaging = ({
  statement,
  result,
  resultMeta,
  statusMessage,
  statusType,
  explainResult,
  isMongo,
  markActive,
}: Params) => {
  const store = useAppStore()

  const mongoQueryPageSize = 200
  const mongoPagingActive = ref(false)
  const mongoPagingLoading = ref(false)
  const mongoPagingHasNext = ref(false)
  const mongoPagingPageIndex = ref(0)
  const mongoPagingSource = ref('')
  const mongoPagingNextToken = ref('')
  const mongoPagingPrevToken = ref('')

  const mongoPageTip = ref('')
  let mongoPageTipTimer: number | null = null

  const showMongoPageTip = (message: string) => {
    if (mongoPageTipTimer) {
      window.clearTimeout(mongoPageTipTimer)
      mongoPageTipTimer = null
    }
    mongoPageTip.value = message
    mongoPageTipTimer = window.setTimeout(() => {
      mongoPageTip.value = ''
      mongoPageTipTimer = null
    }, 1500)
  }

  const resetMongoPaging = () => {
    mongoPagingActive.value = false
    mongoPagingLoading.value = false
    mongoPagingHasNext.value = false
    mongoPagingPageIndex.value = 0
    mongoPagingSource.value = ''
    mongoPagingNextToken.value = ''
    mongoPagingPrevToken.value = ''
  }

  const loadNextMongoPage = async () => {
    if (!store.current) return
    if (!isMongo.value || explainResult.value) return
    if (!mongoPagingActive.value || !mongoPagingHasNext.value) return
    if (mongoPagingLoading.value) return
    if (!result.value) return
    if (!mongoPagingSource.value) return
    if (!mongoPagingNextToken.value) {
      mongoPagingHasNext.value = false
      return
    }

    mongoPagingLoading.value = true
    try {
      const nextIndex = mongoPagingPageIndex.value + 1
      const data = await api.executeStatement(
        store.current.id,
        mongoPagingSource.value,
        store.mongoDatabase,
        mongoPagingNextToken.value,
        mongoQueryPageSize,
        '',
        true,
      )
      const incoming = data.rows || []
      const nextRows = incoming.length > mongoQueryPageSize ? incoming.slice(0, mongoQueryPageSize) : incoming
      if (nextRows.length === 0) {
        mongoPagingHasNext.value = false
        mongoPagingNextToken.value = ''
        return
      }
      result.value = {
        ...data,
        columns: data.columns || result.value.columns,
        rows: [...(result.value.rows || []), ...nextRows],
        rowCount: (result.value.rows?.length ?? 0) + nextRows.length,
      }
      mongoPagingPageIndex.value = nextIndex
      mongoPagingNextToken.value = data.nextToken || ''
      mongoPagingPrevToken.value = data.prevToken || ''
      mongoPagingHasNext.value = !!data.nextToken
      mongoPagingActive.value = mongoPagingHasNext.value || !!mongoPagingPrevToken.value
      resultMeta.value = `Rows: ${result.value.rowCount} | Page ${mongoPagingPageIndex.value + 1} | ${data.elapsedMs}ms`
      markActive()
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      statusMessage.value = `Failed | ${message}`
      statusType.value = 'failed'
      resultMeta.value = ''
      mongoPagingHasNext.value = false
    } finally {
      mongoPagingLoading.value = false
    }
  }

  onBeforeUnmount(() => {
    if (mongoPageTipTimer) {
      window.clearTimeout(mongoPageTipTimer)
      mongoPageTipTimer = null
    }
  })

  return {
    mongoQueryPageSize,
    mongoPagingActive,
    mongoPagingLoading,
    mongoPagingHasNext,
    mongoPagingPageIndex,
    mongoPagingSource,
    mongoPagingNextToken,
    mongoPagingPrevToken,
    mongoPageTip,
    showMongoPageTip,
    resetMongoPaging,
    loadNextMongoPage,
  }
}
