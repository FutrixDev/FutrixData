import { afterEach, describe, expect, it } from 'vitest'
import protobuf from 'protobufjs'

import {
  AUTO_DETECT_MAX_BYTES,
  autoDetectMessage,
  clearAutoDetectCache,
  clearProtobufRootCache,
  isLikelyProtobuf,
} from './protobuf'

const userSchema = `
syntax = "proto3";
message User {
  string name = 1;
  int32 age = 2;
}
`

const orderSchema = `
syntax = "proto3";
message Order {
  string order_id = 1;
  int64 total_cents = 2;
  string customer_email = 3;
}
`

const buildBytes = (schema: string, typeName: string, payload: Record<string, unknown>): Uint8Array => {
  const root = protobuf.parse(schema, { keepCase: true }).root
  const type = root.lookupType(typeName)
  return type.encode(type.create(payload)).finish()
}

const toBase64 = (bytes: Uint8Array): string => Buffer.from(bytes).toString('base64')

afterEach(() => {
  clearAutoDetectCache()
  clearProtobufRootCache()
})

describe('isLikelyProtobuf', () => {
  it('accepts well-formed protobuf bytes', () => {
    const bytes = buildBytes(userSchema, 'User', { name: 'Alice', age: 30 })
    expect(isLikelyProtobuf(bytes)).toBe(true)
  })

  it('rejects empty buffers', () => {
    expect(isLikelyProtobuf(new Uint8Array())).toBe(false)
  })

  it('rejects plain ASCII text', () => {
    expect(isLikelyProtobuf(new TextEncoder().encode('hello world'))).toBe(false)
  })

  it('rejects JSON-shaped bytes', () => {
    expect(isLikelyProtobuf(new TextEncoder().encode('{"a":1}'))).toBe(false)
  })

  it('rejects buffers with leading zero tag', () => {
    expect(isLikelyProtobuf(new Uint8Array([0x00, 0x01, 0x02]))).toBe(false)
  })

  it('rejects truncated length-delimited fields', () => {
    expect(isLikelyProtobuf(new Uint8Array([0x0a, 0x05, 0x61]))).toBe(false)
  })
})

describe('autoDetectMessage', () => {
  it('returns null when no schemas are provided', () => {
    const bytes = buildBytes(userSchema, 'User', { name: 'Bob', age: 25 })
    expect(autoDetectMessage(toBase64(bytes), [])).toBeNull()
  })

  it('picks the round-trip-matching message type with high confidence', () => {
    const bytes = buildBytes(userSchema, 'User', { name: 'Alice', age: 30 })
    const result = autoDetectMessage(toBase64(bytes), [
      { schemaId: 'rps_user', schemaName: 'user.proto', content: userSchema },
      { schemaId: 'rps_order', schemaName: 'order.proto', content: orderSchema },
    ])
    expect(result).not.toBeNull()
    expect(result!.schemaId).toBe('rps_user')
    expect(result!.messageType).toBe('User')
    expect(result!.confidence).toBe('high')
  })

  it('returns null for plain text values', () => {
    const result = autoDetectMessage('hello world', [
      { schemaId: 'rps_user', schemaName: 'user.proto', content: userSchema },
    ])
    expect(result).toBeNull()
  })

  it('returns null for values exceeding the size budget', () => {
    const big = Buffer.alloc(AUTO_DETECT_MAX_BYTES + 1, 0x0a).toString('base64')
    const result = autoDetectMessage(big, [
      { schemaId: 'rps_user', schemaName: 'user.proto', content: userSchema },
    ])
    expect(result).toBeNull()
  })

  it('handles raw wire-format input (decoded UTF-8 string)', () => {
    const bytes = buildBytes(userSchema, 'User', { name: 'Carol', age: 7 })
    const text = new TextDecoder().decode(bytes)
    const result = autoDetectMessage(text, [
      { schemaId: 'rps_user', schemaName: 'user.proto', content: userSchema },
    ])
    expect(result).not.toBeNull()
    expect(result!.messageType).toBe('User')
  })

  it('caches results across consecutive calls', () => {
    const bytes = buildBytes(orderSchema, 'Order', {
      order_id: 'o_1',
      total_cents: '1299',
      customer_email: 'a@b.co',
    })
    const sources = [
      { schemaId: 'rps_order', schemaName: 'order.proto', content: orderSchema },
      { schemaId: 'rps_user', schemaName: 'user.proto', content: userSchema },
    ]
    const a = autoDetectMessage(toBase64(bytes), sources)
    const b = autoDetectMessage(toBase64(bytes), sources)
    expect(a).toEqual(b)
    expect(a!.schemaId).toBe('rps_order')
    expect(a!.messageType).toBe('Order')
  })

  it('preserves confidence across cache hits', () => {
    // Two schemas that both round-trip the same bytes; without proper caching
    // the second call would lose the runner-up score and report "high".
    const ambiguousA = `
syntax = "proto3";
message MsgA {
  string id = 1;
  string label = 2;
}
`
    const ambiguousB = `
syntax = "proto3";
message MsgB {
  string id = 1;
  string label = 2;
}
`
    const bytes = buildBytes(ambiguousA, 'MsgA', { id: 'x', label: 'y' })
    const sources = [
      { schemaId: 'rps_a', schemaName: 'a.proto', content: ambiguousA },
      { schemaId: 'rps_b', schemaName: 'b.proto', content: ambiguousB },
    ]
    const first = autoDetectMessage(toBase64(bytes), sources)
    const second = autoDetectMessage(toBase64(bytes), sources)
    expect(first).not.toBeNull()
    expect(second).not.toBeNull()
    expect(second!.confidence).toBe(first!.confidence)
    expect(second!.score).toBe(first!.score)
  })
})
