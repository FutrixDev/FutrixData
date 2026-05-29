import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '@/services/api'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import type { DataSource, SecretProviderSummary } from '@/types'
import { applyMongoFormOptions, inferMongoConnMode } from '@/modules/mongo/datasource'
import { dataSourceTypeOptions, normalizeDatasourceType } from '@/modules/datasource/types'
import { parseAwsCredentials } from '@/modules/dynamodb/credentials'
import { tApp } from '@/modules/i18n/appI18n'
import { datasourceLimitNotice, hasReachedDatasourceLimit, resolvePlanLimitMessage } from '@/modules/plan/limits'

type InstallGuide = { run: string; remove: string; connect?: string }

export function useDatasourceFormView() {
  const store = useAppStore()
  const authStore = useAuthStore()
  const route = useRoute()
  const router = useRouter()

  const form = reactive({
    name: '', type: 'mysql', host: '', port: '', username: '', password: '', database: '', authSource: '', options: '',
    sqlMode: 'userpass', sqlUri: '',
    pgSslEnabled: false, pgSslRootCert: '', pgSslCertFileName: '',
    mysqlSslEnabled: false, mysqlSslRootCert: '', mysqlSslCertFileName: '',
    mongoMode: 'userpass', mongoUri: '', mongoReplicaSet: '', mongoHosts: '', mongoTls: false, mongoSslRootCert: '', mongoSslCertFileName: '',
    dynamoAuthMode: 'sso', dynamoRegion: '', dynamoProfile: '', dynamoEndpoint: '',
    dynamoUseStaticCreds: false, dynamoAccessKeyId: '', dynamoSecretAccessKey: '', dynamoSessionToken: '',
    dynamoSSOAccountId: '', dynamoSSORoleName: '', dynamoSSOCredentialExpiration: '', dynamoSSOConfigPath: '',
    d1AccountId: '', d1DatabaseId: '', d1DatabaseName: '', d1Binding: '', d1OauthToken: '',
    d1SupportDev: false, d1DevProjectPath: '',
    chromaScheme: 'http', chromaTenant: 'default_tenant', chromaDatabase: 'default_database', chromaApiToken: '',
    passwordSecretMode: 'manual', passwordSecretProviderId: '', passwordSecretKey: '',
    passwordSecretField: 'password', passwordSecretVersion: '',
  })
  let suppressDynamoSSOProfileSelection = false
  const runWithSuppressedDynamoSSOProfileSelection = (fn: () => void) => {
    suppressDynamoSSOProfileSelection = true
    try {
      fn()
    } finally {
      queueMicrotask(() => {
        suppressDynamoSSOProfileSelection = false
      })
    }
  }

  const errors = ref<string[]>([])
  const fieldErrors = reactive<Record<string, boolean>>({})
  const preservedAIConfigId = ref('')
  const secretProviders = ref<SecretProviderSummary[]>([])
  const secretProvidersAvailable = computed(() => secretProviders.value.length > 0)

  const loadSecretProviders = async () => {
    try {
      const providers = await api.listSecretProviders()
      secretProviders.value = Array.isArray(providers) ? providers : []
    } catch {
      // Secret providers are optional; the form falls back to manual values.
      secretProviders.value = []
    }
  }

  const resolvePreferredSecretProviderId = (preferred: string) => {
    const trimmed = stringsOrEmpty(preferred)
    if (trimmed && secretProviders.value.some((item) => item.id === trimmed)) return trimmed
    const defaultProvider = secretProviders.value.find((item) => item.default)
    if (defaultProvider) return defaultProvider.id
    return secretProviders.value[0]?.id || ''
  }
  const testStatusText = ref('')
  const testStatusDetail = ref('')
  const testStatusClass = ref('')
  const d1CreateStatusText = ref('')
  const d1CreateStatusDetail = ref('')
  const d1CreateStatusClass = ref('')
  const d1Accounts = ref<Array<{ id: string; name: string }>>([])
  const d1Databases = ref<Array<{ id: string; name: string }>>([])
  const d1OAuthLoading = ref(false)
  const d1DatabasesLoading = ref(false)
  const d1CreateDatabaseOpen = ref(false)
  const d1CreateDatabaseLoading = ref(false)
  const d1CreateDatabaseName = ref('')
  const d1OAuthVerified = ref(false)
  // True when editing an existing D1 datasource whose API token is stored server-side
  // — either inline (redacted to "[REDACTED]" in the edit payload) or delegated to a
  // SecretRef (absent from the payload). Both must refresh via the id-based binding.
  const d1HasStoredToken = ref(false)
  const d1WranglerMissing = ref(false)
  const d1WranglerChecked = ref(false)
  const d1WranglerCheckToken = ref(0)
  const d1LegacyMode = ref<'local' | 'cloud' | ''>('')
  const preservedD1Options = ref<Record<string, any>>({})
  // Non-password secret references (e.g. options.uri for SQL/Mongo direct-URL mode)
  // have no dedicated form UI yet. Capture them on load and re-emit them on save so
  // editing such a datasource does not silently drop its only connection secret.
  const preservedSecretRefs = ref<Record<string, any>>({})
  // Datasource type the preserved refs were captured under; changing type abandons
  // them since the connection shape (and which option paths apply) no longer match.
  const preservedSecretRefsType = ref('')
  // Direct-connection fields as loaded from the stored datasource. A preserved
  // options.uri ref is only dropped when the user actually CHANGES these (i.e. takes
  // over the connection directly); pre-existing host/port metadata stored alongside
  // the ref must not, by itself, trigger a drop on an unrelated edit.
  type ConnectionSignature = {
    host: string
    port: string
    uri: string
    hosts: string
    username: string
    password: string
    database: string
    authSource: string
  }
  const emptyConnectionSignature = (): ConnectionSignature => ({
    host: '',
    port: '',
    uri: '',
    hosts: '',
    username: '',
    password: '',
    database: '',
    authSource: '',
  })
  const preservedConnectionSnapshot = ref<ConnectionSignature>(emptyConnectionSignature())
  // The loaded DynamoDB SSO identity (profile + account + role). Temporary SSO
  // credentials stored only as options.credentials.* SecretRefs are scoped to this
  // exact identity, so when the user changes it the preserved credential refs are
  // stale and must be dropped — otherwise the resolved static credentials would keep
  // authenticating with the old role instead of the newly selected one.
  const preservedSSOIdentitySnapshot = ref('')
  // Whether the loaded datasource's password is backed by a SecretRef. Both a
  // ref-backed and an inline-stored password come back redacted as "[REDACTED]";
  // only the ref-backed case may have the sentinel cleared when switching to manual
  // entry, since clearing it for an inline password would overwrite the stored
  // credential with an empty password on the next save.
  const loadedHasPasswordRef = ref(false)
  const d1SupportDevTouched = ref(false)
  const d1DatabaseCreateOptionValue = '__create__'
  const d1DatabaseLoadRequestToken = ref(0)
  const dynamoSSOProfiles = ref<Array<{
    name: string
    region: string
    ssoRegion: string
    startUrl: string
    accountId: string
    roleName: string
  }>>([])
  const dynamoSSOProfilesLoading = ref(false)
  const dynamoSSOConfigApplyLoading = ref(false)
  const dynamoSSOOAuthLoading = ref(false)
  const dynamoSSOVerified = ref(false)
  const dynamoRegionManualOverride = ref(false)
  const dynamoLastAutofilledRegion = ref('')
  const d1OAuthAuthenticated = computed(() => Boolean(form.d1OauthToken.trim()))
  const d1OAuthConnected = computed(() => Boolean(form.d1AccountId.trim() && form.d1OauthToken.trim()))
  // The list/get payloads never expose the real token: an inline token is redacted to
  // "[REDACTED]" and a SecretRef-backed token is absent (empty). When editing a
  // datasource that has a stored token, the form stays "connected" and refreshes
  // through the id-based binding (which resolves the token server-side) instead of
  // calling the token-based API with the marker/empty value.
  const d1EditingDatasourceId = () =>
    store.formMode === 'edit' ? stringsOrEmpty(store.formId) : ''
  const d1StoredTokenActive = () => {
    const token = stringsOrEmpty(form.d1OauthToken)
    return Boolean(d1EditingDatasourceId()) && d1HasStoredToken.value && (token === '' || token === '[REDACTED]')
  }
  // A connection is established when the user typed a token or a stored token (inline
  // or SecretRef) backs the datasource — used to keep the account/database selectors
  // visible even though the SecretRef case leaves form.d1OauthToken empty.
  const d1ConnectionEstablished = computed(() =>
    Boolean(form.d1OauthToken.trim()) || d1StoredTokenActive())
  // Creating a Cloud database is a client-side write that calls Cloudflare with the
  // token in the form. A stored token is empty or the "[REDACTED]" marker (the real
  // value lives server-side), so the create option is only offered once the user has a
  // fresh token in hand; otherwise the call would send the marker or fail as missing.
  const d1CanCreateDatabase = computed(() => {
    const token = form.d1OauthToken.trim()
    return Boolean(token) && token !== '[REDACTED]'
  })
  const isDynamoSSO = computed(() => isDynamo.value && String(form.dynamoAuthMode) === 'sso')
  const dynamoSSOConnected = computed(() => Boolean(
    isDynamoSSO.value
      && stringsOrEmpty(form.dynamoProfile)
      && stringsOrEmpty(form.dynamoSSOAccountId)
      && stringsOrEmpty(form.dynamoSSORoleName)
      && stringsOrEmpty(form.dynamoAccessKeyId)
      && stringsOrEmpty(form.dynamoSecretAccessKey)
      && stringsOrEmpty(form.dynamoSessionToken),
  ))
  const dynamoSSOSelectedProfile = computed(() => {
    const profile = stringsOrEmpty(form.dynamoProfile)
    if (!profile) return null
    return dynamoSSOProfiles.value.find((item) => item.name === profile) || null
  })
  const dynamoSSOConfigEndpoint = computed(() => stringsOrEmpty(dynamoSSOSelectedProfile.value?.startUrl))
  const dynamoSSOHasConfigEndpoint = computed(() => Boolean(isDynamoSSO.value && dynamoSSOConfigEndpoint.value))

  const extractStoredCertificatePath = (raw: unknown) => {
    const value = String(raw ?? '').trim()
    if (!value) return ''
    if (value.includes('\n')) return ''
    if (/-----BEGIN [A-Z ]+-----/.test(value)) return ''
    return value
  }

  const extractCertificateNameFromPath = (path: string) => {
    const normalized = String(path || '').trim().replace(/\\/g, '/')
    if (!normalized) return ''
    const pieces = normalized.split('/')
    return pieces[pieces.length - 1] || ''
  }

  const isMongo = computed(() => form.type === 'mongodb')
  const isRedis = computed(() => normalizeDatasourceType(form.type) === 'redis')
  const isSQL = computed(() => form.type === 'mysql' || form.type === 'postgresql')
  const isDynamo = computed(() => form.type === 'dynamodb')
  const isD1 = computed(() => form.type === 'd1')
  const isChroma = computed(() => form.type === 'chromadb')
  // Mirrors the password input's v-if in DatasourceFormView.vue: the shared
  // password field (SQL userpass, Mongo userpass, Redis, Elasticsearch) is the
  // only place the existing-secret reference toggle applies.
  const passwordFieldApplicable = computed(() => (
    (!isMongo.value || form.mongoMode === 'userpass')
    && (!isSQL.value || form.sqlMode === 'userpass')
    && !isDynamo.value
    && !isD1.value
    && !isChroma.value
  ))
  const showPasswordSecretRef = computed(() => (
    passwordFieldApplicable.value && secretProvidersAvailable.value
  ))
  const usePasswordSecretRef = computed(() => (
    showPasswordSecretRef.value && form.passwordSecretMode === 'existing'
  ))
  const pgSslStoredCertificatePath = computed(() => {
    if (normalizeDatasourceType(form.type) !== 'postgresql') return ''
    return extractStoredCertificatePath(form.pgSslRootCert)
  })
  const pgSslDisplayedCertificateName = computed(() => {
    const uploadedName = String(form.pgSslCertFileName || '').trim()
    if (uploadedName) return uploadedName
    const pathName = extractCertificateNameFromPath(pgSslStoredCertificatePath.value)
    if (pathName) return pathName
    return tApp('datasource.form.sslCertificateDefaultName')
  })
  const mysqlSslStoredCertificatePath = computed(() => {
    if (normalizeDatasourceType(form.type) !== 'mysql') return ''
    return extractStoredCertificatePath(form.mysqlSslRootCert)
  })
  const mysqlSslDisplayedCertificateName = computed(() => {
    const uploadedName = String(form.mysqlSslCertFileName || '').trim()
    if (uploadedName) return uploadedName
    const pathName = extractCertificateNameFromPath(mysqlSslStoredCertificatePath.value)
    if (pathName) return pathName
    return tApp('datasource.form.sslCertificateDefaultName')
  })
  const mongoSslStoredCertificatePath = computed(() => {
    if (normalizeDatasourceType(form.type) !== 'mongodb') return ''
    return extractStoredCertificatePath(form.mongoSslRootCert)
  })
  const mongoSslDisplayedCertificateName = computed(() => {
    const uploadedName = String(form.mongoSslCertFileName || '').trim()
    if (uploadedName) return uploadedName
    const pathName = extractCertificateNameFromPath(mongoSslStoredCertificatePath.value)
    if (pathName) return pathName
    return tApp('datasource.form.sslCertificateDefaultName')
  })
  const showD1DatabaseSelector = computed(() =>
    Boolean(isD1.value && form.d1AccountId.trim() && d1ConnectionEstablished.value))
  const formTitle = computed(() => (store.formMode === 'edit' ? tApp('datasource.form.title.edit') : tApp('datasource.form.title.create')))

  const defaultPortForType = (type: string) => {
    switch (normalizeDatasourceType(type)) {
      case 'mysql': return 3306
      case 'postgresql': return 5432
      case 'mongodb': return 27017
      case 'redis': return 6379
      case 'elasticsearch': return 9200
      case 'dynamodb': return 0
      case 'd1': return 0
      case 'chromadb': return 8000
      default: return 0
    }
  }

  const defaultDatabaseForType = (type: string) => {
    switch (normalizeDatasourceType(type)) {
      case 'mysql': return 'mysql'
      case 'postgresql': return 'postgres'
      default: return ''
    }
  }

  const portPlaceholder = computed(() => String(defaultPortForType(form.type) || 0))
  const databasePlaceholder = computed(() => defaultDatabaseForType(form.type) || tApp('datasource.form.databasePlaceholder'))
  const hint = computed(() => {
    if (form.type === 'mysql') {
      return form.sqlMode === 'uri' ? tApp('datasource.form.hint.mysqlUri') : tApp('datasource.form.hint.mysql')
    }
    if (form.type === 'postgresql') {
      return form.sqlMode === 'uri' ? tApp('datasource.form.hint.postgresqlUri') : tApp('datasource.form.hint.postgresql')
    }
    if (normalizeDatasourceType(form.type) === 'redis') return tApp('datasource.form.hint.redis')
    if (form.type === 'elasticsearch') {
      return tApp('datasource.form.hint.elasticsearch')
    }
    if (form.type === 'dynamodb') {
      return tApp('datasource.form.hint.dynamodb')
    }
    if (form.type === 'd1') {
      return tApp('datasource.form.hint.d1')
    }
    if (form.type === 'chromadb') {
      return tApp('datasource.form.hint.chromadb')
    }
    if (form.type === 'mongodb') {
      return tApp('datasource.form.hint.mongodb')
    }
    return ''
  })

  const installGuide = computed<InstallGuide | null>(() => {
    switch (normalizeDatasourceType(form.type)) {
      case 'mysql':
        return {
          run: 'docker run --name futrix-mysql -e MYSQL_ROOT_PASSWORD=root -p 3306:3306 -d mysql:8',
          remove: 'docker rm -f futrix-mysql',
          connect: 'Host: 127.0.0.1  Port: 3306  Username: root  Password: root',
        }
      case 'mongodb':
        return {
          run: 'docker run --name futrix-mongo -p 27017:27017 -d mongo:7',
          remove: 'docker rm -f futrix-mongo',
          connect: 'Host: 127.0.0.1  Port: 27017  Database: admin',
        }
      case 'redis':
        return {
          run: 'docker run --name futrix-redis -p 6379:6379 -d redis:7',
          remove: 'docker rm -f futrix-redis',
          connect: 'Host: 127.0.0.1  Port: 6379',
        }
      case 'chromadb':
        return {
          run: 'docker run --name futrix-chroma -p 8000:8000 -d chromadb/chroma:latest',
          remove: 'docker rm -f futrix-chroma',
          connect: 'Host: 127.0.0.1  Port: 8000  Scheme: http  Tenant: default_tenant  Database: default_database',
        }
      default:
        return null
    }
  })

  const copyText = async (text: string) => {
    const raw = stringsOrEmpty(text)
    if (!raw) return
    if (typeof navigator === 'undefined' || !navigator.clipboard?.writeText) {
      store.setNotice(tApp('common.clipboardUnavailable'), 'error')
      return
    }
    try {
      await navigator.clipboard.writeText(raw)
      store.setNotice(tApp('common.copied'), 'success')
    } catch (err) {
      store.setNotice(err instanceof Error ? err.message : tApp('common.copyFailed'), 'error')
    }
  }
  const copyInstall = async (text: string) => copyText(text)

  const maskDynamoSensitiveValue = (value: string) => {
    const raw = stringsOrEmpty(value)
    if (raw.length <= 8) return raw
    return `${raw.slice(0, 4)}${'*'.repeat(raw.length - 8)}${raw.slice(-4)}`
  }
  const maskedDynamoSecretAccessKey = computed(() => maskDynamoSensitiveValue(form.dynamoSecretAccessKey))
  const maskedDynamoSessionToken = computed(() => maskDynamoSensitiveValue(form.dynamoSessionToken))
  const copyDynamoSecretAccessKey = async () => copyText(form.dynamoSecretAccessKey)
  const copyDynamoSessionToken = async () => copyText(form.dynamoSessionToken)

  const fieldClass = (field: string) => (fieldErrors[field] ? 'field-error' : '')
  const setFieldError = (field: string) => { fieldErrors[field] = true }
  const clearFieldErrors = () => {
    errors.value = []
    Object.keys(fieldErrors).forEach((key) => { fieldErrors[key] = false })
  }

  const clearTestStatus = () => {
    testStatusText.value = ''
    testStatusDetail.value = ''
    testStatusClass.value = ''
  }

  const clearD1CreateStatus = () => {
    d1CreateStatusText.value = ''
    d1CreateStatusDetail.value = ''
    d1CreateStatusClass.value = ''
  }

  const detectD1WranglerAvailability = async () => {
    const requestToken = d1WranglerCheckToken.value + 1
    d1WranglerCheckToken.value = requestToken
    try {
      const installed = await api.d1IsWranglerInstalled()
      if (requestToken !== d1WranglerCheckToken.value || !isD1.value) return
      d1WranglerMissing.value = !Boolean(installed)
      d1WranglerChecked.value = true
    } catch {
      if (requestToken !== d1WranglerCheckToken.value || !isD1.value) return
      d1WranglerMissing.value = false
      d1WranglerChecked.value = false
    }
  }

  const readFileText = (file: File) => new Promise<string>((resolve, reject) => {
    if (typeof (file as any)?.text === 'function') {
      ;(file as any).text().then(resolve).catch(reject)
      return
    }
    if (typeof (file as any)?.arrayBuffer === 'function') {
      ;(file as any).arrayBuffer()
        .then((buf: ArrayBuffer) => resolve(new TextDecoder('utf-8').decode(buf)))
        .catch(reject)
      return
    }
    if (typeof FileReader !== 'undefined') {
      const reader = new FileReader()
      reader.onload = () => resolve(String(reader.result ?? ''))
      reader.onerror = () => reject(reader.error ?? new Error(tApp('kb.file.readFailed')))
      reader.readAsText(file)
      return
    }
    reject(new Error(tApp('datasource.form.fileReaderUnsupported')))
  })

  const importDynamoCredentialsFromFile = async (file: File) => {
    if (!isDynamo.value) return
    try {
      const raw = await readFileText(file)
      const parsed = parseAwsCredentials(raw, form.dynamoProfile.trim() || undefined)
      form.dynamoUseStaticCreds = true
      form.dynamoAccessKeyId = parsed.credentials.accessKeyId
      form.dynamoSecretAccessKey = parsed.credentials.secretAccessKey
      form.dynamoSessionToken = parsed.credentials.sessionToken || ''
      store.setNotice(
        parsed.profile
          ? tApp('datasource.form.awsCredentialsImportedWithProfile', { profile: parsed.profile })
          : tApp('datasource.form.awsCredentialsImported'),
        'success',
      )
    } catch (err) {
      store.setNotice(err instanceof Error ? err.message : String(err), 'error')
    }
  }

  const clearDynamoSSOAuthorization = () => {
    form.dynamoSSOAccountId = ''
    form.dynamoSSORoleName = ''
    form.dynamoSSOCredentialExpiration = ''
    form.dynamoUseStaticCreds = false
    form.dynamoAccessKeyId = ''
    form.dynamoSecretAccessKey = ''
    form.dynamoSessionToken = ''
    dynamoSSOVerified.value = false
  }

  const resetDynamoRegionPreference = () => {
    dynamoRegionManualOverride.value = false
    dynamoLastAutofilledRegion.value = ''
  }

  const markDynamoRegionAsManual = () => {
    const region = stringsOrEmpty(form.dynamoRegion)
    if (!region) {
      resetDynamoRegionPreference()
      return
    }
    const autoRegion = stringsOrEmpty(dynamoLastAutofilledRegion.value)
    if (autoRegion && region === autoRegion) return
    dynamoRegionManualOverride.value = true
  }

  const clearDynamoSSOSession = () => {
    clearDynamoSSOAuthorization()
  }

  const normalizeDynamoSSOProfiles = (input: unknown) => {
    const rows = Array.isArray(input) ? input : []
    const normalized = rows
      .map((item: any) => ({
        name: stringsOrEmpty(item?.name),
        region: stringsOrEmpty(item?.region),
        ssoRegion: stringsOrEmpty(item?.ssoRegion),
        startUrl: stringsOrEmpty(item?.startUrl),
        accountId: stringsOrEmpty(item?.accountId),
        roleName: stringsOrEmpty(item?.roleName),
      }))
      .filter((item) => item.name)
    const seen = new Set<string>()
    return normalized.filter((item) => {
      if (seen.has(item.name)) return false
      seen.add(item.name)
      return true
    })
  }

  const resolvePreferredDynamoProfile = (
    profiles: Array<{ name: string }>,
    preferredProfile: string,
  ) => {
    const preferred = stringsOrEmpty(preferredProfile)
    if (preferred && profiles.some((item) => item.name === preferred)) {
      return preferred
    }
    if (profiles.some((item) => item.name === 'default')) {
      return 'default'
    }
    if (profiles.length === 1) {
      return profiles[0].name
    }
    return profiles[0]?.name || ''
  }

  const applyDynamoProfileDefaults = (profileName: string) => {
    const profile = dynamoSSOProfiles.value.find((item) => item.name === stringsOrEmpty(profileName))
    if (!profile) return
    const nextRegion = stringsOrEmpty(profile.region) || stringsOrEmpty(profile.ssoRegion)
    const currentRegion = stringsOrEmpty(form.dynamoRegion)
    const previousAutoRegion = stringsOrEmpty(dynamoLastAutofilledRegion.value)
    const allowAutofillRegion = Boolean(
      !currentRegion
      || !dynamoRegionManualOverride.value
      || (previousAutoRegion && currentRegion === previousAutoRegion),
    )
    if (nextRegion && allowAutofillRegion) {
      form.dynamoRegion = nextRegion
      dynamoLastAutofilledRegion.value = nextRegion
      dynamoRegionManualOverride.value = false
    }
    if (profile.accountId) {
      form.dynamoSSOAccountId = profile.accountId
    }
    if (profile.roleName) {
      form.dynamoSSORoleName = profile.roleName
    }
  }

  const loadDynamoSSOProfiles = async () => {
    if (!isDynamoSSO.value) return
    dynamoSSOProfilesLoading.value = true
    let loaded = false
    try {
      const profiles = await api.dynamoDBSSOListProfiles(stringsOrEmpty(form.dynamoSSOConfigPath))
      dynamoSSOProfiles.value = normalizeDynamoSSOProfiles(profiles)
      if (!dynamoSSOProfiles.value.length) {
        form.dynamoProfile = ''
        return false
      }
      form.dynamoProfile = resolvePreferredDynamoProfile(dynamoSSOProfiles.value, form.dynamoProfile)
      applyDynamoProfileDefaults(form.dynamoProfile)
      loaded = true
    } catch (err) {
      store.setNotice(err instanceof Error ? err.message : String(err), 'error')
      dynamoSSOProfiles.value = []
    } finally {
      dynamoSSOProfilesLoading.value = false
    }
    return loaded
  }

  const applyDynamoSSOConfigPath = async () => {
    if (!isDynamoSSO.value || dynamoSSOProfilesLoading.value || dynamoSSOConfigApplyLoading.value) return
    dynamoSSOConfigApplyLoading.value = true
    try {
      const loaded = await loadDynamoSSOProfiles()
      if (loaded) {
        store.setNotice(tApp('datasource.form.dynamo.ssoConfigPathApplied'), 'success')
      }
    } finally {
      dynamoSSOConfigApplyLoading.value = false
    }
  }

  const handleDynamoSSOProfileSelection = () => {
    if (!isDynamoSSO.value) return
    clearDynamoSSOAuthorization()
    applyDynamoProfileDefaults(form.dynamoProfile)
  }

  const dynamoSSOOAuthAuthorize = async () => {
    if (!isDynamoSSO.value || dynamoSSOOAuthLoading.value) return
    const profile = stringsOrEmpty(form.dynamoProfile)
    const region = stringsOrEmpty(form.dynamoRegion)
    const configPath = stringsOrEmpty(form.dynamoSSOConfigPath)
    if (!profile) {
      store.setNotice(tApp('validation.dynamoProfileRequired'), 'error')
      return
    }
    if (!region) {
      store.setNotice(tApp('validation.regionRequired'), 'error')
      return
    }

    dynamoSSOOAuthLoading.value = true
    clearDynamoSSOAuthorization()
    try {
      const authorized = await api.dynamoDBSSOOAuthAuthorize(profile, region, configPath)
      const nextProfile = stringsOrEmpty((authorized as any)?.profile) || profile
      const nextRegion = stringsOrEmpty((authorized as any)?.region) || region
      const accountId = stringsOrEmpty((authorized as any)?.accountId)
      const roleName = stringsOrEmpty((authorized as any)?.roleName)
      const accessKeyId = stringsOrEmpty((authorized as any)?.accessKeyId)
      const secretAccessKey = stringsOrEmpty((authorized as any)?.secretAccessKey)
      const sessionToken = stringsOrEmpty((authorized as any)?.sessionToken)
      const expirationRaw = Number((authorized as any)?.expiration || 0)
      if (!accountId || !roleName || !accessKeyId || !secretAccessKey || !sessionToken) {
        throw new Error(tApp('datasource.form.dynamo.ssoAuthorizeInvalidResponse'))
      }

      form.dynamoProfile = nextProfile
      form.dynamoRegion = nextRegion
      form.dynamoSSOAccountId = accountId
      form.dynamoSSORoleName = roleName
      form.dynamoUseStaticCreds = true
      form.dynamoAccessKeyId = accessKeyId
      form.dynamoSecretAccessKey = secretAccessKey
      form.dynamoSessionToken = sessionToken
      form.dynamoSSOCredentialExpiration = Number.isFinite(expirationRaw) && expirationRaw > 0 ? String(expirationRaw) : ''
      dynamoSSOVerified.value = true
      store.setNotice(tApp('datasource.form.dynamo.ssoAuthorizeSuccess'), 'success')
    } catch (err) {
      clearDynamoSSOAuthorization()
      store.setNotice(err instanceof Error ? err.message : String(err), 'error')
    } finally {
      dynamoSSOOAuthLoading.value = false
    }
  }

  const normalizeCertificateText = (raw: string) => raw.replace(/\r\n/g, '\n').trim()
  const isPEMCertificate = (value: string) => {
    const upper = value.toUpperCase()
    return upper.includes('-----BEGIN CERTIFICATE-----') && upper.includes('-----END CERTIFICATE-----')
  }

  const importPostgresCertificateFromFile = async (file: File) => {
    if (normalizeDatasourceType(form.type) !== 'postgresql') return
    try {
      const raw = await readFileText(file)
      const normalized = normalizeCertificateText(raw)
      if (!normalized) throw new Error(tApp('datasource.form.sslCertificateEmpty'))
      if (!isPEMCertificate(normalized)) throw new Error(tApp('datasource.form.sslCertificateInvalidPem'))
      form.pgSslEnabled = true
      form.pgSslRootCert = normalized
      form.pgSslCertFileName = String(file.name || '').trim()
      const certificateName = form.pgSslCertFileName || tApp('datasource.form.sslCertificateDefaultName')
      store.setNotice(tApp('datasource.form.postgresSslCertificateImported', { name: certificateName }), 'success')
    } catch (err) {
      store.setNotice(err instanceof Error ? err.message : String(err), 'error')
    }
  }

  const importMySQLCertificateFromFile = async (file: File) => {
    if (normalizeDatasourceType(form.type) !== 'mysql') return
    try {
      const raw = await readFileText(file)
      const normalized = normalizeCertificateText(raw)
      if (!normalized) throw new Error(tApp('datasource.form.sslCertificateEmpty'))
      if (!isPEMCertificate(normalized)) throw new Error(tApp('datasource.form.sslCertificateInvalidPem'))
      form.mysqlSslEnabled = true
      form.mysqlSslRootCert = normalized
      form.mysqlSslCertFileName = String(file.name || '').trim()
      const certificateName = form.mysqlSslCertFileName || tApp('datasource.form.sslCertificateDefaultName')
      store.setNotice(tApp('datasource.form.mysqlSslCertificateImported', { name: certificateName }), 'success')
    } catch (err) {
      store.setNotice(err instanceof Error ? err.message : String(err), 'error')
    }
  }

  const importMongoCertificateFromFile = async (file: File) => {
    if (normalizeDatasourceType(form.type) !== 'mongodb') return
    try {
      const raw = await readFileText(file)
      const normalized = normalizeCertificateText(raw)
      if (!normalized) throw new Error(tApp('datasource.form.sslCertificateEmpty'))
      if (!isPEMCertificate(normalized)) throw new Error(tApp('datasource.form.sslCertificateInvalidPem'))
      form.mongoTls = true
      form.mongoSslRootCert = normalized
      form.mongoSslCertFileName = String(file.name || '').trim()
      const certificateName = form.mongoSslCertFileName || tApp('datasource.form.sslCertificateDefaultName')
      store.setNotice(tApp('datasource.form.mongoSslCertificateImported', { name: certificateName }), 'success')
    } catch (err) {
      store.setNotice(err instanceof Error ? err.message : String(err), 'error')
    }
  }

  const showPostgresCertificatePath = () => {
    const path = pgSslStoredCertificatePath.value
    if (!path) return
    store.setNotice(tApp('datasource.form.postgresSslCertificatePathNotice', { path }), 'info')
  }

  const showMySQLCertificatePath = () => {
    const path = mysqlSslStoredCertificatePath.value
    if (!path) return
    store.setNotice(tApp('datasource.form.mysqlSslCertificatePathNotice', { path }), 'info')
  }

  const showMongoCertificatePath = () => {
    const path = mongoSslStoredCertificatePath.value
    if (!path) return
    store.setNotice(tApp('datasource.form.mongoSslCertificatePathNotice', { path }), 'info')
  }

  const syncFormStateFromRoute = () => {
    const id = typeof route.params.id === 'string' ? route.params.id : ''
    if (route.name === 'datasource-edit' && id) {
      store.formMode = 'edit'
      store.formId = id
      return
    }
    store.formMode = 'create'
    store.formId = null
  }

  const ensureDatasourcesLoaded = async () => {
    if (!store.datasources.length) await store.loadDatasources()
  }

  const stringsOrEmpty = (value: unknown) => String(value ?? '').trim()
  // Normalize a port for connection-change detection: an unset port renders as 0 in
  // the payload but as "" in the stored record, so collapse both to "".
  const normalizePortValue = (value: unknown) => {
    const s = stringsOrEmpty(value)
    return s === '0' ? '' : s
  }
  // Direct-connection fingerprint of a built payload. Computed from the payload (not
  // the raw stored record) so the loaded baseline accounts for form defaults like the
  // type's default port; comparing two of these tells whether the user changed the
  // connection or merely edited an unrelated field.
  const connectionSignatureOf = (p: any) => ({
    host: stringsOrEmpty(p?.host),
    port: normalizePortValue(p?.port),
    uri: stringsOrEmpty(p?.options?.uri),
    hosts: Array.isArray(p?.options?.hosts) ? p.options.hosts.join(',') : '',
    // Credential/identity fields also define the connection: the SQL/Mongo adapters
    // prefer a resolved options.uri over these, so editing any of them means the user
    // is taking over the connection and the stale external URI ref must be dropped.
    username: stringsOrEmpty(p?.username),
    password: stringsOrEmpty(p?.password),
    database: stringsOrEmpty(p?.database),
    authSource: stringsOrEmpty(p?.authSource),
  })
  // Fingerprint of the DynamoDB SSO identity a built payload authenticates as. When
  // this differs from the loaded baseline the temporary SSO credentials (held only as
  // options.credentials.* refs) belong to the previous role and must not be re-emitted.
  const ssoIdentitySignatureOf = (p: any) =>
    [
      stringsOrEmpty(p?.options?.profile),
      stringsOrEmpty(p?.options?.ssoAccountId),
      stringsOrEmpty(p?.options?.ssoRoleName),
    ].join('\u0000')
  const optionStringValue = (options: Record<string, any> | undefined, key: string) => {
    const value = options?.[key]
    return typeof value === 'string' ? value.trim() : ''
  }
  // Whether the built payload carries a concrete, user-supplied value at an
  // `options.*` secret-ref path (e.g. options.apiToken, options.credentials.x). A
  // ref-backed path stores no plaintext, so any non-empty value other than the
  // redaction sentinel means the user typed a replacement and the stale external
  // ref must be dropped — otherwise ResolveDatasource would overwrite the edit.
  const payloadOptionPathHasUserValue = (payload: any, fieldPath: string) => {
    if (!fieldPath.startsWith('options.')) return false
    const segments = fieldPath.slice('options.'.length).split('.')
    let node: any = payload?.options
    for (const segment of segments) {
      if (node == null || typeof node !== 'object') return false
      node = node[segment]
    }
    if (typeof node !== 'string') return node != null && node !== ''
    const trimmed = node.trim()
    return trimmed !== '' && trimmed !== '[REDACTED]'
  }
  const parseOptions = () => (form.options.trim() ? JSON.parse(form.options) : {})
  const inferSQLConnMode = (ds: DataSource) => {
    const normalizedType = normalizeDatasourceType(ds.type || '')
    if (normalizedType !== 'mysql' && normalizedType !== 'postgresql') return 'userpass'
    return optionStringValue(ds.options as Record<string, any> | undefined, 'uri') ? 'uri' : 'userpass'
  }
  const boolFromAny = (value: unknown) => {
    if (typeof value === 'boolean') return value
    const normalized = String(value ?? '').trim().toLowerCase()
    return normalized === '1' || normalized === 'true' || normalized === 'yes' || normalized === 'on'
  }
  const inferPostgresSSLEnabled = (options: Record<string, any> | undefined) => {
    if (!options || typeof options !== 'object') return false
    if (Object.prototype.hasOwnProperty.call(options, 'sslEnabled')) {
      return boolFromAny((options as any).sslEnabled)
    }
    const sslMode = optionStringValue(options, 'sslmode').toLowerCase()
    if (sslMode) {
      return sslMode !== 'disable' && sslMode !== 'disabled' && sslMode !== 'off' && sslMode !== 'false' && sslMode !== '0'
    }
    return optionStringValue(options, 'sslrootcert') !== ''
  }
  const inferMySQLSSLEnabled = (options: Record<string, any> | undefined) => {
    if (!options || typeof options !== 'object') return false
    if (Object.prototype.hasOwnProperty.call(options, 'sslEnabled')) {
      return boolFromAny((options as any).sslEnabled)
    }
    const tlsValue = optionStringValue(options, 'tls').toLowerCase()
    if (tlsValue) {
      return tlsValue !== 'disable' && tlsValue !== 'disabled' && tlsValue !== 'off' && tlsValue !== 'false' && tlsValue !== '0'
    }
    return optionStringValue(options, 'sslrootcert') !== ''
  }
  const inferMongoSSLEnabled = (options: Record<string, any> | undefined) => {
    if (!options || typeof options !== 'object') return false
    if (Object.prototype.hasOwnProperty.call(options, 'sslEnabled')) {
      return boolFromAny((options as any).sslEnabled)
    }
    if (Object.prototype.hasOwnProperty.call(options, 'tls')) {
      return boolFromAny((options as any).tls)
    }
    return optionStringValue(options, 'sslrootcert') !== ''
  }
  const hasLegacyD1DevMetadata = (options: Record<string, any> | null | undefined) => {
    if (!options || typeof options !== 'object') return false
    const wranglerConfigPath = stringsOrEmpty((options as any).wranglerConfigPath)
    if (!wranglerConfigPath) return false
    if (boolFromAny((options as any).supportDev)) return false
    if (stringsOrEmpty((options as any).devProjectPath)) return false
    return true
  }
  const normalizeD1Accounts = (accounts: unknown, fallbackAccountId = '') => {
    const rows = Array.isArray(accounts) ? accounts : []
    const seen = new Set<string>()
    const normalized = rows
      .map((item: any) => ({
        id: stringsOrEmpty(item?.id || item?.accountId || item?.accountTag),
        name: stringsOrEmpty(item?.name),
      }))
      .filter((item) => item.id)
      .map((item) => {
        if (seen.has(item.id)) return null
        seen.add(item.id)
        return { id: item.id, name: item.name || item.id }
      })
      .filter((item): item is { id: string; name: string } => Boolean(item))
    const fallback = stringsOrEmpty(fallbackAccountId)
    if (fallback && !seen.has(fallback)) {
      normalized.push({ id: fallback, name: fallback })
    }
    return normalized
  }

  const resolvePreferredD1AccountId = (
    accounts: Array<{ id: string; name: string }>,
    preferredAccountId: string,
    fallbackAccountId: string,
  ) => {
    const preferred = stringsOrEmpty(preferredAccountId)
    if (preferred && accounts.some((item) => item.id === preferred)) {
      return preferred
    }
    if (accounts.length === 1) {
      return accounts[0].id
    }
    const fallback = stringsOrEmpty(fallbackAccountId)
    if (preferred && fallback && accounts.some((item) => item.id === fallback)) {
      return fallback
    }
    if (!accounts.length) {
      return preferred || fallback
    }
    return ''
  }

  const syncD1DatabaseName = () => {
    const selectedId = stringsOrEmpty(form.d1DatabaseId)
    if (!selectedId || selectedId === d1DatabaseCreateOptionValue) return
    const match = d1Databases.value.find((item) => item.id === selectedId)
    if (!match) return
    form.d1DatabaseName = match.name
  }

  const invalidateD1DatabaseLoadRequests = () => {
    d1DatabaseLoadRequestToken.value += 1
  }

  const loadD1CloudDatabases = async () => {
    if (!isD1.value) return false
    const requestToken = d1DatabaseLoadRequestToken.value + 1
    d1DatabaseLoadRequestToken.value = requestToken
    const accountId = stringsOrEmpty(form.d1AccountId)
    const token = stringsOrEmpty(form.d1OauthToken)
    const editingId = d1EditingDatasourceId()
    const useStoredToken = d1StoredTokenActive()
    if (!token && !useStoredToken) {
      d1OAuthVerified.value = false
    }
    if (!accountId || (!token && !useStoredToken)) {
      d1Databases.value = []
      d1DatabasesLoading.value = false
      return false
    }
    d1DatabasesLoading.value = true
    try {
      const list = useStoredToken
        ? await api.d1ListCloudDatabasesForDatasource(editingId, accountId)
        : await api.d1ListCloudDatabases(accountId, token)
      if (requestToken !== d1DatabaseLoadRequestToken.value) return false
      d1Databases.value = Array.isArray(list)
        ? list
            .map((item: any) => ({ id: stringsOrEmpty(item?.id), name: stringsOrEmpty(item?.name) }))
            .filter((item) => item.id && item.name)
        : []
      d1OAuthVerified.value = true
      const selectedDatabaseID = stringsOrEmpty(form.d1DatabaseId)
      if (!d1Databases.value.some((item) => item.id === selectedDatabaseID)) {
        if (selectedDatabaseID) {
          const selectedDatabaseName = stringsOrEmpty(form.d1DatabaseName) || selectedDatabaseID
          d1Databases.value = [{ id: selectedDatabaseID, name: selectedDatabaseName }, ...d1Databases.value]
        } else {
          form.d1DatabaseId = d1Databases.value[0]?.id || ''
        }
      }
      syncD1DatabaseName()
      return true
    } catch (err) {
      if (requestToken !== d1DatabaseLoadRequestToken.value) return false
      d1Databases.value = []
      d1OAuthVerified.value = false
      store.setNotice(err instanceof Error ? err.message : String(err), 'error')
      return false
    } finally {
      if (requestToken === d1DatabaseLoadRequestToken.value) {
        d1DatabasesLoading.value = false
      }
    }
  }

  const d1OAuthLogin = async () => {
    if (!isD1.value || d1OAuthLoading.value) return
    if (!d1WranglerChecked.value) {
      await detectD1WranglerAvailability()
    }
    if (d1WranglerMissing.value) {
      store.setNotice(tApp('datasource.form.d1.wranglerInstallHint'), 'error')
      return
    }
    d1OAuthLoading.value = true
    const previousVerified = d1OAuthVerified.value
    const previousAccountId = stringsOrEmpty(form.d1AccountId)
    const previousDatabaseId = stringsOrEmpty(form.d1DatabaseId)
    const previousDatabaseName = stringsOrEmpty(form.d1DatabaseName)
    const shouldForceReLogin = Boolean(d1OAuthVerified.value && d1OAuthAuthenticated.value)
    try {
      const session = shouldForceReLogin ? await api.d1OAuthReLogin() : await api.d1OAuthLogin()
      const nextToken = stringsOrEmpty((session as any)?.token)
      if (!nextToken) {
        throw new Error(tApp('datasource.form.d1.oauthInvalidResponse'))
      }
      form.d1OauthToken = nextToken
      d1OAuthVerified.value = true
      d1Accounts.value = normalizeD1Accounts((session as any)?.accounts, stringsOrEmpty((session as any)?.accountId))
      const nextAccountId = resolvePreferredD1AccountId(
        d1Accounts.value,
        previousAccountId,
        stringsOrEmpty((session as any)?.accountId),
      )
      const accountChanged = nextAccountId !== previousAccountId
      form.d1AccountId = nextAccountId
      if (accountChanged) {
        form.d1DatabaseId = ''
        form.d1DatabaseName = ''
        d1Databases.value = []
      } else if (previousDatabaseId && !d1Databases.value.some((item) => item.id === previousDatabaseId)) {
        d1Databases.value = [{ id: previousDatabaseId, name: previousDatabaseName || previousDatabaseId }, ...d1Databases.value]
        form.d1DatabaseId = previousDatabaseId
        form.d1DatabaseName = previousDatabaseName || previousDatabaseId
      }
      if (nextAccountId) {
        const refreshed = await loadD1CloudDatabases()
        if (!refreshed) {
          return
        }
      }
      d1CreateDatabaseOpen.value = false
      d1CreateDatabaseName.value = ''
      clearD1CreateStatus()
      store.setNotice(tApp('datasource.form.d1.oauthSuccess'), 'success')
    } catch (err) {
      d1OAuthVerified.value = Boolean(previousVerified && stringsOrEmpty(form.d1OauthToken))
      store.setNotice(err instanceof Error ? err.message : String(err), 'error')
    } finally {
      d1OAuthLoading.value = false
    }
  }

  const createD1Database = async () => {
    if (d1CreateDatabaseLoading.value) return
    clearD1CreateStatus()
    const accountId = stringsOrEmpty(form.d1AccountId)
    const token = stringsOrEmpty(form.d1OauthToken)
    if (!accountId || !token) {
      d1CreateStatusText.value = tApp('status.failed')
      d1CreateStatusDetail.value = tApp('validation.d1OauthRequired')
      d1CreateStatusClass.value = 'failed'
      return
    }
    const trimmedName = stringsOrEmpty(d1CreateDatabaseName.value)
    if (!trimmedName) {
      d1CreateStatusText.value = tApp('status.failed')
      d1CreateStatusDetail.value = tApp('validation.d1CreateDatabaseNameRequired')
      d1CreateStatusClass.value = 'failed'
      return
    }
    d1CreateDatabaseLoading.value = true
    try {
      const created = await api.d1CreateCloudDatabase(accountId, token, trimmedName)
      const normalized = {
        id: stringsOrEmpty((created as any)?.id),
        name: stringsOrEmpty((created as any)?.name),
      }
      if (!normalized.id || !normalized.name) {
        throw new Error(tApp('datasource.form.d1.createInvalidResponse'))
      }
      if (!d1Databases.value.some((item) => item.id === normalized.id)) {
        d1Databases.value = [...d1Databases.value, normalized]
      }
      form.d1DatabaseId = normalized.id
      form.d1DatabaseName = normalized.name
      d1CreateDatabaseOpen.value = false
      d1CreateDatabaseName.value = ''
      d1CreateStatusText.value = tApp('status.success')
      d1CreateStatusDetail.value = tApp('datasource.form.d1.createSuccess', { name: normalized.name })
      d1CreateStatusClass.value = 'connected'
    } catch (err) {
      d1CreateStatusText.value = tApp('status.failed')
      d1CreateStatusDetail.value = err instanceof Error ? err.message : String(err)
      d1CreateStatusClass.value = 'failed'
      form.d1DatabaseId = ''
    } finally {
      d1CreateDatabaseLoading.value = false
    }
  }

  const cancelCreateD1Database = () => {
    d1CreateDatabaseOpen.value = false
    d1CreateDatabaseName.value = ''
    clearD1CreateStatus()
    if (!form.d1DatabaseId) {
      form.d1DatabaseName = ''
    }
  }

  const handleD1AccountSelection = () => {
    form.d1DatabaseId = ''
    form.d1DatabaseName = ''
    d1CreateDatabaseOpen.value = false
    d1CreateDatabaseName.value = ''
    d1Databases.value = []
    d1DatabasesLoading.value = false
    invalidateD1DatabaseLoadRequests()
    if (!stringsOrEmpty(form.d1AccountId)) {
      return
    }
  }

  const handleD1DatabaseSelection = async () => {
    if (stringsOrEmpty(form.d1DatabaseId) !== d1DatabaseCreateOptionValue) {
      d1CreateDatabaseOpen.value = false
      d1CreateDatabaseName.value = ''
      syncD1DatabaseName()
      return
    }
    form.d1DatabaseId = ''
    form.d1DatabaseName = ''
    d1CreateDatabaseOpen.value = true
  }

  const buildPayload = () => {
    const options = isMongo.value
      ? applyMongoFormOptions(undefined, {
          mode: form.mongoMode,
          uri: form.mongoUri,
          tls: form.mongoTls,
          sslEnabled: form.mongoTls,
          sslrootcert: form.mongoSslRootCert,
          replicaSet: form.mongoReplicaSet,
          hosts: form.mongoHosts,
        })
      : isSQL.value
        ? (() => {
            const applySQLSSLOptions = (next: Record<string, any>) => {
              const normalizedType = normalizeDatasourceType(form.type)
              if (normalizedType === 'postgresql') {
                next.sslEnabled = Boolean(form.pgSslEnabled)
                const certificate = form.pgSslRootCert.trim()
                if (form.pgSslEnabled && certificate) next.sslrootcert = certificate
                else delete next.sslrootcert
              }
              if (normalizedType === 'mysql') {
                next.sslEnabled = Boolean(form.mysqlSslEnabled)
                const certificate = form.mysqlSslRootCert.trim()
                if (form.mysqlSslEnabled && certificate) next.sslrootcert = certificate
                else delete next.sslrootcert
              }
              return next
            }
            if (form.sqlMode === 'uri') {
              const next: Record<string, any> = {}
              const uri = form.sqlUri.trim()
              if (uri) next.uri = uri
              return applySQLSSLOptions(next)
            }
            const next = parseOptions()
            if (next && typeof next === 'object') {
              delete next.uri
            }
            return applySQLSSLOptions(next)
          })()
        : isDynamo.value
          ? (() => {
              const next: Record<string, any> = {}
              const authMode = isDynamoSSO.value ? 'sso' : 'profile'
              const region = stringsOrEmpty(form.dynamoRegion)
              const profile = stringsOrEmpty(form.dynamoProfile)
              const endpoint = stringsOrEmpty(form.dynamoEndpoint)
              const accountId = stringsOrEmpty(form.dynamoSSOAccountId)
              const roleName = stringsOrEmpty(form.dynamoSSORoleName)
              const configPath = stringsOrEmpty(form.dynamoSSOConfigPath)
              next.authMode = authMode
              if (region) next.region = region
              if (profile) next.profile = profile
              if (endpoint) next.endpoint = endpoint
              if (authMode === 'sso') {
                if (accountId) next.ssoAccountId = accountId
                if (roleName) next.ssoRoleName = roleName
                if (configPath) next.ssoConfigPath = configPath
                const expiration = Number(form.dynamoSSOCredentialExpiration || 0)
                if (Number.isFinite(expiration) && expiration > 0) {
                  next.ssoCredentialExpiration = expiration
                }
              } else {
                delete next.ssoAccountId
                delete next.ssoRoleName
                delete next.ssoConfigPath
                delete next.ssoCredentialExpiration
              }
              if (form.dynamoUseStaticCreds) {
                const accessKeyId = stringsOrEmpty(form.dynamoAccessKeyId)
                const secretAccessKey = stringsOrEmpty(form.dynamoSecretAccessKey)
                const sessionToken = stringsOrEmpty(form.dynamoSessionToken)
                if (accessKeyId || secretAccessKey || sessionToken) {
                  next.credentials = {
                    accessKeyId,
                    secretAccessKey,
                    ...(sessionToken ? { sessionToken } : {}),
                  }
                }
              }
              return next
            })()
          : isD1.value
            ? (() => {
                const next: Record<string, any> = store.formMode === 'edit'
                  ? { ...preservedD1Options.value }
                  : {}
                if (d1LegacyMode.value === 'local' || d1LegacyMode.value === 'cloud') {
                  next.mode = d1LegacyMode.value
                } else {
                  delete next.mode
                }
                if (form.d1AccountId.trim()) next.accountId = form.d1AccountId.trim()
                else delete next.accountId
                if (form.d1DatabaseId.trim()) next.databaseId = form.d1DatabaseId.trim()
                else delete next.databaseId
                if (form.d1DatabaseName.trim()) next.databaseName = form.d1DatabaseName.trim()
                else delete next.databaseName
                if (form.d1Binding.trim()) next.binding = form.d1Binding.trim()
                else delete next.binding
                // A SecretRef-backed token leaves form.d1OauthToken empty; the ref is
                // re-emitted below under options.apiToken. Keep token auth mode (and
                // drop any inline value) so D1Adapter uses the resolved token instead of
                // silently falling back to wrangler auth.
                const tokenRefPreserved =
                  normalizeDatasourceType(form.type) === preservedSecretRefsType.value &&
                  Boolean(preservedSecretRefs.value['options.apiToken'])
                if (form.d1OauthToken.trim()) {
                  next.authMode = 'token'
                  next.apiToken = form.d1OauthToken.trim()
                } else if (tokenRefPreserved) {
                  next.authMode = 'token'
                  delete next.apiToken
                } else {
                  delete next.apiToken
                  if (String(next.authMode || '').trim().toLowerCase() === 'token') delete next.authMode
                }
                const preserveLegacyDevMetadata = store.formMode === 'edit'
                  && hasLegacyD1DevMetadata(next)
                  && !d1SupportDevTouched.value
                const devProjectPath = form.d1DevProjectPath.trim()
                const supportDev = Boolean(form.d1SupportDev && devProjectPath)
                if (supportDev) {
                  next.supportDev = true
                  next.devProjectPath = devProjectPath
                } else {
                  delete next.devProjectPath
                  if (preserveLegacyDevMetadata) {
                    delete next.supportDev
                  } else {
                    next.supportDev = false
                    delete next.wranglerConfigPath
                    delete next.migrationsDir
                  }
                }
                return next
              })()
            : isChroma.value
              ? (() => {
                  const next: Record<string, any> = {
                    scheme: stringsOrEmpty(form.chromaScheme).toLowerCase() || 'http',
                    tenant: stringsOrEmpty(form.chromaTenant) || 'default_tenant',
                    database: stringsOrEmpty(form.chromaDatabase) || 'default_database',
                  }
                  const apiToken = stringsOrEmpty(form.chromaApiToken)
                  if (apiToken) next.apiToken = apiToken
                  return next
                })()
              : parseOptions()

    if (store.formMode === 'edit' && preservedAIConfigId.value) options.aiConfigId = preservedAIConfigId.value
    else delete options.aiConfigId

    const payload = {
      name: form.name.trim(),
      type: normalizeDatasourceType(form.type),
      host: form.host.trim(),
      port: form.port ? Number(form.port) : 0,
      username: form.username.trim(),
      password: form.password,
      database: form.database.trim(),
      authSource: form.authSource.trim(),
      options,
    }
    if (isMongo.value && form.mongoMode === 'uri') { payload.host = ''; payload.port = 0 }
    if (isSQL.value && form.sqlMode === 'uri') {
      payload.host = ''
      payload.port = 0
      payload.username = ''
      payload.password = ''
      payload.database = ''
    }
    if (isDynamo.value) {
      payload.host = ''
      payload.port = 0
      payload.username = ''
      payload.password = ''
      payload.database = ''
      payload.authSource = ''
    }
    if (isD1.value) {
      payload.host = ''
      payload.port = 0
      payload.username = ''
      payload.password = ''
      payload.authSource = ''
      payload.database = form.d1DatabaseName.trim()
    }
    if (isChroma.value) {
      payload.username = ''
      payload.password = ''
      payload.database = ''
      payload.authSource = ''
    }
    // Re-emit non-password refs (e.g. options.uri) that have no form UI so editing
    // does not drop them, then layer the password ref from the form when in
    // existing-secret mode. Preserved refs are abandoned when the user changes the
    // datasource type or supplies direct connection details, otherwise the stale
    // external ref would silently override the new connection fields at resolve time.
    const secretRefs: Record<string, any> = {}
    const sameType = normalizeDatasourceType(form.type) === preservedSecretRefsType.value
    if (sameType) {
      // The URI ref is delegated connection material. Drop it only when the user
      // actually changes a direct-connection field versus what was loaded — not
      // merely because host/port metadata was already stored alongside the ref.
      // Otherwise editing an unrelated field would orphan the only connection
      // secret and leave the datasource unresolvable.
      const snapshot = preservedConnectionSnapshot.value
      const current = connectionSignatureOf(payload)
      const connectionChanged =
        current.host !== snapshot.host ||
        current.port !== snapshot.port ||
        current.uri !== snapshot.uri ||
        current.hosts !== snapshot.hosts ||
        current.username !== snapshot.username ||
        current.password !== snapshot.password ||
        current.database !== snapshot.database ||
        current.authSource !== snapshot.authSource
      // A discrete password SecretRef means the user is taking control of the
      // credential directly. A delegated options.uri carries the full connection
      // string and SQL/Mongo adapters prefer it over individual fields, so it would
      // silently shadow the new password ref — drop the URI ref in that case too.
      const passwordRefTakesOverConnection = usePasswordSecretRef.value
      // Temporary DynamoDB SSO credentials live only as options.credentials.* refs and
      // are scoped to the loaded profile/account/role. If the user re-pointed the
      // datasource at a different SSO identity, those refs are stale and must be
      // dropped so resolution falls back to a fresh SSO exchange rather than silently
      // reusing the old role's static credentials.
      const ssoIdentityChanged =
        ssoIdentitySignatureOf(payload) !== preservedSSOIdentitySnapshot.value
      for (const [fieldPath, ref] of Object.entries(preservedSecretRefs.value)) {
        // The URI ref maps to the host/port/uri connection fields; drop it when the
        // user took over the connection.
        if (fieldPath === 'options.uri') {
          if (connectionChanged || passwordRefTakesOverConnection) continue
        } else if (fieldPath.startsWith('options.credentials.')) {
          if (ssoIdentityChanged || payloadOptionPathHasUserValue(payload, fieldPath)) continue
        } else if (payloadOptionPathHasUserValue(payload, fieldPath)) {
          // A form-controlled option (DynamoDB credentials, Chroma/D1 token, …) was
          // given a concrete value; the user's value wins over the external ref.
          continue
        }
        secretRefs[fieldPath] = ref
      }
    }
    if (usePasswordSecretRef.value) {
      const providerConfigId = stringsOrEmpty(form.passwordSecretProviderId)
      const key = stringsOrEmpty(form.passwordSecretKey)
      const field = stringsOrEmpty(form.passwordSecretField) || 'password'
      const version = stringsOrEmpty(form.passwordSecretVersion)
      payload.password = ''
      secretRefs.password = {
        providerConfigId,
        field,
        key,
        ...(version ? { version } : {}),
      }
    }
    if (Object.keys(secretRefs).length > 0) {
      ;(payload as any).secretRefs = secretRefs
    }
    return payload
  }

  const validate = (payload: any) => {
    const nextErrors: string[] = []
    clearFieldErrors()

    if (!payload.name) { nextErrors.push(tApp('validation.nameRequired')); setFieldError('name') }
    if (!payload.type) { nextErrors.push(tApp('validation.typeRequired')); setFieldError('type') }

    const isSQLType = payload.type === 'mysql' || payload.type === 'postgresql'
    const isMongoType = payload.type === 'mongodb'
    const isDynamoType = payload.type === 'dynamodb'
    const isD1Type = payload.type === 'd1'
    const isChromaType = payload.type === 'chromadb'
    const sqlURIMode = isSQLType && form.sqlMode === 'uri'
    const d1Mode = String(payload.options?.mode || '').trim().toLowerCase()
    const d1IsLocalMode = d1Mode === 'local'
    const d1IsCloudMode = d1Mode === 'cloud'
    const mongoHasUri = isMongoType && payload.options?.uri
    const mongoHasHosts = isMongoType && Array.isArray(payload.options?.hosts) && payload.options.hosts.length > 0
    // A resolvable secret ref supplies that option out of band (the plaintext value
    // is absent by design), so a field backed only by a complete ref counts as
    // present for required-field checks — mirrors the backend HasResolvableOptionRef.
    const hasResolvableOptionRef = (fieldPath: string) => {
      const ref = (payload as any).secretRefs?.[fieldPath]
      return Boolean(
        stringsOrEmpty(ref?.providerConfigId) &&
          stringsOrEmpty(ref?.key) &&
          stringsOrEmpty(ref?.field),
      )
    }
    const hasResolvableOptionUriRef = hasResolvableOptionRef('options.uri')
    const mongoAlternativeConnection = mongoHasUri || mongoHasHosts || hasResolvableOptionUriRef

    const validateHostPort = () => {
      if (!payload.host) { nextErrors.push(tApp('validation.hostRequired')); setFieldError('host') }
      if (!payload.port || Number.isNaN(payload.port) || payload.port <= 0) { nextErrors.push(tApp('validation.portRequired')); setFieldError('port') }
    }

    if (isDynamoType) {
      if (!payload.options?.region) {
        nextErrors.push(tApp('validation.regionRequired'))
        setFieldError('dynamoRegion')
      }
      if (String(payload.options?.authMode || '').trim().toLowerCase() === 'sso') {
        if (!stringsOrEmpty(payload.options?.profile)) {
          nextErrors.push(tApp('validation.dynamoProfileRequired'))
          setFieldError('dynamoProfile')
        }
        if (!stringsOrEmpty(payload.options?.ssoAccountId)) {
          nextErrors.push(tApp('validation.dynamoSSOAccountRequired'))
          setFieldError('dynamoSSOAccountId')
        }
        if (!stringsOrEmpty(payload.options?.ssoRoleName)) {
          nextErrors.push(tApp('validation.dynamoSSORoleRequired'))
          setFieldError('dynamoSSORoleName')
        }
        // Each credential is satisfied by an inline value OR a resolvable SecretRef:
        // an existing SSO datasource may store its credentials only as preserved refs
        // (plaintext absent by design), so an unrelated edit or test must not be
        // rejected for "missing" inline credentials it never carries.
        const hasAccessKeyId =
          Boolean(stringsOrEmpty(payload.options?.credentials?.accessKeyId)) ||
          hasResolvableOptionRef('options.credentials.accessKeyId')
        const hasSecretAccessKey =
          Boolean(stringsOrEmpty(payload.options?.credentials?.secretAccessKey)) ||
          hasResolvableOptionRef('options.credentials.secretAccessKey')
        const hasSessionToken =
          Boolean(stringsOrEmpty(payload.options?.credentials?.sessionToken)) ||
          hasResolvableOptionRef('options.credentials.sessionToken')
        if (!hasAccessKeyId || !hasSecretAccessKey || !hasSessionToken) {
          nextErrors.push(tApp('validation.dynamoSSOCredentialsRequired'))
          setFieldError('dynamoSSORoleName')
        }
      }
    } else if (isD1Type) {
      if (!payload.options?.databaseId) {
        nextErrors.push(tApp('validation.d1DatabaseIdRequired'))
        setFieldError('d1DatabaseId')
      }
      if (d1IsLocalMode) {
        if (!payload.options?.binding) {
          nextErrors.push(tApp('validation.d1BindingRequired'))
          setFieldError('d1Binding')
        }
      } else {
        if (!payload.options?.accountId) {
          nextErrors.push(tApp(d1IsCloudMode ? 'validation.d1AccountIdRequired' : 'validation.d1OauthRequired'))
          setFieldError('d1Oauth')
        }
      }
      if (!payload.options?.databaseName && !d1IsLocalMode && !d1IsCloudMode) {
        nextErrors.push(tApp('validation.d1DatabaseNameRequired'))
        setFieldError('d1DatabaseId')
      }
    } else if (isChromaType) {
      validateHostPort()
      const scheme = stringsOrEmpty(payload.options?.scheme).toLowerCase()
      if (scheme && scheme !== 'http' && scheme !== 'https') {
        nextErrors.push(tApp('validation.chromadbSchemeInvalid'))
        setFieldError('chromaScheme')
      }
    } else if (sqlURIMode) {
      if (!String(payload.options?.uri || '').trim() && !hasResolvableOptionUriRef) {
        nextErrors.push(tApp('validation.sqlUriRequired'))
        setFieldError('sqlUri')
      }
    } else if ((!isMongoType || !mongoAlternativeConnection) && !hasResolvableOptionUriRef) {
      validateHostPort()
    }

    if (isSQLType && !sqlURIMode && !hasResolvableOptionUriRef && !payload.username) {
      nextErrors.push(tApp('validation.usernameRequired'))
      setFieldError('username')
    }

    if (payload.options?.hosts) {
      const invalid = payload.options.hosts.some((host: string) => !/^[^:]+:\\d+$/.test(host))
      if (invalid) nextErrors.push(tApp('validation.mongoHostsFormat'))
    }
    if (usePasswordSecretRef.value) {
      const ref = (payload as any).secretRefs?.password
      if (!stringsOrEmpty(ref?.providerConfigId)) {
        nextErrors.push(tApp('validation.secretProviderRequired'))
        setFieldError('passwordSecretProviderId')
      }
      if (!stringsOrEmpty(ref?.key)) {
        nextErrors.push(tApp('validation.secretKeyRequired'))
        setFieldError('passwordSecretKey')
      }
    }
    return nextErrors
  }

  const fillForm = (ds: DataSource | null) => {
    clearFieldErrors()
    clearTestStatus()
    clearD1CreateStatus()
    dynamoSSOProfilesLoading.value = false
    dynamoSSOConfigApplyLoading.value = false
    dynamoSSOOAuthLoading.value = false
    dynamoSSOProfiles.value = []
    clearDynamoSSOSession()
    resetDynamoRegionPreference()
    if (!ds) {
      d1SupportDevTouched.value = false
      preservedAIConfigId.value = ''
      preservedD1Options.value = {}
      preservedSecretRefs.value = {}
      preservedSecretRefsType.value = ''
      preservedConnectionSnapshot.value = emptyConnectionSignature()
      preservedSSOIdentitySnapshot.value = ''
      loadedHasPasswordRef.value = false
      d1Accounts.value = []
      d1Databases.value = []
      d1CreateDatabaseOpen.value = false
      d1CreateDatabaseName.value = ''
      d1OAuthVerified.value = false
      d1HasStoredToken.value = false
      runWithSuppressedDynamoSSOProfileSelection(() => {
        Object.assign(form, {
          name: '', type: 'mysql', host: '', port: String(defaultPortForType('mysql')), username: '', password: '',
          database: defaultDatabaseForType('mysql'), authSource: '', options: '',
          sqlMode: 'userpass', sqlUri: '',
          pgSslEnabled: false, pgSslRootCert: '', pgSslCertFileName: '',
          mysqlSslEnabled: false, mysqlSslRootCert: '', mysqlSslCertFileName: '',
          mongoMode: 'userpass', mongoUri: '', mongoReplicaSet: '', mongoHosts: '', mongoTls: false,
          mongoSslRootCert: '', mongoSslCertFileName: '',
          dynamoAuthMode: 'sso', dynamoRegion: '', dynamoProfile: '', dynamoEndpoint: '',
          dynamoUseStaticCreds: false, dynamoAccessKeyId: '', dynamoSecretAccessKey: '', dynamoSessionToken: '',
          dynamoSSOAccountId: '', dynamoSSORoleName: '', dynamoSSOCredentialExpiration: '', dynamoSSOConfigPath: '',
          d1AccountId: '', d1DatabaseId: '', d1DatabaseName: '', d1Binding: '', d1OauthToken: '',
          d1SupportDev: false, d1DevProjectPath: '',
          chromaScheme: 'http', chromaTenant: 'default_tenant', chromaDatabase: 'default_database', chromaApiToken: '',
          passwordSecretMode: 'manual', passwordSecretProviderId: '', passwordSecretKey: '',
          passwordSecretField: 'password', passwordSecretVersion: '',
        })
      })
      return
    }
    const normalizedOptions = { ...(ds.options || {}) } as Record<string, any>
    const hasOptions = Boolean(ds.options && typeof ds.options === 'object')
    const postgresSslRootCertValue = optionStringValue(ds.options as Record<string, any> | undefined, 'sslrootcert')
    const postgresSslStoredPath = extractStoredCertificatePath(postgresSslRootCertValue)
    const mysqlSslRootCertValue = optionStringValue(ds.options as Record<string, any> | undefined, 'sslrootcert')
    const mysqlSslStoredPath = extractStoredCertificatePath(mysqlSslRootCertValue)
    const mongoSslRootCertValue = optionStringValue(ds.options as Record<string, any> | undefined, 'sslrootcert')
    const mongoSslStoredPath = extractStoredCertificatePath(mongoSslRootCertValue)
    preservedAIConfigId.value = typeof normalizedOptions.aiConfigId === 'string' ? String(normalizedOptions.aiConfigId).trim() : ''
    delete normalizedOptions.aiConfigId
    // Keep every secret ref except password (which has its own form controls) so
    // buildPayload can re-emit non-password refs the UI does not surface.
    preservedSecretRefs.value = Object.fromEntries(
      Object.entries(ds.secretRefs || {}).filter(([fieldPath]) => fieldPath !== 'password'),
    )
    preservedSecretRefsType.value = normalizeDatasourceType(ds.type || '')
    loadedHasPasswordRef.value = Boolean((ds.secretRefs || {}).password)
    const legacyD1DevMetadata = hasLegacyD1DevMetadata(ds.options as Record<string, any> | undefined)
    runWithSuppressedDynamoSSOProfileSelection(() => {
      Object.assign(form, {
        name: ds.name || '',
        type: normalizeDatasourceType(ds.type || 'mysql'),
        host: ds.host || '',
        port: ds.port ? String(ds.port) : '',
        username: ds.username || '',
        password: ds.password || '',
        database: ds.database || '',
        authSource: ds.authSource || '',
        sqlMode: inferSQLConnMode(ds),
        sqlUri: optionStringValue(ds.options as Record<string, any> | undefined, 'uri'),
        pgSslEnabled: inferPostgresSSLEnabled(ds.options as Record<string, any> | undefined),
        pgSslRootCert: postgresSslRootCertValue,
        pgSslCertFileName: extractCertificateNameFromPath(postgresSslStoredPath),
        mysqlSslEnabled: inferMySQLSSLEnabled(ds.options as Record<string, any> | undefined),
        mysqlSslRootCert: mysqlSslRootCertValue,
        mysqlSslCertFileName: extractCertificateNameFromPath(mysqlSslStoredPath),
        options: (
          normalizeDatasourceType(ds.type || '') === 'dynamodb'
          || normalizeDatasourceType(ds.type || '') === 'd1'
          || normalizeDatasourceType(ds.type || '') === 'chromadb'
        )
          ? ''
          : (hasOptions ? JSON.stringify(normalizedOptions, null, 2) : ''),
        mongoMode: inferMongoConnMode(ds),
        mongoUri: ds.options?.uri ? String(ds.options.uri) : '',
        mongoReplicaSet: ds.options?.replicaSet ? String(ds.options.replicaSet) : '',
        mongoHosts: Array.isArray(ds.options?.hosts) ? ds.options.hosts.join(',') : '',
        mongoTls: inferMongoSSLEnabled(ds.options as Record<string, any> | undefined),
        mongoSslRootCert: mongoSslRootCertValue,
        mongoSslCertFileName: extractCertificateNameFromPath(mongoSslStoredPath),
        dynamoAuthMode: String(ds.options?.authMode || '').trim().toLowerCase() === 'sso' ? 'sso' : 'profile',
        dynamoRegion: ds.options?.region ? String(ds.options.region) : '',
        dynamoProfile: ds.options?.profile ? String(ds.options.profile) : '',
        dynamoEndpoint: ds.options?.endpoint ? String(ds.options.endpoint) : '',
        dynamoUseStaticCreds: Boolean((ds.options as any)?.credentials),
        dynamoAccessKeyId: (ds.options as any)?.credentials?.accessKeyId ? String((ds.options as any).credentials.accessKeyId) : '',
        dynamoSecretAccessKey: (ds.options as any)?.credentials?.secretAccessKey
          ? String((ds.options as any).credentials.secretAccessKey)
          : '',
        dynamoSessionToken: (ds.options as any)?.credentials?.sessionToken ? String((ds.options as any).credentials.sessionToken) : '',
        dynamoSSOAccountId: ds.options?.ssoAccountId ? String(ds.options.ssoAccountId) : '',
        dynamoSSORoleName: ds.options?.ssoRoleName ? String(ds.options.ssoRoleName) : '',
        dynamoSSOCredentialExpiration: ds.options?.ssoCredentialExpiration ? String(ds.options.ssoCredentialExpiration) : '',
        dynamoSSOConfigPath: ds.options?.ssoConfigPath ? String(ds.options.ssoConfigPath) : '',
        d1AccountId: ds.options?.accountId ? String(ds.options.accountId) : '',
        d1DatabaseId: ds.options?.databaseId ? String(ds.options.databaseId) : '',
        d1DatabaseName: ds.options?.databaseName ? String(ds.options.databaseName) : (ds.database || ''),
        d1Binding: ds.options?.binding ? String(ds.options.binding) : '',
        d1OauthToken: ds.options?.apiToken ? String(ds.options.apiToken) : '',
        d1SupportDev: (
          boolFromAny((ds.options as any)?.supportDev) && stringsOrEmpty((ds.options as any)?.devProjectPath) !== ''
        ) || legacyD1DevMetadata,
        d1DevProjectPath: ds.options?.devProjectPath ? String(ds.options.devProjectPath) : '',
        chromaScheme: stringsOrEmpty(ds.options?.scheme) || 'http',
        chromaTenant: stringsOrEmpty(ds.options?.tenant) || 'default_tenant',
        chromaDatabase: stringsOrEmpty(ds.options?.database) || 'default_database',
        chromaApiToken: stringsOrEmpty(ds.options?.apiToken),
        passwordSecretMode: ds.secretRefs?.password ? 'existing' : 'manual',
        passwordSecretProviderId: stringsOrEmpty(ds.secretRefs?.password?.providerConfigId),
        passwordSecretKey: stringsOrEmpty(ds.secretRefs?.password?.key),
        passwordSecretField: stringsOrEmpty(ds.secretRefs?.password?.field) || 'password',
        passwordSecretVersion: stringsOrEmpty(ds.secretRefs?.password?.version),
      })
    })
    if (normalizeDatasourceType(ds.type || '') === 'dynamodb') {
      const configuredRegion = stringsOrEmpty(ds.options?.region)
      dynamoRegionManualOverride.value = Boolean(configuredRegion)
      dynamoLastAutofilledRegion.value = ''
    } else {
      resetDynamoRegionPreference()
    }
    const dynamoAuthMode = stringsOrEmpty(ds.options?.authMode).toLowerCase()
    const hasPersistedDynamoSSOCredentials = Boolean(
      stringsOrEmpty((ds.options as any)?.credentials?.accessKeyId)
      && stringsOrEmpty((ds.options as any)?.credentials?.secretAccessKey)
      && stringsOrEmpty((ds.options as any)?.credentials?.sessionToken),
    )
    dynamoSSOVerified.value = Boolean(
      normalizeDatasourceType(ds.type || '') === 'dynamodb'
      && dynamoAuthMode === 'sso'
      && stringsOrEmpty(ds.options?.profile)
      && stringsOrEmpty(ds.options?.ssoAccountId)
      && stringsOrEmpty(ds.options?.ssoRoleName)
      && hasPersistedDynamoSSOCredentials,
    )
    d1SupportDevTouched.value = false
    d1CreateDatabaseOpen.value = false
    d1CreateDatabaseName.value = ''
    d1OAuthVerified.value = false
    if (normalizeDatasourceType(ds.type || '') === 'd1') {
      preservedD1Options.value = { ...normalizedOptions }
      const rawMode = stringsOrEmpty(ds.options?.mode).toLowerCase()
      d1LegacyMode.value = rawMode === 'local' || rawMode === 'cloud' ? rawMode : ''
      const accountId = stringsOrEmpty(ds.options?.accountId)
      const databaseId = stringsOrEmpty(ds.options?.databaseId)
      const databaseName = stringsOrEmpty(ds.options?.databaseName) || stringsOrEmpty(ds.database)
      d1Accounts.value = accountId ? [{ id: accountId, name: accountId }] : []
      d1Databases.value = databaseId
        ? [{ id: databaseId, name: databaseName || databaseId }]
        : []
      // A token is stored when the payload carries an (inline, possibly redacted)
      // apiToken or delegates it to a SecretRef; either way the real value lives
      // server-side and the edit form treats the connection as already authenticated.
      const hasStoredToken =
        stringsOrEmpty(ds.options?.apiToken) !== '' ||
        Boolean((ds.secretRefs || {})['options.apiToken'])
      d1HasStoredToken.value = hasStoredToken
      d1OAuthVerified.value = hasStoredToken
    } else {
      d1HasStoredToken.value = false
      preservedD1Options.value = {}
      d1LegacyMode.value = ''
      d1Accounts.value = []
      d1Databases.value = []
      form.d1SupportDev = false
      form.d1DevProjectPath = ''
      d1SupportDevTouched.value = false
    }
    // Baseline the loaded connection from the fully-populated form so an unrelated
    // edit won't look like the user took over the connection and drop a preserved
    // options.uri ref. Defer to nextTick so the form.type watcher has applied its
    // defaults (e.g. the type's default port) before we fingerprint the connection.
    void nextTick(() => {
      try {
        const baseline = buildPayload()
        preservedConnectionSnapshot.value = connectionSignatureOf(baseline)
        preservedSSOIdentitySnapshot.value = ssoIdentitySignatureOf(baseline)
      } catch {
        preservedConnectionSnapshot.value = emptyConnectionSignature()
        preservedSSOIdentitySnapshot.value = ''
      }
    })
  }

  const save = async () => {
    if (store.formMode === 'create' && hasReachedDatasourceLimit(authStore.effectivePlan, store.datasources.length)) {
      const message = datasourceLimitNotice(authStore.effectivePlan)
      errors.value = [message]
      store.setNotice(message, 'error')
      return
    }
    let payload
    try { payload = buildPayload() } catch { errors.value = [tApp('validation.optionsJson')]; return }
    const validationErrors = validate(payload)
    if (validationErrors.length) { errors.value = validationErrors; return }
    try {
      if (store.formMode === 'create') await api.createDatasource(payload)
      else if (store.formId) await api.updateDatasource(store.formId, payload)
      await store.loadDatasources()
      router.push({ name: 'datasources' })
    } catch (err) {
      const planMessage = resolvePlanLimitMessage(err, authStore.effectivePlan)
      if (planMessage) {
        errors.value = [planMessage]
        store.setNotice(planMessage, 'error')
        return
      }
      errors.value = [err instanceof Error ? err.message : String(err)]
    }
  }

  const testConnection = async () => {
    clearTestStatus()
    let payload
    try { payload = buildPayload() } catch { errors.value = [tApp('validation.optionsJson')]; return }
    const validationErrors = validate(payload)
    if (validationErrors.length) { errors.value = validationErrors; return }
    try {
      testStatusText.value = tApp('status.testing')
      testStatusDetail.value = ''
      testStatusClass.value = 'testing'
      await api.testDatasourcePayload(payload, store.formId || '')
      testStatusText.value = tApp('status.connected')
      testStatusDetail.value = ''
      testStatusClass.value = 'connected'
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      testStatusText.value = tApp('status.failed')
      testStatusDetail.value = message
      testStatusClass.value = 'failed'
    }
  }

  const cancel = () => { router.push({ name: 'datasources' }) }

  const markD1SupportDevTouched = () => {
    d1SupportDevTouched.value = true
  }

  const loadFormData = async () => {
    await ensureDatasourcesLoaded()
    if (!store.formId) {
      fillForm(null)
      return
    }
    fillForm(store.datasources.find((item) => item.id === store.formId) || null)
    // A SecretRef-backed token leaves form.d1OauthToken empty (the real value is
    // resolved server-side), so d1OAuthConnected is false. Trigger the refresh on a
    // stored token too; loadD1CloudDatabases routes it through the id-based binding.
    if (isD1.value && (d1OAuthConnected.value || d1HasStoredToken.value)) {
      await loadD1CloudDatabases()
    }
    if (isDynamoSSO.value) {
      await loadDynamoSSOProfiles()
    }
  }

  const handlePasswordSecretModeChange = () => {
    if (form.passwordSecretMode === 'existing') {
      if (!stringsOrEmpty(form.passwordSecretProviderId)) {
        form.passwordSecretProviderId = resolvePreferredSecretProviderId(form.passwordSecretProviderId)
      }
      if (!stringsOrEmpty(form.passwordSecretField)) {
        form.passwordSecretField = 'password'
      }
    } else {
      // Switched back to manual entry. For a ref-backed datasource fillForm loaded
      // the redacted sentinel; leaving it would save password:"[REDACTED]" with no
      // refs, which the backend treats as "unchanged" and restores the old ref —
      // silently failing to drop the reference. Clear the marker so the user must
      // type a real value (or save an empty password) to replace the secret.
      //
      // Only do this when the loaded password was actually ref-backed. An inline
      // password also comes back as "[REDACTED]"; clearing it there would send an
      // empty password the backend reads as an intentional wipe of the stored
      // credential, so the sentinel must be preserved for inline-password records.
      if (loadedHasPasswordRef.value && form.password === '[REDACTED]') {
        form.password = ''
      }
    }
  }

  onMounted(async () => {
    syncFormStateFromRoute()
    await Promise.all([loadSecretProviders(), loadFormData()])
  })

  watch(
    () => form.type,
    (next, prev) => {
      const nextPort = defaultPortForType(next)
      const prevPort = defaultPortForType(prev || '')
      const currentPort = (form.port || '').trim()
      if (nextPort && (currentPort === '' || (prevPort && currentPort === String(prevPort)))) form.port = String(nextPort)

      const nextDb = defaultDatabaseForType(next)
      const prevDb = defaultDatabaseForType(prev || '')
      const currentDb = (form.database || '').trim()
      if ((next === 'mysql' || next === 'postgresql') && nextDb && (currentDb === '' || (prevDb && currentDb === prevDb))) form.database = nextDb
      if (next === 'mongodb' && prevDb && currentDb === prevDb) form.database = ''
      if (normalizeDatasourceType(next) === 'redis') form.database = ''
      if (normalizeDatasourceType(next) === 'elasticsearch') form.database = ''
      if (normalizeDatasourceType(next) === 'dynamodb') form.database = ''
      if (normalizeDatasourceType(next) === 'd1') form.database = ''
      if (normalizeDatasourceType(next) === 'chromadb') form.database = ''
      if (normalizeDatasourceType(next) === 'dynamodb') form.options = ''
      if (normalizeDatasourceType(next) === 'd1') form.options = ''
      if (normalizeDatasourceType(next) === 'chromadb') form.options = ''
      if (normalizeDatasourceType(next) !== 'chromadb') {
        form.chromaScheme = 'http'
        form.chromaTenant = 'default_tenant'
        form.chromaDatabase = 'default_database'
        form.chromaApiToken = ''
      }
      if (normalizeDatasourceType(next) !== 'dynamodb') {
        form.dynamoAuthMode = 'sso'
        form.dynamoSSOAccountId = ''
        form.dynamoSSORoleName = ''
        form.dynamoSSOCredentialExpiration = ''
        form.dynamoSSOConfigPath = ''
        dynamoSSOConfigApplyLoading.value = false
        dynamoSSOProfiles.value = []
        clearDynamoSSOSession()
        resetDynamoRegionPreference()
      } else if (String(form.dynamoAuthMode) === 'sso') {
        void loadDynamoSSOProfiles()
      }
      if (normalizeDatasourceType(next) !== 'mysql' && normalizeDatasourceType(next) !== 'postgresql') {
        form.sqlMode = 'userpass'
        form.sqlUri = ''
      }
      if (normalizeDatasourceType(next) !== 'postgresql') {
        form.pgSslEnabled = false
        form.pgSslRootCert = ''
        form.pgSslCertFileName = ''
      }
      if (normalizeDatasourceType(next) !== 'mysql') {
        form.mysqlSslEnabled = false
        form.mysqlSslRootCert = ''
        form.mysqlSslCertFileName = ''
      }
      if (normalizeDatasourceType(next) !== 'mongodb') {
        form.mongoTls = false
        form.mongoSslRootCert = ''
        form.mongoSslCertFileName = ''
      }
      if (normalizeDatasourceType(next) !== 'd1') {
        preservedD1Options.value = {}
        d1LegacyMode.value = ''
        d1WranglerMissing.value = false
        d1WranglerChecked.value = false
        d1WranglerCheckToken.value += 1
        d1Accounts.value = []
        d1Databases.value = []
        d1DatabasesLoading.value = false
        invalidateD1DatabaseLoadRequests()
        d1CreateDatabaseOpen.value = false
        d1CreateDatabaseName.value = ''
        d1OAuthVerified.value = false
        d1HasStoredToken.value = false
        form.d1SupportDev = false
        form.d1DevProjectPath = ''
        d1SupportDevTouched.value = false
        clearD1CreateStatus()
      } else {
        syncD1DatabaseName()
        void detectD1WranglerAvailability()
      }
      clearTestStatus()
    },
    { immediate: true },
  )

  watch(
    () => [
      form.type,
      form.host,
      form.port,
      form.username,
      form.password,
      form.database,
      form.authSource,
      form.options,
      form.sqlMode,
      form.sqlUri,
      form.pgSslEnabled,
      form.pgSslRootCert,
      form.mysqlSslEnabled,
      form.mysqlSslRootCert,
      form.mongoMode,
      form.mongoUri,
      form.mongoReplicaSet,
      form.mongoHosts,
      form.mongoTls,
      form.mongoSslRootCert,
      form.dynamoAuthMode,
      form.dynamoRegion,
      form.dynamoProfile,
      form.dynamoEndpoint,
      form.dynamoUseStaticCreds,
      form.dynamoAccessKeyId,
      form.dynamoSecretAccessKey,
      form.dynamoSessionToken,
      form.dynamoSSOAccountId,
      form.dynamoSSORoleName,
      form.dynamoSSOConfigPath,
      form.d1AccountId,
      form.d1DatabaseId,
      form.d1DatabaseName,
      form.d1Binding,
      form.d1OauthToken,
      form.d1SupportDev,
      form.d1DevProjectPath,
      form.chromaScheme,
      form.chromaTenant,
      form.chromaDatabase,
      form.chromaApiToken,
      form.passwordSecretMode,
      form.passwordSecretProviderId,
      form.passwordSecretKey,
      form.passwordSecretField,
      form.passwordSecretVersion,
    ],
    () => {
      if (testStatusText.value) clearTestStatus()
      if (stringsOrEmpty(form.d1OauthToken)) {
        d1OAuthVerified.value = false
      }
    },
  )

  watch(
    () => form.d1DatabaseId,
    () => {
      if (!isD1.value) return
      if (stringsOrEmpty(form.d1DatabaseId) === d1DatabaseCreateOptionValue) return
      syncD1DatabaseName()
    },
  )

  watch(
    () => [form.type, form.d1AccountId, form.d1OauthToken],
    async ([nextType, nextAccountId, nextToken], [prevType, prevAccountId, prevToken]) => {
      if (normalizeDatasourceType(nextType) !== 'd1') return
      if (d1OAuthLoading.value) return
      const accountId = stringsOrEmpty(nextAccountId)
      const token = stringsOrEmpty(nextToken)
      // An editing datasource with a stored token reports an empty/redacted token but
      // is still connected; keep it verified and let the id-based refresh run below.
      const useStoredToken = d1StoredTokenActive()
      if (!accountId || (!token && !useStoredToken)) {
        if (!token && !useStoredToken) {
          d1OAuthVerified.value = false
        }
        const requiresOAuthSelection = d1LegacyMode.value !== 'local' && d1LegacyMode.value !== 'cloud'
        invalidateD1DatabaseLoadRequests()
        d1DatabasesLoading.value = false
        if (!accountId && requiresOAuthSelection) {
          d1Databases.value = []
          form.d1DatabaseId = ''
          form.d1DatabaseName = ''
        }
        return
      }
      const accountChanged = accountId !== stringsOrEmpty(prevAccountId)
      const tokenChanged = token !== stringsOrEmpty(prevToken)
      const switchedToD1 = normalizeDatasourceType(prevType || '') !== 'd1'
      if (!accountChanged && !tokenChanged && !switchedToD1) return
      if (accountChanged && !switchedToD1) {
        d1Databases.value = []
        form.d1DatabaseId = ''
        form.d1DatabaseName = ''
        d1CreateDatabaseOpen.value = false
        d1CreateDatabaseName.value = ''
      }
      await loadD1CloudDatabases()
    },
  )

  watch(
    () => [form.type, form.dynamoAuthMode],
    async ([nextType, nextMode], [prevType, prevMode]) => {
      if (normalizeDatasourceType(nextType) !== 'dynamodb') return
      const isSSO = String(nextMode || '') === 'sso'
      const switchedToDynamo = normalizeDatasourceType(prevType || '') !== 'dynamodb'
      const modeChanged = String(nextMode || '') !== String(prevMode || '')
      if (!modeChanged && !switchedToDynamo) return
      if (!isSSO) {
        clearDynamoSSOSession()
        return
      }
      await loadDynamoSSOProfiles()
    },
  )

  watch(
    () => form.dynamoProfile,
    (nextProfile, prevProfile) => {
      if (!isDynamoSSO.value) return
      if (suppressDynamoSSOProfileSelection) return
      if (stringsOrEmpty(nextProfile) === stringsOrEmpty(prevProfile)) return
      handleDynamoSSOProfileSelection()
    },
  )

  watch(() => route.fullPath, async () => {
    syncFormStateFromRoute()
    await loadFormData()
  })

  return {
    store,
    form,
    errors,
    testStatusText,
    testStatusDetail,
    testStatusClass,
    d1CreateStatusText,
    d1CreateStatusDetail,
    d1CreateStatusClass,
    dataSourceTypeOptions,
    isMongo,
    isRedis,
    isSQL,
    secretProviders,
    secretProvidersAvailable,
    showPasswordSecretRef,
    usePasswordSecretRef,
    handlePasswordSecretModeChange,
    isDynamo,
    isDynamoSSO,
    isD1,
    isChroma,
    pgSslStoredCertificatePath,
    pgSslDisplayedCertificateName,
    mysqlSslStoredCertificatePath,
    mysqlSslDisplayedCertificateName,
    mongoSslStoredCertificatePath,
    mongoSslDisplayedCertificateName,
    formTitle,
    portPlaceholder,
    databasePlaceholder,
    hint,
    installGuide,
    copyInstall,
    copyDynamoSecretAccessKey,
    copyDynamoSessionToken,
    maskedDynamoSecretAccessKey,
    maskedDynamoSessionToken,
    importDynamoCredentialsFromFile,
    importPostgresCertificateFromFile,
    importMySQLCertificateFromFile,
    importMongoCertificateFromFile,
    dynamoSSOProfiles,
    dynamoSSOProfilesLoading,
    dynamoSSOConfigApplyLoading,
    dynamoSSOOAuthLoading,
    dynamoSSOVerified,
    dynamoSSOConnected,
    dynamoSSOConfigEndpoint,
    dynamoSSOHasConfigEndpoint,
    loadDynamoSSOProfiles,
    applyDynamoSSOConfigPath,
    handleDynamoSSOProfileSelection,
    dynamoSSOOAuthAuthorize,
    markDynamoRegionAsManual,
    showPostgresCertificatePath,
    showMySQLCertificatePath,
    showMongoCertificatePath,
    fieldClass,
    d1Accounts,
    d1Databases,
    d1OAuthLoading,
    d1DatabasesLoading,
    d1CreateDatabaseOpen,
    d1CreateDatabaseLoading,
    d1CreateDatabaseName,
    d1DatabaseCreateOptionValue,
    d1OAuthAuthenticated,
    d1OAuthVerified,
    d1ConnectionEstablished,
    d1CanCreateDatabase,
    d1WranglerMissing,
    d1OAuthConnected,
    showD1DatabaseSelector,
    d1OAuthLogin,
    handleD1AccountSelection,
    handleD1DatabaseSelection,
    createD1Database,
    cancelCreateD1Database,
    markD1SupportDevTouched,
    save,
    testConnection,
    cancel,
  }
}
