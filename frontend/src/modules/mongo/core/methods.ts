import { mongoCollectionRef } from './refs'

export const mongoCollectionMethods = [
  { label: 'find', snippet: 'find({})' },
  { label: 'insertOne', snippet: 'insertOne({})' },
  { label: 'insertMany', snippet: 'insertMany([{}])' },
  { label: 'updateOne', snippet: 'updateOne({}, {$set: {}})' },
  { label: 'updateMany', snippet: 'updateMany({}, {$set: {}})' },
  { label: 'deleteOne', snippet: 'deleteOne({})' },
  { label: 'deleteMany', snippet: 'deleteMany({})' },
  { label: 'aggregate', snippet: 'aggregate([{}])' },
  { label: 'createIndex', snippet: 'createIndex({field: 1}, {unique: true})' },
  { label: 'dropIndex', snippet: 'dropIndex(\"index_name\")' },
]

export const mongoDbMethods = [{ label: 'createCollection', snippet: 'createCollection(\"collection\")' }]

export function buildMongoStatement(collection: string, snippet: string) {
  return `${mongoCollectionRef(collection)}.${snippet}`
}
