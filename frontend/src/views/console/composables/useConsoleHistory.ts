import { computed, nextTick, ref, watch, type ComputedRef, type Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '@/services/api'
import { useAppStore } from '@/stores/app'
import { tApp } from '@/modules/i18n/appI18n'
import type { HistoryEntry } from '@/types'

type Params = {
  historyTarget: ComputedRef<string>
  historyDatabase: ComputedRef<string>
  templateTarget: Ref<string>
  statement: Ref<string>
  setStatementSilently: (value: string) => void
  focusStatementEnd: () => void
}

export const useConsoleHistory = ({
  historyTarget,
  historyDatabase,
  templateTarget,
  statement,
  setStatementSilently,
  focusStatementEnd,
}: Params) => {
  const store = useAppStore()
  const route = useRoute()
  const router = useRouter()

  const HISTORY_PREVIEW_LIMIT = 1
  const HISTORY_FETCH_LIMIT = HISTORY_PREVIEW_LIMIT + 1

  const historyItems = ref<HistoryEntry[]>([])
  const historyId = computed(() => (typeof route.query?.historyId === 'string' ? route.query.historyId : ''))
  const historyHasMore = computed(() => historyItems.value.length > HISTORY_PREVIEW_LIMIT)
  const visibleHistoryItems = computed(() => historyItems.value.slice(0, HISTORY_PREVIEW_LIMIT))

  const loadHistoryForConsole = async () => {
    if (!store.current) {
      historyItems.value = []
      return
    }
    const target = historyTarget.value.trim()
    const database = historyDatabase.value.trim()
    const filter: Record<string, string | number> = {
      datasourceId: store.current.id,
      limit: HISTORY_FETCH_LIMIT,
    }
    if (target) {
      filter.target = target
    }
    if (database) {
      filter.database = database
    }
    try {
      historyItems.value = await api.listHistory(filter)
    } catch {
      historyItems.value = []
    }
  }

  const addHistory = async (id: string, stmt: string) => {
    const trimmed = stmt.trim()
    if (!trimmed) return
    const database = historyDatabase.value.trim()
    try {
      await api.appendHistory({
        datasourceId: id,
        statement: trimmed,
        database: database || undefined,
      })
    } catch (err) {
      store.setNotice(err instanceof Error ? err.message : String(err), 'error')
    }
    await loadHistoryForConsole()
  }

  const openHistory = () => {
    if (!store.current) return
    const target = historyTarget.value.trim()
    const database = historyDatabase.value.trim()
    const query: Record<string, string> = { datasourceId: store.current.id }
    if (target) query.target = target
    if (database) query.database = database
    router.push({ name: 'history', query })
  }

  const applyHistory = (stmt: string) => {
    statement.value = stmt
  }

  const applyHistoryFromRoute = async () => {
    const id = historyId.value.trim()
    if (!id || !store.current) return
    try {
      const entry = await api.getHistory(id)
      if (entry.datasourceId && entry.datasourceId !== store.current.id) {
        store.setNotice(tApp('console.history.entryDatasourceMismatch'), 'error')
        return
      }
      if (store.current.type === 'mongodb' && entry.database) {
        store.mongoDatabase = entry.database
        store.mongoDatabaseDraft = entry.database
        store.mongoDatabaseSelectable = false
        store.mongoDatabaseMode = false
      }
      const target = entry.targets?.[0] || ''
      templateTarget.value = target
      store.selectedEntity = target
      setStatementSilently(entry.statement || '')
      await nextTick()
      focusStatementEnd()
    } catch (err) {
      store.setNotice(err instanceof Error ? err.message : String(err), 'error')
    }
  }

  watch([historyTarget, historyDatabase], async () => {
    if (route.name === 'console' && store.current) {
      await loadHistoryForConsole()
    }
  })

  watch(historyId, async () => {
    if (route.name === 'console') {
      await applyHistoryFromRoute()
    }
  })

  return {
    historyItems,
    historyId,
    historyHasMore,
    visibleHistoryItems,
    loadHistoryForConsole,
    addHistory,
    openHistory,
    applyHistory,
    applyHistoryFromRoute,
  }
}
