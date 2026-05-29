import { isValidMongoIdent } from './ident'

export function mongoCollectionRef(name: string) {
  if (!name) return 'db.collection'
  if (isValidMongoIdent(name)) return `db.${name}`
  return `db.getCollection(\"${name}\")`
}

export function extractMongoCollectionName(statement: string) {
  const getCollectionMatch = statement.match(/db\s*\.?\s*getCollection\s*\(\s*(['"])(.*?)\1\s*\)/i)
  if (getCollectionMatch) return getCollectionMatch[2]
  const bracketMatch = statement.match(/db\s*\[\s*(['"])(.*?)\1\s*\]/i)
  if (bracketMatch) return bracketMatch[2]
  const dotMatch = statement.match(/db\s*\.\s*([A-Za-z_$][A-Za-z0-9_$]*)/)
  if (dotMatch) return dotMatch[1]
  const looseMatch = statement.match(/db\s*\.\s*([^\.\s\(]+)/)
  if (looseMatch) return looseMatch[1]
  return ''
}
