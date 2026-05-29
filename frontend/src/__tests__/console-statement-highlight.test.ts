import { describe, expect, it } from 'vitest'
import { buildStatementHighlightHtml } from '@/views/console/utils/statementHighlight'

describe('console statement syntax highlight', () => {
  it('highlights SQL keywords and numbers with bold token classes', () => {
    const html = buildStatementHighlightHtml('SELECT * FROM users LIMIT 50;', 'mysql')
    expect(html).toContain('statement-token-keyword-sql')
    expect(html).toContain('>SELECT<')
    expect(html).toContain('statement-token-number')
    expect(html).toContain('>50<')
  })

  it('highlights Mongo operators and methods', () => {
    const html = buildStatementHighlightHtml('db.users.updateOne({}, {$set: {status: "active"}})', 'mongodb')
    expect(html).toContain('statement-token-keyword-mongo')
    expect(html).toContain('>db<')
    expect(html).toContain('statement-token-operator')
    expect(html).toContain('>$set<')
  })

  it('highlights Elasticsearch request verbs', () => {
    const html = buildStatementHighlightHtml('GET /orders/_search', 'elasticsearch')
    expect(html).toContain('statement-token-keyword-es')
    expect(html).toContain('>GET<')
  })
})
