export function deriveMongoDisplay(ds: any) {
  const databaseLabel = ds?.database || mongoDatabaseFromOptions(ds?.options) || '-'
  const hostLabel = mongoHostLabelFromOptions(ds?.host, ds?.port, ds?.options) || '-'
  return { hostLabel, databaseLabel }
}

export function mongoHostLabelFromOptions(host?: string, port?: number, options?: Record<string, any>) {
  if (host && port) {
    return `${host}:${port}`
  }
  if (options?.hosts && Array.isArray(options.hosts) && options.hosts.length) {
    return options.hosts.join(',')
  }
  if (options?.uri) {
    const parsed = parseMongoURI(String(options.uri))
    if (parsed.hosts) {
      return parsed.hosts
    }
  }
  return ''
}

export function mongoDatabaseFromOptions(options?: Record<string, any>) {
  if (!options?.uri) {
    return ''
  }
  const parsed = parseMongoURI(String(options.uri))
  return parsed.db || ''
}

export function mongoDatabaseFromDatasource(ds: any) {
  return ds?.database || mongoDatabaseFromOptions(ds?.options) || ''
}

export function parseMongoURI(uri: string) {
  const input = (uri || '').trim()
  const schemeIdx = input.indexOf('://')
  if (schemeIdx === -1) {
    return { hosts: '', db: '' }
  }
  let rest = input.slice(schemeIdx + 3)
  const at = rest.lastIndexOf('@')
  if (at !== -1) {
    rest = rest.slice(at + 1)
  }
  const slash = rest.indexOf('/')
  if (slash === -1) {
    return { hosts: rest, db: '' }
  }
  const hosts = rest.slice(0, slash)
  let path = rest.slice(slash + 1)
  const q = path.indexOf('?')
  if (q !== -1) {
    path = path.slice(0, q)
  }
  const db = path.split('/')[0] || ''
  return { hosts, db }
}

export function inferMongoConnMode(ds: any) {
  if (!ds || ds.type !== 'mongodb') {
    return 'userpass'
  }
  if (ds?.options?.uri) {
    return 'uri'
  }
  return 'userpass'
}

export function applyMongoFormOptions(base: Record<string, any> | undefined, form: {
  mode: string
  uri: string
  tls: boolean
  sslEnabled: boolean
  sslrootcert: string
  replicaSet: string
  hosts: string
}) {
  const options = base && typeof base === 'object' ? { ...base } : {}
  options.sslEnabled = Boolean(form.sslEnabled)
  const certificate = form.sslrootcert.trim()
  if (form.mode === 'uri') {
    const uri = form.uri.trim()
    if (uri) {
      options.uri = uri
    } else {
      delete options.uri
    }
    delete options.tls
    if (options.sslEnabled && certificate) {
      options.sslrootcert = certificate
    } else {
      delete options.sslrootcert
    }
    delete options.hosts
    delete options.replicaSet
    return options
  }
  delete options.uri
  if (form.sslEnabled || form.tls) {
    options.tls = true
  } else {
    delete options.tls
  }
  if (options.sslEnabled && certificate) {
    options.sslrootcert = certificate
  } else {
    delete options.sslrootcert
  }
  const replicaSet = form.replicaSet.trim()
  if (replicaSet) {
    options.replicaSet = replicaSet
  } else {
    delete options.replicaSet
  }
  const hostsRaw = form.hosts.trim()
  const hosts = hostsRaw
    ? hostsRaw
        .split(',')
        .map((item) => item.trim())
        .filter(Boolean)
    : []
  if (hosts.length) {
    options.hosts = hosts
  } else {
    delete options.hosts
  }
  return options
}
