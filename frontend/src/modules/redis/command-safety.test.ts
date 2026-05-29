import { describe, expect, it } from 'vitest'

import { getRedisCommandRisk } from './command-safety'

describe('getRedisCommandRisk', () => {
  it('flags KEYS commands', () => {
    expect(getRedisCommandRisk('KEYS *')?.id).toBe('keys')
    expect(getRedisCommandRisk('KEYS user:*')?.id).toBe('keys')
  })

  it('flags large scans', () => {
    expect(getRedisCommandRisk('SCAN 0')?.id).toBe('scan')
    expect(getRedisCommandRisk('SCAN 0 MATCH *')?.id).toBe('scan')
    expect(getRedisCommandRisk('SCAN 0 COUNT 1000')?.id).toBe('scan')
    expect(getRedisCommandRisk('SSCAN key 0')?.id).toBe('scan')
  })

  it('allows scoped scans', () => {
    expect(getRedisCommandRisk('SCAN 0 MATCH user:* COUNT 100')).toBeNull()
    expect(getRedisCommandRisk('SSCAN key 0 MATCH field:* COUNT 200')).toBeNull()
  })

  it('flags flush and monitor commands', () => {
    expect(getRedisCommandRisk('FLUSHALL')?.id).toBe('flush')
    expect(getRedisCommandRisk('FLUSHDB')?.id).toBe('flush')
    expect(getRedisCommandRisk('MONITOR')?.id).toBe('monitor')
  })

  it('flags dangerous admin commands', () => {
    expect(getRedisCommandRisk('CLIENT PAUSE 1000')?.id).toBe('client_pause')
    expect(getRedisCommandRisk('SCRIPT KILL')?.id).toBe('script_kill')
    expect(getRedisCommandRisk('CONFIG SET requirepass foo')?.id).toBe('config_set')
    expect(getRedisCommandRisk('SHUTDOWN')?.id).toBe('shutdown')
  })
})
