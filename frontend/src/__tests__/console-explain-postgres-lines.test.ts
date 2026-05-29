import { afterEach, describe, expect, it } from 'vitest'
import { computed, ref } from 'vue'

import { resetAppI18nForTest, setAppLocale } from '@/modules/i18n/appI18n'
import { useConsoleExplain } from '@/views/console/composables/useConsoleExplain'

describe('useConsoleExplain postgres line detail', () => {
  afterEach(() => {
    resetAppI18nForTest()
  })

  it('renders stage + index narrative instead of generic fallback', () => {
    setAppLocale('en')

    const statement = ref('SELECT * FROM users ORDER BY id DESC LIMIT 50;')
    const explainResult = ref({
      usesIndex: true,
      indexes: ['users_pkey'],
      stages: ['Limit', 'Index Scan Backward'],
      detail: [
        'Limit  (cost=0.28..2.01 rows=50 width=92)',
        '  ->  Index Scan Backward using users_pkey on users  (cost=0.28..345.27 rows=10000 width=92)',
      ],
    } as any)

    const explain = useConsoleExplain({
      store: { current: { type: 'postgresql' } },
      statement,
      explainResult,
      isSQL: computed(() => true),
      isMongo: computed(() => false),
    })

    expect(explain.explainNarrativeLines.value).toEqual([
      'Execution stages: Limit -> Index Scan Backward.',
      'Indexes used: users_pkey.',
    ])
    expect(explain.explainNarrative.value).not.toContain('Detailed interpretation is not available yet.')
  })

  it('does not claim no index when index names are present in mixed plans', () => {
    setAppLocale('en')

    const statement = ref('SELECT * FROM users WHERE email = ? LIMIT 1;')
    const explainResult = ref({
      usesIndex: false,
      indexes: ['idx_users_email'],
      stages: ['Seq Scan', 'Index Scan'],
      detail: [
        'Seq Scan on users  (cost=0.00..12.50 rows=1 width=4)',
        'Index Scan using idx_users_email on users  (cost=0.00..8.27 rows=1 width=4)',
      ],
    } as any)

    const explain = useConsoleExplain({
      store: { current: { type: 'postgresql' } },
      statement,
      explainResult,
      isSQL: computed(() => true),
      isMongo: computed(() => false),
    })

    expect(explain.explainNarrativeLines.value).toEqual([
      'Execution stages: Seq Scan -> Index Scan.',
      'Indexes observed in plan: idx_users_email.',
    ])
    expect(explain.explainNarrative.value).not.toContain('No index is used currently')
  })
})
