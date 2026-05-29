import { describe, expect, it } from 'vitest'

import {
  clearRedisCommandDocsCache,
  formatRedisCommandSyntax,
  getRedisCommandCompletion,
  getRedisCommandSuggestions,
  getRedisInlineHint,
  loadRedisCommandDocs,
  refreshRedisCommandDocs,
} from '../modules/redis/command-docs'

describe('redis command docs', () => {
  it('renders SET syntax with optional args', () => {
    const docs = {
      updatedAt: 0,
      commands: {
        SET: {
          arguments: [
            { name: 'key', type: 'key', display_text: 'key' },
            { name: 'value', type: 'string', display_text: 'value' },
            {
              name: 'condition',
              type: 'oneof',
              optional: true,
              arguments: [
                { name: 'nx', type: 'pure-token', token: 'NX' },
                { name: 'xx', type: 'pure-token', token: 'XX' },
              ],
            },
            { name: 'get', type: 'pure-token', token: 'GET', optional: true },
            {
              name: 'expiration',
              type: 'oneof',
              optional: true,
              arguments: [
                { name: 'seconds', type: 'integer', token: 'EX', display_text: 'seconds' },
                { name: 'milliseconds', type: 'integer', token: 'PX', display_text: 'milliseconds' },
                { name: 'unix-time-seconds', type: 'unix-time', token: 'EXAT', display_text: 'unix-time-seconds' },
                {
                  name: 'unix-time-milliseconds',
                  type: 'unix-time',
                  token: 'PXAT',
                  display_text: 'unix-time-milliseconds',
                },
                { name: 'keepttl', type: 'pure-token', token: 'KEEPTTL' },
              ],
            },
          ],
        },
      },
    }

    const syntax = formatRedisCommandSyntax('set', docs)

    expect(syntax).toBe(
      'SET key value [NX|XX] [GET] [EX seconds|PX milliseconds|EXAT unix-time-seconds|PXAT unix-time-milliseconds|KEEPTTL]',
    )
  })

  it('normalizes lowercase command keys on refresh', async () => {
    clearRedisCommandDocsCache()
    const base = loadRedisCommandDocs()
    const nextDocs = {
      updatedAt: base.updatedAt + 1000,
      commands: {
        set: {
          arguments: [
            { name: 'key', type: 'key', display_text: 'key' },
            { name: 'value', type: 'string', display_text: 'value' },
          ],
        },
      },
    }

    await refreshRedisCommandDocs('ds_redis', async () => nextDocs)

    const updated = loadRedisCommandDocs()
    expect(updated.commands.SET).toBeDefined()
    expect(formatRedisCommandSyntax('set', updated)).toBe('SET key value')
  })

  it('refreshes cached docs asynchronously', async () => {
    clearRedisCommandDocsCache()
    const base = loadRedisCommandDocs()
    const nextDocs = {
      updatedAt: base.updatedAt + 1000,
      commands: {
        PING: { arguments: [] },
      },
    }

    await refreshRedisCommandDocs('ds_1', async () => nextDocs)

    const updated = loadRedisCommandDocs()
    expect(updated.updatedAt).toBe(nextDocs.updatedAt)
    expect(updated.commands.PING).toBeDefined()
  })

  it('builds inline completion suffix', () => {
    const docs = {
      updatedAt: 0,
      commands: {
        SET: {
          arguments: [
            { name: 'key', type: 'key', display_text: 'key' },
            { name: 'value', type: 'string', display_text: 'value' },
          ],
        },
      },
    }

    const completion = getRedisCommandCompletion('SET', docs)

    expect(completion).toBe(' key value')

    const completionAfterKey = getRedisCommandCompletion('SET abc', docs)

    expect(completionAfterKey).toBe(' value')
  })

  it('returns inline hint only when caret at end', () => {
    const docs = {
      updatedAt: 0,
      commands: {
        SET: {
          arguments: [
            { name: 'key', type: 'key', display_text: 'key' },
            { name: 'value', type: 'string', display_text: 'value' },
          ],
        },
      },
    }

    const hintAtEnd = getRedisInlineHint('SET', 3, 3, docs)
    expect(hintAtEnd?.suffix).toBe(' key value')

    const hintMid = getRedisInlineHint('SET', 1, 1, docs)
    expect(hintMid).toBeNull()

    const hintAfterKey = getRedisInlineHint('SET abc', 7, 7, docs)
    expect(hintAfterKey?.suffix).toBe(' value')
  })

  it('returns Redis command suggestions from a command prefix', () => {
    const docs = {
      updatedAt: 0,
      commands: {
        SET: {
          summary: 'Set the string value of a key.',
          arguments: [
            { name: 'key', type: 'key', display_text: 'key' },
            { name: 'value', type: 'string', display_text: 'value' },
          ],
        },
        SETBIT: { summary: 'Sets or clears the bit at offset.' },
        GET: { summary: 'Get the value of a key.' },
      },
    }

    const suggestions = getRedisCommandSuggestions('set', docs)

    expect(suggestions.map((item) => item.command)).toEqual(['SET', 'SETBIT'])
    expect(suggestions[0].syntax).toBe('SET key value')
    expect(getRedisCommandSuggestions('set ', docs)).toEqual([])
    expect(getRedisCommandSuggestions('set key', docs)).toEqual([])
  })

  it('includes official module commands in defaults', () => {
    clearRedisCommandDocsCache()
    const docs = loadRedisCommandDocs()

    expect(docs.commands['JSON.GET']).toBeDefined()
    expect(docs.commands['FT.SEARCH']).toBeDefined()
    expect(docs.commands['TS.ADD']).toBeDefined()
    expect(docs.commands['BF.ADD']).toBeDefined()
    expect(docs.commands['GRAPH.QUERY']).toBeDefined()
    expect(docs.commands['AI.TENSORSET']).toBeDefined()
  })
})
