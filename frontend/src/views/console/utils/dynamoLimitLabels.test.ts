import { beforeEach, describe, expect, it } from 'vitest'

import { resetAppI18nForTest, setAppLocale } from '@/modules/i18n/appI18n'
import { formatDynamoClampedLimitLabels } from './dynamoLimitLabels'

describe('dynamo limit labels', () => {
  beforeEach(() => {
    resetAppI18nForTest()
    setAppLocale('en')
  })

  it('formats clamped limit keys with localized labels', () => {
    expect(formatDynamoClampedLimitLabels({
      maxPages: true,
      maxEvaluatedItems: true,
    })).toBe('page limit, evaluated item limit')

    setAppLocale('zh')
    expect(formatDynamoClampedLimitLabels({
      maxPages: true,
      maxEvaluatedItems: true,
    })).toBe('翻页次数上限, 评估项上限')
  })

  it('ignores unknown or inactive clamped limit keys', () => {
    expect(formatDynamoClampedLimitLabels({
      pageSize: false,
      unexpectedLimit: true,
    })).toBe('')
  })
})
