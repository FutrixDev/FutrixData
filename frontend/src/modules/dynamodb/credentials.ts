export type AwsStaticCredentials = {
  accessKeyId: string
  secretAccessKey: string
  sessionToken?: string
}

type ProfileBlock = { accessKeyId?: string; secretAccessKey?: string; sessionToken?: string }

const normalizeAwsKey = (key: string) => key.trim().toLowerCase().replace(/[\s-]+/g, '_')

const stripQuotes = (value: string) => value.replace(/^\"|\"$/g, '').replace(/^'|'$/g, '')

export function parseAwsCredentials(text: string, preferredProfile?: string): { profile: string; credentials: AwsStaticCredentials } {
  const trimmed = String(text || '').trim()
  if (!trimmed) throw new Error('Credentials file is empty.')

  const profiles = parseAwsSharedCredentials(trimmed)
  if (Object.keys(profiles).length) {
    const preferred = String(preferredProfile || '').trim()
    const selectedProfile = preferred && profiles[preferred]
      ? preferred
      : profiles.default
        ? 'default'
        : Object.keys(profiles)[0]

    const block = profiles[selectedProfile]
    const accessKeyId = String(block?.accessKeyId || '').trim()
    const secretAccessKey = String(block?.secretAccessKey || '').trim()
    const sessionToken = String(block?.sessionToken || '').trim()
    if (!accessKeyId || !secretAccessKey) throw new Error('Selected profile is missing access key id or secret access key.')
    return {
      profile: selectedProfile,
      credentials: {
        accessKeyId,
        secretAccessKey,
        ...(sessionToken ? { sessionToken } : {}),
      },
    }
  }

  const csvCreds = parseAwsAccessKeysCsv(trimmed)
  if (csvCreds) {
    return { profile: '', credentials: csvCreds }
  }

  throw new Error('Unrecognized credentials format. Use AWS shared credentials file (~/.aws/credentials) or IAM access keys CSV.')
}

export function parseAwsSharedCredentials(text: string): Record<string, ProfileBlock> {
  const lines = String(text || '').split(/\r?\n/)
  const out: Record<string, ProfileBlock> = {}
  let current = ''

  const ensure = (profile: string) => {
    if (!out[profile]) out[profile] = {}
    return out[profile]
  }

  for (const rawLine of lines) {
    const line = rawLine.trim()
    if (!line) continue
    if (line.startsWith('#') || line.startsWith(';')) continue
    if (line.startsWith('[') && line.endsWith(']')) {
      current = line.slice(1, -1).trim()
      continue
    }
    if (!current) continue

    const idx = line.indexOf('=')
    if (idx === -1) continue
    const key = normalizeAwsKey(line.slice(0, idx))
    const value = stripQuotes(line.slice(idx + 1).trim())
    const block = ensure(current)
    if (key === 'aws_access_key_id') block.accessKeyId = value
    else if (key === 'aws_secret_access_key') block.secretAccessKey = value
    else if (key === 'aws_session_token') block.sessionToken = value
  }
  for (const key of Object.keys(out)) {
    const block = out[key]
    if (!block?.accessKeyId || !block?.secretAccessKey) delete out[key]
  }
  return out
}

export function parseAwsAccessKeysCsv(text: string): AwsStaticCredentials | null {
  const lines = String(text || '').split(/\r?\n/).map((line) => line.trim()).filter(Boolean)
  if (lines.length < 2) return null
  const header = parseCsvLine(lines[0] || '')
  const row = parseCsvLine(lines[1] || '')
  if (header.length < 2 || row.length < 2) return null

  const columnIndex = (want: string) => header.findIndex((h) => normalizeAwsKey(h) === normalizeAwsKey(want))
  const accessIdx = columnIndex('access_key_id')
  const secretIdx = columnIndex('secret_access_key')
  if (accessIdx === -1 || secretIdx === -1) {
    // IAM console CSV uses labels like "Access key ID","Secret access key"
    const accessAlt = header.findIndex((h) => /access\s*key\s*id/i.test(h))
    const secretAlt = header.findIndex((h) => /secret\s*access\s*key/i.test(h))
    if (accessAlt === -1 || secretAlt === -1) return null
    const accessKeyId = String(row[accessAlt] || '').trim()
    const secretAccessKey = String(row[secretAlt] || '').trim()
    if (!accessKeyId || !secretAccessKey) return null
    return { accessKeyId, secretAccessKey }
  }

  const accessKeyId = String(row[accessIdx] || '').trim()
  const secretAccessKey = String(row[secretIdx] || '').trim()
  if (!accessKeyId || !secretAccessKey) return null
  return { accessKeyId, secretAccessKey }
}

function parseCsvLine(line: string): string[] {
  const out: string[] = []
  let cur = ''
  let inQuotes = false
  for (let i = 0; i < line.length; i += 1) {
    const ch = line[i] || ''
    if (ch === '"') {
      if (inQuotes && line[i + 1] === '"') {
        cur += '"'
        i += 1
        continue
      }
      inQuotes = !inQuotes
      continue
    }
    if (ch === ',' && !inQuotes) {
      out.push(cur.trim())
      cur = ''
      continue
    }
    cur += ch
  }
  out.push(cur.trim())
  return out.map((val) => stripQuotes(val))
}
