import { readStringLiteral, splitMongoArgsWithPositions } from '../json'
import { formatMongoFieldKey, isValidMongoIdent } from './ident'
import { extractMongoCollectionName, mongoCollectionRef } from './refs'
import { findMatchingParen } from './parser'

export function findMongoCreateIndexContext(rawValue: string) {
  if (!rawValue) return null
  const trimmedRight = rawValue.replace(/\s+$/, '')
  if (/\.createIndex\s*\(\s*$/i.test(trimmedRight)) return buildMongoCreateIndexContext(rawValue, true)
  if (/\.createIndex\s*$/i.test(trimmedRight)) return buildMongoCreateIndexContext(rawValue, false)
  return null
}

export function findMongoCreateIndexKeyContext(statement: string, caretPos: number) {
  if (!statement || caretPos === null || caretPos === undefined) return null
  const lower = statement.toLowerCase()
  const methodIdx = lower.lastIndexOf('.createindex', caretPos)
  if (methodIdx === -1) return null
  const openParen = statement.indexOf('(', methodIdx)
  if (openParen === -1 || caretPos < openParen) return null
  const closeParen = findMatchingParen(statement, openParen)
  const end = closeParen === -1 ? statement.length : closeParen
  if (caretPos > end) return null
  const argsText = statement.slice(openParen + 1, end)
  const args = splitMongoArgsWithPositions(argsText, openParen + 1)
  let firstArg = args[0]
  if (!firstArg) firstArg = { text: '', start: openParen + 1, end: openParen + 1 }
  if (caretPos < firstArg.start || caretPos > firstArg.end) return null
  const collectionName = extractMongoCollectionName(statement.slice(0, methodIdx))
  return { firstArg, collectionName, base: statement.slice(0, openParen), withParen: true }
}

function buildMongoCreateIndexContext(rawValue: string, withParen: boolean) {
  const lower = rawValue.toLowerCase()
  const idx = lower.lastIndexOf('.createindex')
  const prefix = idx !== -1 ? rawValue.slice(0, idx) : rawValue
  const collectionName = extractMongoCollectionName(prefix)
  const usesGetCollection = /getCollection\s*\(/i.test(prefix) || /db\s*\[/.test(prefix)
  const needsBaseFix = collectionName && !usesGetCollection && !isValidMongoIdent(collectionName)
  const basePrefix = needsBaseFix && collectionName ? `${mongoCollectionRef(collectionName)}.createIndex` : rawValue
  return { base: basePrefix, withParen, collectionName }
}

export function applyCreateIndexFieldSuggestion(statement: string, context: any, field: string) {
  if (!statement || !context || !field) return { value: statement }
  const argStart = context.firstArg.start
  const argEnd = context.firstArg.end
  const argText = statement.slice(argStart, argEnd)
  const trimmed = argText.trim()
  const key = formatMongoFieldKey(field)

  if (!trimmed || !trimmed.includes('{') || !trimmed.includes('}')) {
    const replacement = `{${key}: 1}`
    return { value: statement.slice(0, argStart) + replacement + statement.slice(argEnd), caret: argStart + replacement.length - 1 }
  }

  const openBrace = argText.indexOf('{')
  const closeBrace = argText.lastIndexOf('}')
  if (openBrace === -1 || closeBrace === -1 || closeBrace < openBrace) {
    const replacement = `{${key}: 1}`
    return { value: statement.slice(0, argStart) + replacement + statement.slice(argEnd), caret: argStart + replacement.length - 1 }
  }

  const bodyText = argText.slice(openBrace + 1, closeBrace)
  const hasPlaceholder = /(^|[,{]\s*)("field"|field)\s*:/i.test(bodyText)

  if (!bodyText.trim()) {
    const replacement = `{${key}: 1}`
    return { value: statement.slice(0, argStart) + replacement + statement.slice(argEnd), caret: argStart + replacement.length - 1 }
  }

  if (hasPlaceholder) {
    let i = openBrace + 1
    while (i < closeBrace && /\s/.test(argText[i])) i += 1
    if (i >= closeBrace) {
      const replacement = `{${key}: 1}`
      return { value: statement.slice(0, argStart) + replacement + statement.slice(argEnd), caret: argStart + replacement.length - 1 }
    }

    const keyStart = i
    let keyEnd = i
    if (argText[i] === '"' || argText[i] === "'") {
      try {
        const parsed = readStringLiteral(argText, i)
        keyEnd = parsed.next
      } catch {
        const replacement = `{${key}: 1}`
        return { value: statement.slice(0, argStart) + replacement + statement.slice(argEnd), caret: argStart + replacement.length - 1 }
      }
    } else {
      while (keyEnd < closeBrace && /[A-Za-z0-9_$]/.test(argText[keyEnd])) keyEnd += 1
    }

    const colonPos = argText.indexOf(':', keyEnd)
    if (colonPos === -1) {
      const replacement = `{${key}: 1}`
      return { value: statement.slice(0, argStart) + replacement + statement.slice(argEnd), caret: argStart + replacement.length - 1 }
    }

    const newArgText = argText.slice(0, keyStart) + key + argText.slice(keyEnd)
    return { value: statement.slice(0, argStart) + newArgText + statement.slice(argEnd), caret: argStart + keyStart + key.length }
  }

  const trimmedBody = bodyText.trim()
  const needsComma = trimmedBody && !trimmedBody.endsWith(',')
  const insertion = `${needsComma ? ', ' : ' '}${key}: 1`
  const newArgText = argText.slice(0, closeBrace) + insertion + argText.slice(closeBrace)
  const newValue = statement.slice(0, argStart) + newArgText + statement.slice(argEnd)
  return { value: newValue, caret: argStart + closeBrace + insertion.length - 1 }
}

export function extractMongoIndexFields(indexes: any[]) {
  const set = new Set<string>()
  const list = Array.isArray(indexes) ? indexes : []
  list.forEach((idx) => {
    if (!idx) return
    const cols = String(idx.column || '').split(',').map((s: string) => s.trim()).filter(Boolean)
    if (!cols.length) return
    if (String(idx.name || '') === '_id_' && cols.length === 1 && cols[0] === '_id') return
    cols.forEach((c) => { if (c && c !== '_id') set.add(c) })
  })
  return Array.from(set)
}
