import { describe, expect, it } from 'vitest'

import { parseRedisCommandArgs } from './command-args'

describe('parseRedisCommandArgs', () => {
  it('preserves quoted Redis arguments and binary escapes', () => {
    expect(parseRedisCommandArgs(String.raw`SET "fd quoted key" "\x00A\nB"`)).toEqual([
      'SET',
      'fd quoted key',
      '\x00A\nB',
    ])
  })

  it('preserves empty quoted arguments and escaped apostrophes', () => {
    expect(parseRedisCommandArgs(String.raw`SET 'user profile' 'Bob\'s value' ""`)).toEqual([
      'SET',
      'user profile',
      "Bob's value",
      '',
    ])
  })

  it('keeps backslashes literal inside single quoted arguments', () => {
    expect(parseRedisCommandArgs(String.raw`SET path 'C:\\tmp'`)).toEqual([
      'SET',
      'path',
      'C:\\\\tmp',
    ])
  })

  it('rejects unterminated quoted arguments', () => {
    expect(() => parseRedisCommandArgs(String.raw`SET "unterminated`)).toThrow(/unterminated/i)
  })

  it('rejects non-whitespace after a closing quote', () => {
    expect(() => parseRedisCommandArgs(String.raw`SET "value"suffix`)).toThrow(/whitespace/i)
  })
})
