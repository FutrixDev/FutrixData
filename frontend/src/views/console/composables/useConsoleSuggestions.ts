import { computed, type ComputedRef, type Ref } from 'vue'
import { buildMongoStatement, mongoCollectionMethods } from '@/modules/mongo/core'
import { buildSqlWriteTemplates } from '@/modules/sql/templates'
import { tApp } from '@/modules/i18n/appI18n'
import type { DescribeResult } from '@/types'
import { buildStatement } from '../utils/statements'

type ConsoleSuggestion = { label: string; apply: () => void }

type Params = {
  store: any
  statement: Ref<string>
  templateTarget: Ref<string>
  entityDetail: Ref<DescribeResult | null>
  isMongo: ComputedRef<boolean>
  isSQL: ComputedRef<boolean>
  isDynamo: ComputedRef<boolean>
  mongoDatabaseMode: ComputedRef<boolean>
}

const dynamoDetailValue = (detail: DescribeResult | null, label: string) => {
  const raw = detail?.details?.find((d) => d.label === label)?.value
  return raw === undefined || raw === null ? '' : String(raw)
}

const dynamoQuote = (value: string) => `"${String(value || '').replaceAll('"', '""')}"`

const dynamoIndexKeys = (definition?: string) => {
  const parts = String(definition || '')
    .split('|')
    .map((part) => part.trim())
    .filter(Boolean)

  let partitionKey = ''
  let sortKey = ''

  for (const part of parts) {
    const idx = part.indexOf('=')
    if (idx <= 0) continue
    const key = part.slice(0, idx).trim()
    const value = part.slice(idx + 1).trim()
    if (!key || !value) continue
    if (value === 'HASH') partitionKey = key
    if (value === 'RANGE') sortKey = key
  }

  return { partitionKey, sortKey }
}

export function useConsoleSuggestions({ store, statement, templateTarget, entityDetail, isMongo, isSQL, isDynamo, mongoDatabaseMode }: Params) {
  const mongoSuggestions = computed<ConsoleSuggestion[]>(() => {
    if (!isMongo.value) return []
    if (mongoDatabaseMode.value) {
      return [
        {
          label: 'Create Database',
          apply: () => {
            const dbName = (store.mongoDatabaseDraft || '').trim() || 'my_database'
            const stmt = `db.getSiblingDB(\"${dbName}\").createCollection(\"collection\")`
            statement.value = stmt
          },
        },
        {
          label: 'Create Collection',
          apply: () => {
            statement.value = 'db.createCollection(\"collection\")'
          },
        },
        {
          label: 'Create User',
          apply: () => {
            const dbName = (store.mongoDatabaseDraft || '').trim() || 'admin'
            statement.value = `db.getSiblingDB(\"${dbName}\").createUser({\n  user: \"user\",\n  pwd: \"password\",\n  roles: [{ role: \"readWrite\", db: \"${dbName}\" }],\n})`
          },
        },
      ]
    }
    if (!statement.value.trim()) {
      const target = templateTarget.value || store.selectedEntity || 'collection'
      return mongoCollectionMethods.map((method) => ({
        label: method.label,
        apply: () => {
          statement.value = buildMongoStatement(target, method.snippet)
        },
      }))
    }
    return []
  })

  const sqlSuggestions = computed<ConsoleSuggestion[]>(() => {
    if (!isSQL.value) return []
    if (statement.value.trim()) return []

    const target = (templateTarget.value || store.selectedEntity || '').trim() || 'table'
    const detail = entityDetail.value || undefined
    const writeTemplates = buildSqlWriteTemplates(detail)

    const applyTarget = (tpl: string) => tpl.replaceAll('{{target}}', target)
    const defaultSelect = buildStatement(store.current?.type || 'mysql', target, detail)

    return [
      {
        label: 'SELECT',
        apply: () => {
          statement.value = defaultSelect
        },
      },
      {
        label: 'COUNT',
        apply: () => {
          statement.value = `SELECT COUNT(*) AS count FROM ${target};`
        },
      },
      {
        label: 'INSERT',
        apply: () => {
          statement.value = applyTarget(writeTemplates.insert)
        },
      },
      {
        label: 'UPDATE',
        apply: () => {
          statement.value = applyTarget(writeTemplates.update)
        },
      },
      {
        label: 'DELETE',
        apply: () => {
          statement.value = applyTarget(writeTemplates.delete)
        },
      },
    ]
  })

  const dynamoSuggestions = computed<ConsoleSuggestion[]>(() => {
    if (!isDynamo.value) return []
    if (statement.value.trim()) return []

    const target = (templateTarget.value || store.selectedEntity || '').trim() || 'table'
    const table = dynamoQuote(target)
    const detail = entityDetail.value
    const pk = dynamoDetailValue(detail, 'Partition Key') || 'pk'
    const sk = dynamoDetailValue(detail, 'Sort Key')
    const wherePartitionKey = `${pk} = 'PK#...'`
    const whereItemKey = sk ? `${pk} = 'PK#...' AND ${sk} = 'SK#...'` : wherePartitionKey
    const keyFields = sk ? `'${pk}':'PK#...','${sk}':'SK#...'` : `'${pk}':'PK#...'`

    const suggestions: ConsoleSuggestion[] = [
      {
        label: 'SELECT',
        apply: () => {
          statement.value = `SELECT * FROM ${table} WHERE ${wherePartitionKey}`
        },
      },
      {
        label: 'INSERT',
        apply: () => {
          statement.value = `INSERT INTO ${table} VALUE {${keyFields},'attr':'value'}`
        },
      },
      {
        label: 'UPDATE',
        apply: () => {
          statement.value = `UPDATE ${table} SET attr = 'value' WHERE ${whereItemKey}`
        },
      },
      {
        label: 'DELETE',
        apply: () => {
          statement.value = `DELETE FROM ${table} WHERE ${whereItemKey}`
        },
      },
    ]

    const indexes = Array.isArray(detail?.indexes) ? detail!.indexes : []
    for (const idx of indexes) {
      const name = String(idx?.name || '').trim()
      if (!name) continue
      const keys = dynamoIndexKeys(idx?.definition)
      if (!keys.partitionKey) continue

      const from = `${dynamoQuote(target)}.${dynamoQuote(name)}`
      const where = `${keys.partitionKey} = 'PK#...'`

      suggestions.splice(1, 0, {
        label: `SELECT (${name})`,
        apply: () => {
          statement.value = `SELECT * FROM ${from} WHERE ${where}`
        },
      })
    }

    suggestions.push({
      label: 'SCAN',
      apply: () => {
        statement.value = `SELECT * FROM ${table}`
      },
    })

    return suggestions
  })

  const consoleSuggestions = computed<ConsoleSuggestion[]>(() => {
    if (isMongo.value) return mongoSuggestions.value
    if (isSQL.value) return sqlSuggestions.value
    if (isDynamo.value) return dynamoSuggestions.value
    return []
  })

  const consoleSuggestionsLabel = computed(() => {
    if (isMongo.value) return tApp('console.suggestions.mongoHelpers')
    if (isSQL.value) return tApp('console.suggestions.sqlHelpers')
    if (isDynamo.value) return tApp('console.suggestions.dynamoHelpers')
    return ''
  })

  return { consoleSuggestions, consoleSuggestionsLabel }
}
