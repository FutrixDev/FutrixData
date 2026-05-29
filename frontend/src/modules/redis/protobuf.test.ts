import { describe, expect, it } from 'vitest'
import protobuf from 'protobufjs'

import { decodeRedisProtobufValue, extractProtoMessageTypes } from './protobuf'

const schema = `
syntax = "proto3";

message UserEvent {
  string user_id = 1;
  int32 score = 2;
}
`

describe('redis protobuf helpers', () => {
  it('extracts message names from editable proto schema', () => {
    expect(extractProtoMessageTypes('')).toEqual([])
    expect(extractProtoMessageTypes(schema)).toEqual(['UserEvent'])
  })

  it('decodes protobuf value from base64 payload', () => {
    const root = protobuf.parse(schema, { keepCase: true }).root
    const type = root.lookupType('UserEvent')
    const bytes = type.encode(type.create({ user_id: 'u_1', score: 42 })).finish()
    const base64 = Buffer.from(bytes).toString('base64')

    const result = decodeRedisProtobufValue(base64, schema, 'UserEvent')
    expect(result.isProtobuf).toBe(true)
    expect(result.message).toBe('')
    expect(result.lines.join('\n')).toContain('"user_id": "u_1"')
    expect(result.lines.join('\n')).toContain('"score": 42')
  })

  it('decodes protobuf value from unpadded base64 payload', () => {
    const root = protobuf.parse(schema, { keepCase: true }).root
    const type = root.lookupType('UserEvent')
    const bytes = type.encode(type.create({ user_id: '434', score: 7 })).finish()
    const base64 = Buffer.from(bytes).toString('base64').replace(/=+$/, '')

    const result = decodeRedisProtobufValue(base64, schema, 'UserEvent')
    expect(result.isProtobuf).toBe(true)
    expect(result.message).toBe('')
    expect(result.lines.join('\n')).toContain('"user_id": "434"')
    expect(result.lines.join('\n')).toContain('"score": 7')
  })

  it('decodes protobuf value from quoted base64 payload copied through Redis CLI style text', () => {
    const packagedSchema = `
syntax = "proto3";

package futrix.issue434;

message UserEvent {
  string user_id = 1;
  int32 score = 2;
  string action = 3;
}
`
    const root = protobuf.parse(packagedSchema, { keepCase: true }).root
    const type = root.lookupType('futrix.issue434.UserEvent')
    const bytes = type.encode(type.create({ user_id: 'issue-434', score: 434, action: 'redis-protobuf' })).finish()
    const quotedBase64 = `"${Buffer.from(bytes).toString('base64')}"`

    const result = decodeRedisProtobufValue(quotedBase64, packagedSchema, 'futrix.issue434.UserEvent')
    expect(result.isProtobuf).toBe(true)
    expect(result.message).toBe('')
    expect(result.lines.join('\n')).toContain('"user_id": "issue-434"')
    expect(result.lines.join('\n')).toContain('"score": 434')
    expect(result.lines.join('\n')).toContain('"action": "redis-protobuf"')
  })

  it('decodes protobuf value from raw wire text including leading newline byte', () => {
    const root = protobuf.parse(schema, { keepCase: true }).root
    const type = root.lookupType('UserEvent')
    const bytes = type.encode(type.create({ user_id: 'u_1', score: 7 })).finish()
    const wireText = new TextDecoder().decode(bytes)

    const result = decodeRedisProtobufValue(wireText, schema, 'UserEvent')
    expect(result.isProtobuf).toBe(true)
    expect(result.message).toBe('')
    expect(result.lines.join('\n')).toContain('"user_id": "u_1"')
    expect(result.lines.join('\n')).toContain('"score": 7')
  })

  it('decodes forward-compatible payloads with unknown fields from newer schema', () => {
    const newerSchema = `
syntax = "proto3";

message UserEvent {
  string user_id = 1;
  int32 score = 2;
}
`
    const olderSchema = `
syntax = "proto3";

message UserEvent {
  string user_id = 1;
}
`

    const root = protobuf.parse(newerSchema, { keepCase: true }).root
    const type = root.lookupType('UserEvent')
    const bytes = type.encode(type.create({ user_id: 'u_2', score: 99 })).finish()
    const base64 = Buffer.from(bytes).toString('base64')

    const result = decodeRedisProtobufValue(base64, olderSchema, 'UserEvent')
    expect(result.isProtobuf).toBe(true)
    expect(result.message).toBe('')
    expect(result.lines.join('\n')).toContain('"user_id": "u_2"')
  })

  it('preserves int64 precision by rendering long fields as strings', () => {
    const longSchema = `
syntax = "proto3";

message AuditEvent {
  int64 event_id = 1;
}
`
    const maxInt64 = '9223372036854775807'
    const root = protobuf.parse(longSchema, { keepCase: true }).root
    const type = root.lookupType('AuditEvent')
    const bytes = type.encode(type.create({ event_id: maxInt64 })).finish()
    const base64 = Buffer.from(bytes).toString('base64')

    const result = decodeRedisProtobufValue(base64, longSchema, 'AuditEvent')
    expect(result.isProtobuf).toBe(true)
    expect(result.message).toBe('')
    const parsed = JSON.parse(result.lines.join('\n'))
    expect(parsed.event_id).toBe(maxInt64)
  })

  it('treats empty decoded messages as valid when payload has only unknown fields', () => {
    const newerSchema = `
syntax = "proto3";

message CompatEvent {
  int32 extra = 5;
}
`
    const olderSchema = `
syntax = "proto3";

message CompatEvent {}
`
    const root = protobuf.parse(newerSchema, { keepCase: true }).root
    const type = root.lookupType('CompatEvent')
    const bytes = type.encode(type.create({ extra: 1 })).finish()
    const base64 = Buffer.from(bytes).toString('base64')

    const result = decodeRedisProtobufValue(base64, olderSchema, 'CompatEvent')
    expect(result.isProtobuf).toBe(true)
    expect(result.message).toBe('')
    expect(JSON.parse(result.lines.join('\n'))).toEqual({})
  })

  it('treats empty wire payload as valid for empty protobuf message', () => {
    const emptySchema = `
syntax = "proto3";

message EmptyEvent {}
`
    const result = decodeRedisProtobufValue('', emptySchema, 'EmptyEvent')
    expect(result.isProtobuf).toBe(true)
    expect(result.message).toBe('')
    expect(JSON.parse(result.lines.join('\n'))).toEqual({})
  })

  it('returns Not a Protobuf value for non-protobuf payload', () => {
    const result = decodeRedisProtobufValue('hello-world', schema, 'UserEvent')
    expect(result.isProtobuf).toBe(false)
    expect(result.message).toBe('Not a Protobuf value.')
    expect(result.lines).toEqual([''])
  })
})
