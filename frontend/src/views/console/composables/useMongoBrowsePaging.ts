import { computed, ref, type ComputedRef, type Ref } from 'vue'
import { buildMongoBrowseStatement as buildMongoBrowseStatementBase } from '../utils/statements'

type Params = {
  statement: Ref<string>
  setStatementSilently: (value: string) => void
  runStatement: (explain: boolean, options: { recordHistory?: boolean; statement?: string }) => Promise<void>
  resultRows: ComputedRef<any[]>
  showMongoPageTip: (message: string) => void
  mongoBrowseActive?: Ref<boolean>
  mongoBrowseCollection?: Ref<string>
  mongoPageSize?: Ref<number>
  mongoPageIndex?: Ref<number>
}

export function useMongoBrowsePaging({
  statement,
  setStatementSilently,
  runStatement,
  resultRows,
  showMongoPageTip,
  mongoBrowseActive: mongoBrowseActiveRef,
  mongoBrowseCollection: mongoBrowseCollectionRef,
  mongoPageSize: mongoPageSizeRef,
  mongoPageIndex: mongoPageIndexRef,
}: Params) {
  const mongoBrowseActive = mongoBrowseActiveRef ?? ref(false)
  const mongoBrowseCollection = mongoBrowseCollectionRef ?? ref('')
  const mongoPageSize = mongoPageSizeRef ?? ref(50)
  const mongoPageIndex = mongoPageIndexRef ?? ref(0)
  const mongoPageSizeOptions = [50, 100, 200, 500, 1000]

  const mongoCanPrev = computed(() => mongoPageIndex.value > 0)
  const mongoCanNext = computed(() => resultRows.value.length >= mongoPageSize.value)

  const buildMongoBrowseStatement = (collection: string) =>
    buildMongoBrowseStatementBase(collection, mongoPageIndex.value, mongoPageSize.value)

  const applyMongoBrowse = async () => {
    if (!mongoBrowseActive.value || !mongoBrowseCollection.value) return
    const stmt = buildMongoBrowseStatement(mongoBrowseCollection.value)
    await runStatement(false, { recordHistory: false, statement: stmt })
  }

  const changeMongoPageSize = async () => {
    mongoPageIndex.value = 0
    if (!mongoBrowseActive.value) return
    await applyMongoBrowse()
  }

  const nextMongoPage = async () => {
    if (!mongoCanNext.value) {
      showMongoPageTip('Last page')
      return
    }
    mongoPageIndex.value += 1
    await applyMongoBrowse()
  }

  const prevMongoPage = async () => {
    if (!mongoCanPrev.value) {
      showMongoPageTip('First page')
      return
    }
    mongoPageIndex.value -= 1
    await applyMongoBrowse()
  }

  return {
    mongoBrowseActive,
    mongoBrowseCollection,
    mongoPageSize,
    mongoPageIndex,
    mongoPageSizeOptions,
    mongoCanPrev,
    mongoCanNext,
    buildMongoBrowseStatement,
    changeMongoPageSize,
    nextMongoPage,
    prevMongoPage,
  }
}
