import { afterEach, describe, expect, it } from 'vitest'
import { computed, ref } from 'vue'

import { resetAppI18nForTest, setAppLocale } from '@/modules/i18n/appI18n'
import { useConsoleExplain } from '@/views/console/composables/useConsoleExplain'

describe('useConsoleExplain postgres json detail', () => {
  afterEach(() => {
    resetAppI18nForTest()
  })

  it('renders stage and index narrative from normalized postgres explain result', () => {
    setAppLocale('en')

    const statement = ref('SELECT * FROM users ORDER BY id DESC LIMIT 50;')
    const explainResult = ref({
      usesIndex: true,
      indexes: ['users_pkey'],
      stages: ['Limit', 'Index Scan Backward'],
      totalDocsExamined: 10000,
      detail: [
        {
          Plan: {
            'Node Type': 'Limit',
            'Plan Rows': 50,
            Plans: [
              {
                'Node Type': 'Index Scan Backward',
                'Index Name': 'users_pkey',
                'Plan Rows': 10000,
              },
            ],
          },
        },
      ],
    } as any)

    const explain = useConsoleExplain({
      store: { current: { type: 'postgresql' } },
      statement,
      explainResult,
      isSQL: computed(() => true),
      isMongo: computed(() => false),
    })

    const lines = explain.explainNarrativeLines.value
    expect(lines).toContain('Index usage: yes, hit users_pkey.')
    expect(lines).toContain('Estimated rows to scan/process: about 10000.')
    expect(lines).toContain('Actual rows need EXPLAIN ANALYZE. Turn on Analyze to measure them.')
    expect(lines).toContain('Main operators: Limit, Index Scan Backward (backward index walk).')
    expect(explain.explainNarrative.value).not.toContain('Detailed interpretation is not available yet.')
    expect(explain.explainDetailJson.value).toContain('"Index Name": "users_pkey"')
  })
})
