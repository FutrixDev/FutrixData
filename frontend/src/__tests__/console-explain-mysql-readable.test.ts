import { afterEach, describe, expect, it } from 'vitest'
import { computed, ref } from 'vue'

import { resetAppI18nForTest, setAppLocale } from '@/modules/i18n/appI18n'
import { useConsoleExplain } from '@/views/console/composables/useConsoleExplain'

describe('useConsoleExplain mysql readable narrative', () => {
  afterEach(() => {
    resetAppI18nForTest()
  })

  it('explains index usage, estimated rows, likely rows, and special fields in plain language', () => {
    setAppLocale('zh')

    const statement = ref('SELECT * FROM orders WHERE user_id = 42 ORDER BY created_at DESC LIMIT 100;')
    const explainResult = ref({
      usesIndex: true,
      indexes: ['idx_orders_user_created'],
      stages: ['RANGE SCAN'],
      detail: [
        {
          id: 1,
          select_type: 'SIMPLE',
          table: 'orders',
          type: 'range',
          possible_keys: 'idx_orders_user_created,idx_orders_created',
          key: 'idx_orders_user_created',
          key_len: '8',
          rows: '12000',
          filtered: '5.00',
          Extra: 'Using where; Using filesort',
        },
      ],
    } as any)

    const explain = useConsoleExplain({
      store: { current: { type: 'mysql' } },
      statement,
      explainResult,
      isSQL: computed(() => true),
      isMongo: computed(() => false),
    })

    const lines = explain.explainNarrativeLines.value
    expect(lines).toContain('索引命中：是，命中 idx_orders_user_created。')
    expect(lines).toContain('预计会扫描/处理约 12000 行数据。')
    expect(lines).toContain('按 filtered 估算，实际可能操作约 600 行数据（rows × filtered%）。')
    expect(lines).toContain('访问类型：RANGE（范围扫描，通常会用到索引区间）。')
    expect(lines).toContain('候选索引：idx_orders_user_created、idx_orders_created。')
    expect(lines).toContain('额外信息：Using where（先读取再过滤），Using filesort（需要额外排序，通常说明排序没走索引）。')
  })
})
