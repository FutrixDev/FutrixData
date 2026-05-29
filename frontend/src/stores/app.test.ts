import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'

import { useAppStore } from './app'

describe('app store datasource entity cache', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('preserves cached entity names verbatim when switching back to a datasource', () => {
    const store = useAppStore()

    store.saveEntityListState('ds_sql', {
      items: [' spaced table ', '  '],
      cursor: '',
      done: true,
      pattern: '',
    })

    store.restoreDatasourceEntityState('ds_sql', '')

    expect(store.entities).toEqual([' spaced table ', '  '])
  })

  it('can restore a cached datasource entity list even when local filter patterns differ', () => {
    const store = useAppStore()

    store.saveEntityListState('ds_es', {
      items: ['futrixdata-demo-1', 'logs-prod-2026'],
      cursor: '',
      done: true,
      pattern: '',
    })

    store.restoreDatasourceEntityState('ds_es', 'demo', { allowPatternMismatch: true })

    expect(store.entities).toEqual(['futrixdata-demo-1', 'logs-prod-2026'])
  })
})
