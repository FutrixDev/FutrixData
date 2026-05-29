import {
  DeleteRedisProtobufSchema,
  GetRedisProtobufSchema,
  ListRedisProtobufSchemas,
  SaveRedisProtobufSchema,
} from '@wailsjs/go/main/App'

import { call } from './core'

export type RedisProtobufSchema = {
  id: string
  datasourceId: string
  name: string
  content: string
  createdAt: string
  updatedAt: string
}

export type RedisProtobufSavePayload = {
  id?: string
  datasourceId: string
  name: string
  content: string
}

export const redisProtobufApi = {
  listRedisProtobufSchemas: (datasourceId: string): Promise<RedisProtobufSchema[]> =>
    call(() => ListRedisProtobufSchemas(datasourceId) as Promise<RedisProtobufSchema[]>),
  getRedisProtobufSchema: (id: string): Promise<RedisProtobufSchema> =>
    call(() => GetRedisProtobufSchema(id) as Promise<RedisProtobufSchema>),
  saveRedisProtobufSchema: (payload: RedisProtobufSavePayload): Promise<RedisProtobufSchema> =>
    call(() => SaveRedisProtobufSchema(payload) as Promise<RedisProtobufSchema>),
  deleteRedisProtobufSchema: (id: string): Promise<boolean> =>
    call(() => DeleteRedisProtobufSchema(id) as Promise<boolean>),
}
