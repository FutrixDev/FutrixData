import { describe, expect, it } from 'vitest'

import { buildJsonCodeHighlightHtml, formatJsonCodePanelDraft } from '@/views/console/utils/jsonCodeHighlight'

describe('json code highlight', () => {
  it('highlights json keys, strings, numbers, literals, and braces', () => {
    const html = buildJsonCodeHighlightHtml('{\n  "query": {\n    "size": 10,\n    "active": true,\n    "label": "ok",\n    "fallback": null\n  }\n}')

    expect(html).toContain('elastic-dsl-json-token-brace')
    expect(html).toContain('elastic-dsl-json-token-key')
    expect(html).toContain('&quot;query&quot;')
    expect(html).toContain('elastic-dsl-json-token-number')
    expect(html).toContain('>10<')
    expect(html).toContain('elastic-dsl-json-token-literal')
    expect(html).toContain('>true<')
    expect(html).toContain('>null<')
    expect(html).toContain('elastic-dsl-json-token-string')
    expect(html).toContain('&quot;ok&quot;')
  })

  it('keeps incomplete json editable while still escaping raw text safely', () => {
    const html = buildJsonCodeHighlightHtml('{\n  "query": "oops\n}')

    expect(html).toContain('elastic-dsl-json-token-string')
    expect(html).toContain('&quot;oops')
    expect(html).not.toContain('<script')
  })

  it('keeps short primitive arrays on one line while leaving nested structures expanded', () => {
    const formatted = formatJsonCodePanelDraft({
      query: {
        bool: {
          filter: [
            {
              terms: {
                tag: ['seed', 'doc'],
              },
            },
            {
              bool: {
                should: [
                  { match: { message: 'seed' } },
                  { match: { message: 'doc' } },
                ],
                minimum_should_match: 1,
              },
            },
          ],
        },
      },
    })

    expect(formatted).toContain('"tag": ["seed", "doc"]')
    expect(formatted).toContain('"should": [\n')
  })
})
