import { computed, ref, type ComputedRef, type Ref } from 'vue'
import { api } from '@/services/api'
import { useAppStore } from '@/stores/app'

type Params = {
  entityPattern: Ref<string>
  isMongo: ComputedRef<boolean>
  markActive: () => void
}

export const useMongoDatabaseMode = ({ entityPattern, isMongo, markActive }: Params) => {
  const store = useAppStore()

  const mongoDatabases = ref<string[]>([])
  const mongoDatabaseError = ref('')
  const mongoDatabaseMode = computed(() => store.mongoDatabaseMode)

  const showMongoDatabaseSwitch = computed(
    () => isMongo.value && store.mongoDatabaseSelectable && !mongoDatabaseMode.value,
  )

  const filteredDatabases = computed(() => {
    const pattern = entityPattern.value.trim().toLowerCase()
    if (!pattern) return mongoDatabases.value
    return mongoDatabases.value.filter((name) => name.toLowerCase().includes(pattern))
  })

  const loadMongoDatabases = async () => {
    if (!store.current) return
    try {
      mongoDatabaseError.value = ''
      mongoDatabases.value = await api.listDatabases(store.current.id, entityPattern.value)
      markActive()
    } catch (err) {
      mongoDatabaseError.value = err instanceof Error ? err.message : String(err)
    }
  }

  const enterMongoDatabaseMode = async () => {
    if (!store.current || store.current.type !== 'mongodb') return
    store.mongoDatabaseMode = true
    mongoDatabases.value = []
    mongoDatabaseError.value = ''
    await loadMongoDatabases()
  }

  const applyMongoDatabaseSelection = (name: string) => {
    const trimmed = name.trim()
    if (!trimmed) return false
    store.mongoDatabase = trimmed
    store.mongoDatabaseDraft = trimmed
    if (store.current) {
      store.mongoDatabaseByDatasource[store.current.id] = trimmed
    }
    store.mongoDatabaseSelectable = false
    store.mongoDatabaseMode = false
    return true
  }

  return {
    mongoDatabases,
    mongoDatabaseError,
    mongoDatabaseMode,
    showMongoDatabaseSwitch,
    filteredDatabases,
    loadMongoDatabases,
    enterMongoDatabaseMode,
    applyMongoDatabaseSelection,
  }
}
