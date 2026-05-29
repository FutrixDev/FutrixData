import type { AiContextChip } from '@/types/ai-chat'
import type { DataSource } from '@/types'
import { tApp } from '@/modules/i18n/appI18n'

type BuildContextInput = {
  datasources: Array<Pick<DataSource, 'id' | 'name' | 'type'>>
  currentDatasourceId?: string
  currentDatabase?: string
  currentEntity?: string
}

type ContextGroup = { title: string; items: AiContextChip[] }

const mockEntityFor = (type: string) => {
  if (type === 'mongodb') return ['events', 'profiles', 'logs']
  if (type === 'redis') return ['session:*', 'cache:*', 'queue:*']
  return ['users', 'orders', 'payments']
}

export const buildContextGroups = (input: BuildContextInput): ContextGroup[] => {
  const currentItems: AiContextChip[] = []
  if (input.currentDatabase) {
    currentItems.push({
      id: `db:${input.currentDatabase}`,
      label: input.currentDatabase,
      kind: 'database',
      datasourceId: input.currentDatasourceId,
    })
  }
  if (input.currentEntity) {
    currentItems.push({
      id: `entity:${input.currentEntity}`,
      label: input.currentEntity,
      kind: 'table',
      datasourceId: input.currentDatasourceId,
    })
  }

  const restInDatasource: AiContextChip[] = []
  const otherDatasource: AiContextChip[] = []

  input.datasources.forEach((ds) => {
    const items = mockEntityFor(ds.type)
    items.forEach((name) => {
      const chip: AiContextChip = {
        id: `${ds.id}:${name}`,
        label: `${ds.name}/${name}`,
        kind: ds.type === 'mongodb' ? 'collection' : 'table',
        datasourceId: ds.id,
      }
      if (ds.id === input.currentDatasourceId) {
        restInDatasource.push(chip)
      } else {
        otherDatasource.push(chip)
      }
    })
  })

  const byLabel = (a: AiContextChip, b: AiContextChip) => a.label.localeCompare(b.label)
  restInDatasource.sort(byLabel)
  otherDatasource.sort(byLabel)

  const groups: ContextGroup[] = []
  if (currentItems.length) groups.push({ title: tApp('ai.contextGroup.current'), items: currentItems })
  if (restInDatasource.length) {
    groups.push({ title: tApp('ai.contextGroup.otherInDatasource'), items: restInDatasource })
  }
  if (otherDatasource.length) groups.push({ title: tApp('ai.contextGroup.otherDatasources'), items: otherDatasource })
  return groups
}

export type { BuildContextInput, ContextGroup }
