import protobuf from 'protobufjs'

export const notProtobufValueMessage = 'Not a Protobuf value.'

type DecodeRedisProtobufResult = {
  isProtobuf: boolean
  lines: string[]
  message: string
}

const toNotProtobuf = (): DecodeRedisProtobufResult => ({
  isProtobuf: false,
  lines: [''],
  message: notProtobufValueMessage,
})

const toLookupCandidates = (name: string): string[] => {
  const trimmed = String(name || '').trim()
  if (!trimmed) return []
  if (trimmed.startsWith('.')) return [trimmed, trimmed.slice(1)]
  return [trimmed, `.${trimmed}`]
}

const bytesToHex = (bytes: Uint8Array): string => {
  return Array.from(bytes).map((byte) => byte.toString(16).padStart(2, '0')).join('')
}

const decodeBase64 = (text: string): Uint8Array | null => {
  try {
    if (typeof atob === 'function') {
      const decoded = atob(text)
      return Uint8Array.from(decoded, (char) => char.charCodeAt(0))
    }
  } catch {
    return null
  }
  try {
    if (typeof Buffer !== 'undefined') {
      return Uint8Array.from(Buffer.from(text, 'base64'))
    }
  } catch {
    return null
  }
  return null
}

const decodeHex = (text: string): Uint8Array | null => {
  const normalized = text.startsWith('0x') ? text.slice(2) : text
  if (normalized.length === 0 || normalized.length % 2 !== 0) return null
  if (!/^[0-9a-fA-F]+$/.test(normalized)) return null
  const bytes = new Uint8Array(normalized.length / 2)
  for (let i = 0; i < normalized.length; i += 2) {
    const byte = Number.parseInt(normalized.slice(i, i + 2), 16)
    if (!Number.isFinite(byte)) return null
    bytes[i / 2] = byte
  }
  return bytes
}

