import { afterEach, describe, expect, it } from 'vitest'
import { computed, ref } from 'vue'

import { resetAppI18nForTest, setAppLocale } from '@/modules/i18n/appI18n'
import { useConsoleExplain } from '@/views/console/composables/useConsoleExplain'

describe('useConsoleExplain postgres readable narrative', () => {
  afterEach(() => {
    resetAppI18nForTest()
  })

  it('shows estimated rows and asks for ANALYZE when actual rows are unavailable', () => {
    setAppLocale('en')

    const statement = ref('SELECT * FROM users ORDER BY id DESC LIMIT 50;')
    const explainResult = ref({
      usesIndex: true,
      indexes: ['users_pkey'],
      stages: ['Limit', 'Index Scan Backward'],
      detail: [
        {
          Plan: {
            'Node Type': 'Limit',
            'Plan Rows': 50,
            Plans: [
              {
                'Node Type': 'Index Scan',
                'Scan Direction': 'Backward',
                'Index Name': 'users_pkey',
                'Plan Rows': 10000,
                'Index Cond': '(id IS NOT NULL)',
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
    expect(lines).toContain('Index condition: (id IS NOT NULL).')
  })

  it('explains seq scan impact and actual rows from ANALYZE', () => {
    setAppLocale('en')

    const statement = ref('SELECT * FROM users u JOIN orders o ON o.user_id = u.id WHERE u.status = \'active\';')
    const explainResult = ref({
      usesIndex: false,
      indexes: ['idx_orders_user_id'],
      stages: ['Nested Loop', 'Index Scan', 'Seq Scan'],
      detail: [
        {
          Plan: {
            'Node Type': 'Nested Loop',
            'Plan Rows': 5000,
            'Actual Rows': 4200,
            'Actual Loops': 1,
            Plans: [
              {
                'Node Type': 'Index Scan',
                'Relation Name': 'orders',
                'Index Name': 'idx_orders_user_id',
                'Plan Rows': 5000,
                'Actual Rows': 4200,
                'Actual Loops': 1,
                'Index Cond': '(o.user_id = u.id)',
              },
              {
                'Node Type': 'Seq Scan',
                'Relation Name': 'users',
                'Plan Rows': 1000,
                'Actual Rows': 800,
                'Actual Loops': 300,
                Filter: "(u.status = 'active')",
                'Rows Removed by Filter': 120000,
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
    expect(lines).toContain('Index usage: partial. Indexes idx_orders_user_id appear in plan, but some steps still do full scans.')
    expect(lines).toContain('Estimated rows to scan/process: about 5000.')
    expect(lines).toContain('Actual rows processed (ANALYZE): about 240000.')
    expect(lines).toContain('Seq Scan on users: this step scans the whole table.')
    expect(lines).toContain('Index condition: (o.user_id = u.id).')
    expect(lines).toContain("Filter condition: (u.status = 'active').")
    expect(lines).toContain('Rows removed by filter: about 120000.')
  })
})
