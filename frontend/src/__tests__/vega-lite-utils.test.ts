import { describe, expect, it } from 'vitest'

import { enhanceVegaLiteSpec } from '@/utils/vegaLite'

describe('enhanceVegaLiteSpec', () => {
  it('adds schema and responsive sizing defaults', () => {
    const input = {
      data: { values: [{ name: 'A', value: 1 }] },
      mark: 'bar',
      encoding: {
        x: { field: 'name', type: 'nominal' },
        y: { field: 'value', type: 'quantitative' },
      },
    }

    const out = enhanceVegaLiteSpec(input)

    expect(out.$schema).toBe('https://vega.github.io/schema/vega-lite/v5.json')
    expect(out.width).toBe('container')
    expect(out.height).toBe('container')
    expect(out.autosize).toEqual({ type: 'fit', contains: 'padding' })
    expect(out.config).toBeTruthy()
  })

  it('normalizes mark to object', () => {
    const out = enhanceVegaLiteSpec({
      data: { values: [{ name: 'A', value: 1 }] },
      mark: 'line',
      encoding: {
        x: { field: 'name', type: 'nominal' },
        y: { field: 'value', type: 'quantitative' },
      },
    })

    expect(typeof out.mark).toBe('object')
    expect((out.mark as any).type).toBe('line')
  })
})