const unwrapQuotedText = (text: string): string => {
  if (text.length < 2) return text
  const first = text[0]
  const last = text[text.length - 1]
  if ((first !== '"' && first !== "'") || first !== last) return text
  try {
    if (first === '"') return JSON.parse(text)
  } catch {
    // Fall through to a conservative Redis-style unquote below.
  }
  return text.slice(1, -1).replace(/\\(["'\\])/g, '$1')
}

const normalizeBase64Text = (text: string): string | null => {
  const compact = String(text || '').trim().replace(/\s+/g, '')
  if (compact.length < 2) return null
  const unquoted = unwrapQuotedText(compact)
  const normalized = unquoted.replace(/-/g, '+').replace(/_/g, '/')
  if (!/^[A-Za-z0-9+/]+={0,2}$/.test(normalized)) return null
  if (/=/.test(normalized.slice(0, -2))) return null
  const withoutPadding = normalized.replace(/=+$/, '')
  if (withoutPadding.length < 2) return null
  const remainder = withoutPadding.length % 4
  if (remainder === 1) return null
  return withoutPadding + '='.repeat((4 - remainder) % 4)
}

const listByteCandidates = (raw: string): Uint8Array[] => {
  const text = String(raw ?? '')
  const trimmed = text.trim()
  const unquotedTrimmed = unwrapQuotedText(trimmed)
  const candidates: Uint8Array[] = []
  const seen = new Set<string>()

  const add = (bytes: Uint8Array | null) => {
    if (!bytes) return
    const normalized = Uint8Array.from(bytes)
    const signature = bytesToHex(normalized)
    if (seen.has(signature)) return
    seen.add(signature)
    candidates.push(normalized)
  }

  if (/^(?:0x)?[0-9a-fA-F]+$/.test(trimmed)) {
    add(decodeHex(trimmed))
  }
  if (unquotedTrimmed !== trimmed && /^(?:0x)?[0-9a-fA-F]+$/.test(unquotedTrimmed)) {
    add(decodeHex(unquotedTrimmed))
  }

  const base64Text = normalizeBase64Text(trimmed)
  if (base64Text) {
    add(decodeBase64(base64Text))
  }

  add(new TextEncoder().encode(text))
  if (unquotedTrimmed !== trimmed) {
    add(new TextEncoder().encode(unquotedTrimmed))
  }

  return candidates
}

const parseRoot = (schema: string): protobuf.Root | null => {
  const text = String(schema || '').trim()
  if (!text) return null
  try {
    return protobuf.parse(text, { keepCase: true }).root
  } catch {
    return null
  }
}

// LRU cache of parsed Roots keyed by a cheap content fingerprint. Schemas
// rarely change but get re-parsed often when the user clicks between keys,
// so caching shaves real time off auto-detect.
const ROOT_CACHE_MAX = 32
const rootCache = new Map<string, protobuf.Root | null>()

const fingerprintSchema = (schema: string): string => {
  const text = String(schema || '')
  let h = 0x811c9dc5
  for (let i = 0; i < text.length; i++) {
    h ^= text.charCodeAt(i)
    h = (h + ((h << 1) + (h << 4) + (h << 7) + (h << 8) + (h << 24))) >>> 0
  }
  return `${text.length.toString(36)}_${h.toString(16)}`
}

const getCachedRoot = (schema: string): protobuf.Root | null => {
  const key = fingerprintSchema(schema)
  if (rootCache.has(key)) {
    const cached = rootCache.get(key) ?? null
    rootCache.delete(key)
    rootCache.set(key, cached)
    return cached
  }
  const root = parseRoot(schema)
  rootCache.set(key, root)
  if (rootCache.size > ROOT_CACHE_MAX) {
    const oldest = rootCache.keys().next().value
    if (oldest) rootCache.delete(oldest)
  }
  return root
}

export const clearProtobufRootCache = () => {
  rootCache.clear()
}

export function extractProtoMessageTypes(schema: string): string[] {
  const root = getCachedRoot(schema)
  if (!root) return []

  const out: string[] = []
  const walk = (scope: protobuf.NamespaceBase, prefix: string) => {
    const nested = Array.isArray(scope.nestedArray) ? scope.nestedArray : []
    for (const item of nested) {
      if (item instanceof protobuf.Type) {
        out.push(`${prefix}${item.name}`)
      }
      if (item instanceof protobuf.Namespace) {
        walk(item, `${prefix}${item.name}.`)
      }
    }
  }

  walk(root, '')
  return out
}

const normalizeDecoded = (type: protobuf.Type, payload: protobuf.Message<{}>): Record<string, unknown> => {
  return type.toObject(payload, {
    longs: String,
    enums: String,
    bytes: String,
    defaults: false,
    arrays: true,
    objects: true,
    oneofs: true,
  }) as Record<string, unknown>
}

const decodeWithType = (type: protobuf.Type, input: Uint8Array): protobuf.Message<{}> | null => {
  try {
    return type.decode(Uint8Array.from(input))
  } catch {
    return null
  }
}

export function decodeRedisProtobufValue(rawValue: string, schema: string, messageName: string): DecodeRedisProtobufResult {
  const root = getCachedRoot(schema)
  if (!root) return toNotProtobuf()

  let type: protobuf.Type | null = null
  for (const candidate of toLookupCandidates(messageName)) {
    const lookedUp = root.lookup(candidate)
    if (lookedUp instanceof protobuf.Type) {
      type = lookedUp
      break
    }
  }
  if (!type) return toNotProtobuf()

  const values = listByteCandidates(rawValue)
  for (const bytes of values) {
    const decoded = decodeWithType(type, bytes)
    if (!decoded) continue
    const obj = normalizeDecoded(type, decoded)
    return {
      isProtobuf: true,
      lines: JSON.stringify(obj, null, 2).split('\n'),
      message: '',
    }
  }
  return toNotProtobuf()
}

// ---------------- Auto-detect ----------------

// Wire-format pre-filter: walk bytes assuming protobuf framing and bail out on
// obvious non-protobuf payloads. Permissive — a true positive only means
// "worth trying decode candidates", not "definitely protobuf".
export function isLikelyProtobuf(bytes: Uint8Array): boolean {
  if (!bytes || bytes.length === 0) return false
  let i = 0
  let fields = 0
  while (i < bytes.length) {
    let tag = 0
    let shift = 0
    let read = 0
    while (i < bytes.length && read < 5) {
      const b = bytes[i++]
      tag |= (b & 0x7f) << shift
      read++
      if ((b & 0x80) === 0) break
      shift += 7
    }
    if (read === 0) return false
    if (read >= 5 && (bytes[i - 1] & 0x80) !== 0) return false
    const wireType = tag & 0x7
    const fieldNumber = tag >>> 3
    if (fieldNumber === 0) return false
    if (fieldNumber > 536870911) return false
    switch (wireType) {
      case 0: {
        let count = 0
        while (i < bytes.length && count < 10) {
          const b = bytes[i++]
          count++
          if ((b & 0x80) === 0) break
        }
        if (count === 0) return false
        if (count >= 10 && (bytes[i - 1] & 0x80) !== 0) return false
        break
      }
      case 1:
        if (i + 8 > bytes.length) return false
        i += 8
        break
      case 2: {
        let len = 0
        let lshift = 0
        let lread = 0
        while (i < bytes.length && lread < 5) {
          const b = bytes[i++]
          len |= (b & 0x7f) << lshift
          lread++
          if ((b & 0x80) === 0) break
          lshift += 7
        }
        if (lread === 0) return false
        if (len < 0 || i + len > bytes.length) return false
        i += len
        break
      }
      case 5:
        if (i + 4 > bytes.length) return false
        i += 4
        break
      default:
        return false
    }
    fields++
    if (fields > 4096) return false
  }
  return fields > 0 && i === bytes.length
}

const bytesEqual = (a: Uint8Array, b: Uint8Array): boolean => {
  if (a.length !== b.length) return false
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) return false
  }
  return true
}

const countPopulatedFields = (obj: Record<string, unknown>): number => {
  let count = 0
  for (const key of Object.keys(obj)) {
    const value = (obj as Record<string, unknown>)[key]
    if (value === null || value === undefined) continue
    if (Array.isArray(value)) {
      if (value.length === 0) continue
    } else if (typeof value === 'string') {
      if (value.length === 0) continue
    }
    count++
  }
  return count
}

export type AutoDetectResult = {
  schemaId: string
  schemaName: string
  messageType: string
  confidence: 'high' | 'medium' | 'low'
  score: number
} | null

export type AutoDetectSource = {
  schemaId: string
  schemaName: string
  content: string
}

export const AUTO_DETECT_MAX_BYTES = 64 * 1024

const AUTO_CACHE_MAX = 64
const autoCache = new Map<string, AutoDetectResult>()

const fingerprintBytes = (bytes: Uint8Array): string => {
  let h = 0x811c9dc5
  for (let i = 0; i < bytes.length; i++) {
    h ^= bytes[i]
    h = (h + ((h << 1) + (h << 4) + (h << 7) + (h << 8) + (h << 24))) >>> 0
  }
  return `${bytes.length.toString(36)}_${h.toString(16)}`
}

const fingerprintSources = (sources: AutoDetectSource[]): string => {
  return sources
    .map((s) => `${s.schemaId}:${fingerprintSchema(s.content)}`)
    .sort()
    .join('|')
}

type RawMatch = {
  type: protobuf.Type
  source: AutoDetectSource
  score: number
  roundTrip: boolean
}

const scoreMatch = (type: protobuf.Type, bytes: Uint8Array): { score: number; roundTrip: boolean } | null => {
  const decoded = decodeWithType(type, bytes)
  if (!decoded) return null
  let obj: Record<string, unknown>
  try {
    obj = normalizeDecoded(type, decoded)
  } catch {
    return null
  }
  let reencoded: Uint8Array
  try {
    reencoded = type.encode(decoded).finish()
  } catch {
    return { score: 1 + countPopulatedFields(obj) * 10, roundTrip: false }
  }
  const roundTrip = bytesEqual(reencoded, bytes)
  const populated = countPopulatedFields(obj)
  const lenRatio = bytes.length === 0 ? 0 : reencoded.length / bytes.length
  const proximity = Math.max(0, 1 - Math.abs(1 - lenRatio))
  const declared = type.fieldsArray.length
  const coverage = declared === 0 ? 0 : populated / declared
  const score = (roundTrip ? 1000 : 0) + populated * 10 + proximity * 50 + coverage * 200
  return { score, roundTrip }
}

const collectTypes = (root: protobuf.Root): protobuf.Type[] => {
  const out: protobuf.Type[] = []
  const walk = (scope: protobuf.NamespaceBase) => {
    const nested = Array.isArray(scope.nestedArray) ? scope.nestedArray : []
    for (const item of nested) {
      if (item instanceof protobuf.Type) out.push(item)
      if (item instanceof protobuf.Namespace) walk(item)
    }
  }
  walk(root)
  return out
}

const detectForBytes = (bytes: Uint8Array, sources: AutoDetectSource[]): AutoDetectResult => {
  let bestForBytes: RawMatch | null = null
  let secondForBytes = -1
  for (const source of sources) {
    const root = getCachedRoot(source.content)
    if (!root) continue
    for (const type of collectTypes(root)) {
      const result = scoreMatch(type, bytes)
      if (!result) continue
      if (!bestForBytes || result.score > bestForBytes.score) {
        if (bestForBytes) secondForBytes = bestForBytes.score
        bestForBytes = { type, source, score: result.score, roundTrip: result.roundTrip }
      } else if (result.score > secondForBytes) {
        secondForBytes = result.score
      }
    }
  }

  if (!bestForBytes || bestForBytes.score <= 0) return null
  const margin = bestForBytes.score - secondForBytes
  let confidence: 'high' | 'medium' | 'low' | null = null
  if (bestForBytes.roundTrip && margin >= 50) confidence = 'high'
  else if (bestForBytes.roundTrip) confidence = 'medium'
  else if (margin >= 30) confidence = 'low'
  if (!confidence) return null

  return {
    schemaId: bestForBytes.source.schemaId,
    schemaName: bestForBytes.source.schemaName,
    messageType: bestForBytes.type.fullName.replace(/^\./, ''),
    confidence,
    score: bestForBytes.score,
  }
}

export function autoDetectMessage(rawValue: string, sources: AutoDetectSource[]): AutoDetectResult {
  if (!Array.isArray(sources) || sources.length === 0) return null
  const candidates = listByteCandidates(rawValue)
  if (candidates.length === 0) return null

  const usable = candidates.filter((b) => b.length > 0 && b.length <= AUTO_DETECT_MAX_BYTES)
  if (usable.length === 0) return null

  const sourcesKey = fingerprintSources(sources)

  let best: AutoDetectResult = null

  for (const bytes of usable) {
    if (!isLikelyProtobuf(bytes)) continue

    const cacheKey = `${fingerprintBytes(bytes)}::${sourcesKey}`
    let outcome: AutoDetectResult
    if (autoCache.has(cacheKey)) {
      outcome = autoCache.get(cacheKey) ?? null
      autoCache.delete(cacheKey)
      autoCache.set(cacheKey, outcome)
    } else {
      outcome = detectForBytes(bytes, sources)
      autoCache.set(cacheKey, outcome)
      if (autoCache.size > AUTO_CACHE_MAX) {
        const oldest = autoCache.keys().next().value
        if (oldest) autoCache.delete(oldest)
      }
    }

    if (outcome && (!best || outcome.score > best.score)) {
      best = outcome
    }
  }

  return best
}

export const clearAutoDetectCache = () => {
  autoCache.clear()
}
