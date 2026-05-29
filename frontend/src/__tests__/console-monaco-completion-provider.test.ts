import { describe, expect, it } from 'vitest'
import { buildMonacoCompletionPayload } from '@/components/consoleMonacoCompletion'

describe('console monaco completion provider payload', () => {
  it('returns SQL keyword suggestions for select prefix', () => {
    const payload = buildMonacoCompletionPayload({
      statement: 'sel',
      cursorOffset: 3,
      datasourceType: 'mysql',
      entities: ['table_0001'],
      entityDetail: null,
      entityDetailsMap: {},
      activeEntity: 'table_0001',
    })

    const labels = payload.items.map((item) => item.label)
    expect(labels).toContain('SELECT')
  })

  it('returns mongo db method suggestions for db dot context', () => {
    const payload = buildMonacoCompletionPayload({
      statement: 'db.',
      cursorOffset: 3,
      datasourceType: 'mongodb',
      entities: ['events'],
      entityDetail: null,
      entityDetailsMap: {},
      activeEntity: 'events',
    })

    const labels = payload.items.map((item) => item.label)
    expect(labels).toContain('createCollection()')
  })

  it('returns bracket mongo collection completion with valid accessor insertion text', () => {
    const payload = buildMonacoCompletionPayload({
      statement: 'db["us',
      cursorOffset: 6,
      datasourceType: 'mongodb',
      entities: ['users'],
      entityDetail: null,
      entityDetailsMap: {},
      activeEntity: 'users',
    })

    const usersItem = payload.items.find((item) => item.label === 'users')
    expect(usersItem?.insertText).toBe('users"].')
  })

  it('replaces trailing auto-closed bracket tokens in mongo bracket context', () => {
    const statement = 'db["us"]'
    const payload = buildMonacoCompletionPayload({
      statement,
      cursorOffset: 6,
      datasourceType: 'mongodb',
      entities: ['users'],
      entityDetail: null,
      entityDetailsMap: {},
      activeEntity: 'users',
    })

    const usersItem = payload.items.find((item) => item.label === 'users')
    expect(usersItem?.insertText).toBe('users"].')
    expect(payload.insertStart).toBe(4)
    expect(payload.insertEnd).toBe(8)

    const replaced = statement.slice(0, payload.insertStart) + usersItem!.insertText + statement.slice(payload.insertEnd)
    expect(replaced).toBe('db["users"].')
  })

  it('avoids duplicate dots when replacing bracket collection before chained method', () => {
    const statement = 'db["us"].find({})'
    const payload = buildMonacoCompletionPayload({
      statement,
      cursorOffset: 6,
      datasourceType: 'mongodb',
      entities: ['users'],
      entityDetail: null,
      entityDetailsMap: {},
      activeEntity: 'users',
    })

    const usersItem = payload.items.find((item) => item.label === 'users')
    expect(usersItem?.insertText).toBe('users"].')

    const replaced = statement.slice(0, payload.insertStart) + usersItem!.insertText + statement.slice(payload.insertEnd)
    expect(replaced).toBe('db["users"].find({})')
  })

  it('returns elastic index suggestions for path context', () => {
    const payload = buildMonacoCompletionPayload({
      statement: 'GET /',
      cursorOffset: 5,
      datasourceType: 'elasticsearch',
      entities: ['futrixdata-demo-1'],
      entityDetail: null,
      entityDetailsMap: {},
      activeEntity: 'futrixdata-demo-1',
    })

    const labels = payload.items.map((item) => item.label)
    expect(labels).toContain('futrixdata-demo-1')
  })
})
