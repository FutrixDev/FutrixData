import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/services/api'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import type { DataSource } from '@/types'
import { deriveMongoDisplay } from '@/modules/mongo/datasource'
import { formatDatasourceTypeLabel, normalizeDatasourceType } from '@/modules/datasource/types'
import { tApp } from '@/modules/i18n/appI18n'
import { datasourceLimitNotice, hasReachedDatasourceLimit } from '@/modules/plan/limits'

export function useDatasourceListView() {
  const store = useAppStore()
  const authStore = useAuthStore()
  const router = useRouter()

  const statusValue = (id: string) => store.status[id] || 'unknown'
  const statusDetail = (id: string) => store.statusDetails[id]

  const flashTestId = ref('')
  let flashTimer: number | null = null
  const flashTestResult = (id: string) => {
    flashTestId.value = id
    if (flashTimer) {
      window.clearTimeout(flashTimer)
      flashTimer = null
    }
    flashTimer = window.setTimeout(() => {
      flashTestId.value = ''
      flashTimer = null
    }, 1600)
  }

  const PROBE_TTL_MS = 30 * 60 * 1000
  const isExpired = (id: string) => {
    const checkedAt = store.statusCheckedAt[id] || 0
    if (!checkedAt) return false
    return Date.now() - checkedAt > PROBE_TTL_MS
  }
  const shouldProbe = (id: string) => {
    const value = statusValue(id)
    if (value === 'failed' || value === 'unknown') return true
    if (value === 'connected') return isExpired(id)
    return false
  }

  const statusLabel = (id: string) => {
    const value = statusValue(id)
    if (value === 'connected') return tApp('status.connected')
    if (value === 'failed') return tApp('status.failed')
    if (value === 'testing') return tApp('status.testing')
    return tApp('status.unknown')
  }

  const statusWeight = (id: string) => {
    const value = statusValue(id)
    if (value === 'failed') return 3
    if (value === 'testing') return 2
    if (value === 'connected') return 1
    return 0
  }

  const statusBadgeClass = (id: string) => {
    const value = statusValue(id)
    if (value === 'connected') return 'connected'
    if (value === 'failed') return 'failed'
    return ''
  }

  const statusClass = (id: string) => {
    const value = statusValue(id)
    if (value === 'connected') return 'status-connected'
    if (value === 'failed') return 'status-failed'
    return ''
  }

  const statusCheckedAtLabel = (id: string) => {
    const checkedAt = store.statusCheckedAt[id]
    if (!checkedAt) return ''
    return new Date(checkedAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  }

  const mongoLabel = (ds: DataSource) => deriveMongoDisplay(ds)
  const datasourceTypeLabel = (value: string) => formatDatasourceTypeLabel(normalizeDatasourceType(value))
  const datasourceTypeClass = (value: string) => {
    const rawType = String(value || 'unknown').trim().toLowerCase().replace(/[^a-z0-9]+/g, '_')
    return `datasource-type--${rawType || 'unknown'}`
  }
  const nonEmptyString = (value: unknown) => (typeof value === 'string' ? value.trim() : '')
  const sensitiveCredentialKeys = new Set(['user', 'username', 'password', 'passwd', 'pwd', 'pass'])
  const redactCredentialAssignments = (value: string) =>
    value.replace(
      /(^|[\s;?&])([a-z_][a-z0-9_]*)(\s*=\s*)('[^']*'|"[^"]*"|[^;\s&]+)/gi,
      (_match, prefix: string, key: string, separator: string, rawValue: string) => {
        if (!sensitiveCredentialKeys.has(key.toLowerCase())) return `${prefix}${key}${separator}${rawValue}`
        const quoted = rawValue.length >= 2 && (rawValue.startsWith('"') || rawValue.startsWith("'")) && rawValue[0] === rawValue[rawValue.length - 1]
        const redactedValue = quoted ? `${rawValue[0]}***${rawValue[0]}` : '***'
        return `${prefix}${key}${separator}${redactedValue}`
      },
    )
  const redactDSNCredentials = (value: string) => {
    const queryIndex = value.indexOf('?')
    const parseEnd = queryIndex >= 0 ? queryIndex : value.length
    const atIndex = value.lastIndexOf('@', parseEnd - 1)
    if (atIndex <= 0) return value
    return value.slice(atIndex + 1)
  }
  const redactUriUserInfo = (value: string) => {
    const trimmed = String(value || '').trim()
    if (!trimmed) return ''
    if (trimmed.includes('@') && !trimmed.includes('://')) {
      return redactCredentialAssignments(redactDSNCredentials(trimmed))
    }
    try {
      const parsed = new URL(trimmed)
      let changed = false
      if (parsed.username || parsed.password) {
        parsed.username = ''
        parsed.password = ''
        changed = true
      }
      const keys = Array.from(new Set(Array.from(parsed.searchParams.keys())))
      for (const key of keys) {
        if (!sensitiveCredentialKeys.has(key.toLowerCase())) continue
        const values = parsed.searchParams.getAll(key)
        if (values.length !== 1 || values[0] !== '***') changed = true
        parsed.searchParams.delete(key)
        parsed.searchParams.append(key, '***')
      }
      const redacted = changed ? parsed.toString() : trimmed
      return redactCredentialAssignments(redacted)
    } catch {
      const schemeRedacted = trimmed.replace(/^([a-z][a-z0-9+.-]*:\/\/)[^@/]*@/i, '$1')
      if (schemeRedacted !== trimmed) return redactCredentialAssignments(schemeRedacted)
      return redactCredentialAssignments(redactDSNCredentials(trimmed))
    }
  }

  const databaseMetaLabel = (ds: DataSource) => {
    const normalized = normalizeDatasourceType(ds.type)
    if (normalized === 'mongodb') {
      const label = mongoLabel(ds).databaseLabel
      return label ? tApp('datasource.meta.databaseLabel', { value: label }) : ''
    }
    if (normalized === 'mysql' || normalized === 'postgresql') {
      const db = ds.database ? String(ds.database).trim() : ''
      return db ? tApp('datasource.meta.databaseLabel', { value: db }) : ''
    }
    if (normalized === 'd1') {
      const db = String(ds.options?.databaseName || ds.database || ds.options?.databaseId || '').trim()
      return db ? tApp('datasource.meta.databaseLabel', { value: db }) : ''
    }
    return ''
  }

  const endpointLabel = (ds: DataSource) => {
    const normalized = normalizeDatasourceType(ds.type)
    if (normalized === 'mongodb') {
      const uri = ds.options?.uri ? String(ds.options.uri) : ''
      if (uri) return uri
      if (Array.isArray(ds.options?.hosts) && ds.options.hosts.length) return ds.options.hosts.join(',')
    }
    if (normalized === 'mysql' || normalized === 'postgresql') {
      const uri = nonEmptyString(ds.options?.uri)
      if (uri) return redactUriUserInfo(uri)
    }
    if (normalized === 'dynamodb') {
      const endpoint = ds.options?.endpoint ? String(ds.options.endpoint).trim() : ''
      if (endpoint) return endpoint
      const region = ds.options?.region ? String(ds.options.region).trim() : ''
      return region
    }
    if (normalized === 'd1') {
      const mode = String(ds.options?.mode || '').trim().toLowerCase()
      if (mode === 'local') {
        const binding = String(ds.options?.binding || '').trim()
        return binding
      }
      return tApp('datasource.list.d1Endpoint')
    }
    const host = ds.host ? String(ds.host) : ''
    if (!host) return ''
    const port = ds.port ? Number(ds.port) : 0
    return port > 0 ? `${host}:${port}` : host
  }

  const copyText = async (text: string, emptyMessage: string) => {
    if (!text) {
      store.setNotice(emptyMessage, 'error')
      return
    }
    try {
      await navigator.clipboard.writeText(text)
      store.setNotice(tApp('common.copied'), 'success')
    } catch (err) {
      store.setNotice(err instanceof Error ? err.message : tApp('common.copyFailed'), 'error')
    }
  }

  const copyEndpoint = async (ds: DataSource) => copyText(endpointLabel(ds), tApp('datasource.list.noEndpointToCopy'))
  const copyStatusDetail = async (ds: DataSource) => copyText(statusDetail(ds.id) || '', tApp('datasource.list.noErrorToCopy'))

  const shouldShowD1ReAuthentication = (ds: DataSource) => {
    if (normalizeDatasourceType(ds.type) !== 'd1') return false
    const authMode = String(ds.options?.authMode || '').trim().toLowerCase()
    if (authMode !== 'token') return false
    return statusValue(ds.id) !== 'connected'
  }

  const filtered = computed(() => {
    const search = store.listSearch.trim().toLowerCase()
    const items = store.datasources.slice()
    const filteredItems = search
      ? items.filter((ds) => `${ds.name} ${ds.host} ${normalizeDatasourceType(ds.type)}`.toLowerCase().includes(search))
      : items

    switch (store.listSort) {
      case 'name-desc':
        filteredItems.sort((a, b) => a.name.localeCompare(b.name)).reverse()
        break
      case 'type-asc':
        filteredItems.sort((a, b) => a.type.localeCompare(b.type))
        break
      case 'status':
        filteredItems.sort((a, b) => statusWeight(a.id) - statusWeight(b.id))
        break
      default:
        filteredItems.sort((a, b) => a.name.localeCompare(b.name))
    }
    return filteredItems
  })

  const openCreate = () => {
    if (hasReachedDatasourceLimit(authStore.effectivePlan, store.datasources.length)) {
      store.setNotice(datasourceLimitNotice(authStore.effectivePlan), 'error')
      return
    }
    store.formMode = 'create'
    store.formId = null
    router.push({ name: 'datasource-create' })
  }

  const editDatasource = (ds: DataSource) => {
    store.formMode = 'edit'
    store.formId = ds.id
    router.push({ name: 'datasource-edit', params: { id: ds.id } })
  }

  const openConsole = (ds: DataSource) => {
    const normalized = ds.type === 'redis_cluster' ? { ...ds, type: 'redis' } : ds
    store.setCurrentDatasource(normalized)
    router.push({ name: 'console', params: { id: ds.id } })
  }

  const testDatasource = async (ds: DataSource, options: { silent?: boolean } = {}) => {
    store.status[ds.id] = 'testing'
    try {
      await api.testDatasource(ds.id)
      store.status[ds.id] = 'connected'
      store.statusDetails[ds.id] = ''
      store.statusCheckedAt[ds.id] = Date.now()
      if (!options.silent) flashTestResult(ds.id)
      try {
        const metrics = await api.getDatasourceMetrics(ds.id)
        if (metrics && (metrics.cpuAvailable || metrics.memoryAvailable || (metrics.warnings || []).length > 0)) {
          store.datasourceMetrics[ds.id] = metrics
        } else {
          delete store.datasourceMetrics[ds.id]
        }
      } catch {
        delete store.datasourceMetrics[ds.id]
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      store.status[ds.id] = 'failed'
      store.statusDetails[ds.id] = message
      store.statusCheckedAt[ds.id] = Date.now()
      delete store.datasourceMetrics[ds.id]
      if (!options.silent) flashTestResult(ds.id)
    }
  }

  const testAll = async () => {
    for (const ds of store.datasources) {
      // eslint-disable-next-line no-await-in-loop
      await testDatasource(ds)
    }
  }

  const autoProbeDatasources = async () => {
    const targets = store.datasources.filter((ds) => shouldProbe(ds.id))
    if (!targets.length) return
    await Promise.allSettled(targets.map((ds) => testDatasource(ds, { silent: true })))
  }

  const deleteTarget = ref<DataSource | null>(null)
  const deleteBusy = ref(false)
  const d1ReAuthenticationLoading = ref<Record<string, boolean>>({})
  const isD1ReAuthenticationLoading = (id: string) => Boolean(d1ReAuthenticationLoading.value[id])
  const dynamoReAuthenticationLoading = ref<Record<string, boolean>>({})
  const isDynamoReAuthenticationLoading = (id: string) => Boolean(dynamoReAuthenticationLoading.value[id])

  const reAuthenticateD1Datasource = async (ds: DataSource) => {
    if (normalizeDatasourceType(ds.type) !== 'd1') return
    if (isD1ReAuthenticationLoading(ds.id)) return
    d1ReAuthenticationLoading.value = { ...d1ReAuthenticationLoading.value, [ds.id]: true }
    try {
      const session = await api.d1OAuthReLogin()
      const token = String((session as any)?.token || '').trim()
      if (!token) {
        store.setNotice(tApp('datasource.list.d1ReAuthenticationNeedToken'), 'error')
        return
      }

      const currentAccountID = String(ds.options?.accountId || '').trim()
      const normalizedAccounts = Array.isArray((session as any)?.accounts)
        ? (session as any).accounts
            .map((item: any) => String(item?.id || item?.accountId || item?.accountTag || '').trim())
            .filter((item: string) => item)
        : []
      const uniqueAccounts = Array.from(new Set(normalizedAccounts))
      const fallbackAccountID = String((session as any)?.accountId || '').trim()
      if (currentAccountID && uniqueAccounts.length && !uniqueAccounts.includes(currentAccountID)) {
        store.setNotice(tApp('datasource.list.d1ReAuthenticationAccountMismatch'), 'error')
        return
      }
      const nextAccountID = currentAccountID || fallbackAccountID || uniqueAccounts[0] || ''

      const nextOptions = {
        ...(ds.options || {}),
        ...(nextAccountID ? { accountId: nextAccountID } : {}),
        authMode: 'token',
        apiToken: token,
      }
      await api.updateDatasource(ds.id, {
        name: ds.name,
        type: normalizeDatasourceType(ds.type),
        host: ds.host || '',
        port: Number(ds.port || 0),
        username: ds.username || '',
        password: ds.password || '',
        database: ds.database || '',
        authSource: ds.authSource || '',
        options: nextOptions,
      })
      await store.loadDatasources()
      const refreshed = store.datasources.find((item) => item.id === ds.id) || ds
      await testDatasource(refreshed, { silent: true })
      if (statusValue(refreshed.id) === 'connected') {
        store.setNotice(tApp('datasource.list.d1ReAuthenticationSuccess'), 'success')
      } else {
        store.setNotice(statusDetail(refreshed.id) || tApp('status.failed'), 'error')
      }
    } catch (err) {
      store.setNotice(err instanceof Error ? err.message : String(err), 'error')
    } finally {
      const next = { ...d1ReAuthenticationLoading.value }
      delete next[ds.id]
      d1ReAuthenticationLoading.value = next
    }
  }

  const shouldShowDynamoReAuthentication = (ds: DataSource) => {
    if (normalizeDatasourceType(ds.type) !== 'dynamodb') return false
    if (String(ds.options?.authMode || '').trim().toLowerCase() !== 'sso') return false
    return statusValue(ds.id) !== 'connected'
  }

  const reAuthenticateDynamoDatasource = async (ds: DataSource) => {
    if (normalizeDatasourceType(ds.type) !== 'dynamodb') return
    if (isDynamoReAuthenticationLoading(ds.id)) return
    dynamoReAuthenticationLoading.value = { ...dynamoReAuthenticationLoading.value, [ds.id]: true }
    try {
      const profile = String(ds.options?.profile || '').trim()
      const region = String(ds.options?.region || '').trim()
      const configPath = String(ds.options?.ssoConfigPath || '').trim()
      if (!profile) {
        store.setNotice(tApp('datasource.list.dynamoReAuthenticationNeedProfile'), 'error')
        return
      }
      if (!region) {
        store.setNotice(tApp('datasource.list.dynamoReAuthenticationNeedRoleContext'), 'error')
        return
      }

      const authorized = await api.dynamoDBSSOOAuthAuthorize(profile, region, configPath)
      const accountId = String((authorized as any)?.accountId || '').trim()
      const roleName = String((authorized as any)?.roleName || '').trim()
      const accessKeyId = String((authorized as any)?.accessKeyId || '').trim()
      const secretAccessKey = String((authorized as any)?.secretAccessKey || '').trim()
      const sessionToken = String((authorized as any)?.sessionToken || '').trim()
      const expiration = Number((authorized as any)?.expiration || 0)
      if (!accountId || !roleName || !accessKeyId || !secretAccessKey || !sessionToken) {
        store.setNotice(tApp('datasource.list.dynamoReAuthenticationNeedToken'), 'error')
        return
      }

      const nextOptions = {
        ...(ds.options || {}),
        authMode: 'sso',
        profile,
        region,
        ssoAccountId: accountId,
        ssoRoleName: roleName,
        ...(configPath ? { ssoConfigPath: configPath } : {}),
        ...(Number.isFinite(expiration) && expiration > 0 ? { ssoCredentialExpiration: expiration } : {}),
        credentials: {
          accessKeyId,
          secretAccessKey,
          sessionToken,
        },
      }

      await api.updateDatasource(ds.id, {
        name: ds.name,
        type: normalizeDatasourceType(ds.type),
        host: ds.host || '',
        port: Number(ds.port || 0),
        username: ds.username || '',
        password: ds.password || '',
        database: ds.database || '',
        authSource: ds.authSource || '',
        options: nextOptions,
      })

      await store.loadDatasources()
      const refreshed = store.datasources.find((item) => item.id === ds.id) || ds
      await testDatasource(refreshed, { silent: true })
      if (statusValue(refreshed.id) === 'connected') {
        store.setNotice(tApp('datasource.list.dynamoReAuthenticationSuccess'), 'success')
      } else {
        store.setNotice(statusDetail(refreshed.id) || tApp('status.failed'), 'error')
      }
    } catch (err) {
      store.setNotice(err instanceof Error ? err.message : String(err), 'error')
    } finally {
      const next = { ...dynamoReAuthenticationLoading.value }
      delete next[ds.id]
      dynamoReAuthenticationLoading.value = next
    }
  }

  const openDelete = (ds: DataSource) => { deleteTarget.value = ds }
  const closeDelete = () => { if (!deleteBusy.value) deleteTarget.value = null }

  const confirmDelete = async () => {
    if (!deleteTarget.value) return
    deleteBusy.value = true
    try {
      await api.deleteDatasource(deleteTarget.value.id)
      await store.loadDatasources()
      store.setNotice(tApp('datasource.list.deleted'))
    } catch (err) {
      store.setNotice(err instanceof Error ? err.message : String(err), 'error')
    } finally {
      deleteBusy.value = false
      deleteTarget.value = null
    }
  }

  watch(() => store.datasources, () => { void autoProbeDatasources() }, { immediate: true })

  return {
    store,
    filtered,
    datasourceTypeLabel,
    datasourceTypeClass,
    databaseMetaLabel,
    endpointLabel,
    copyEndpoint,
    copyStatusDetail,
    statusLabel,
    statusBadgeClass,
    statusClass,
    statusDetail,
    flashTestId,
    statusCheckedAtLabel,
    shouldShowD1ReAuthentication,
    isD1ReAuthenticationLoading,
    reAuthenticateD1Datasource,
    shouldShowDynamoReAuthentication,
    isDynamoReAuthenticationLoading,
    reAuthenticateDynamoDatasource,
    openCreate,
    editDatasource,
    openConsole,
    testDatasource,
    testAll,
    deleteTarget,
    deleteBusy,
    openDelete,
    closeDelete,
    confirmDelete,
  }
}
